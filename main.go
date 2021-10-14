package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/faiface/beep"
	"github.com/faiface/beep/wav"
	"github.com/sirupsen/logrus"
)

const (
	LEFT = iota
	RIGHT

	SILENCE = 0.0000003
)

var (
	BuildDate    string
	BuildVersion string

	Version  = flag.Bool("version", false, "Show build information")
	LogLevel = flag.String("log-level", "debug", "Level of the output log")
)

type Sampler struct {
	/* open and ready to read input stream */
	inputStream beep.Streamer

	/*
	 * indicates that the underlying (input) stream has nothing to
	 * read anymore
	 */
	EOF bool

	/*
	 * contains remaining non-silence samples from the last stream that
	 * should get reused next time
	 */
	buffer [][2]float64

	/* last error that happened */
	err error

	/* some stats */
	Min float64
	Max float64
	N   int64
	Sum float64
}

func NewSampler(inputStream beep.Streamer) *Sampler {
	return &Sampler{
		inputStream: inputStream,
		Min:         math.MaxFloat64,
		Max:         -math.MaxFloat64,
	}
}

func (s Sampler) Err() error {
	return s.err
}

func (s *Sampler) Stream(samples [][2]float64) (int, bool) {
	/* use buffer before reading from stream again */
	if len(s.buffer) > 0 {
		logrus.Debugf("Using buffer from last time with length %d\n", len(s.buffer))

		/* TODO: Check if copy to samples is valid without allocation */
		n := copy(samples, s.buffer)

		/* reset buffer so next stream will not trigger this branch again */
		s.buffer = nil
		return n, true
	}

	/* read from underlying input stream */
	n, ok := s.inputStream.Stream(samples)
	if !ok {
		/* set error and return immediately */
		s.err = fmt.Errorf("error reading stream: %w", s.inputStream.Err())
		return n, ok && s.Err() == nil
	}

	if n == 0 {
		logrus.Debugf("EOF reached")
		s.EOF = true
		return n, ok
	}

	var silenceDetected bool
	var end int
	logrus.Debugf("Cycling through %d samples\n", len(samples))
	for i, sample := range samples {

		if sample[LEFT] > s.Max {
			s.Max = sample[LEFT]
		}
		if sample[RIGHT] > s.Max {
			s.Max = sample[RIGHT]
		}
		if sample[LEFT] < s.Min {
			s.Min = sample[LEFT]
		}
		if sample[RIGHT] < s.Min {
			s.Min = sample[RIGHT]
		}

		s.Sum += sample[LEFT] + sample[RIGHT]
		s.N += 1

		/* cut sample at silence */
		// fmt.Println(sample)
		if IsSilence(sample) {
			if !silenceDetected {
				logrus.Debugf("detected silence @%d\n", i)
				end = i
				silenceDetected = true
			}
		} else if silenceDetected {
			/* if sample has been split by silence, finish reading*/
			s.buffer = samples[:end]
			samples = samples[i:]
			return end, false
		}
	}

	/* just skip the samples if all are silence */
	if silenceDetected {
		return 0, true
	}

	return n, ok
}

func IsSilence(sample [2]float64) bool {
	return math.Abs(sample[LEFT]) < SILENCE &&
		math.Abs(sample[RIGHT]) < SILENCE
}

func main() {
	flag.Parse()

	if *Version {
		fmt.Printf("BuildDate: %s\n", BuildDate)
		fmt.Printf("BuildVersion: %s\n", BuildVersion)
		return
	}

	level, err := logrus.ParseLevel(*LogLevel)
	if err != nil {
		logrus.Errorf("error parsing log level %s: %w", level, err)
		return
	}

	logrus.SetLevel(level)

	if flag.NArg() != 2 {
		logrus.Error("usage: go-sample <recording> <out>")
		return
	}

	if !strings.Contains(flag.Arg(1), "%d") {
		logrus.Error("output file needs %d formatter")
		return
	}

	inF, err := os.Open(flag.Arg(0))
	if err != nil {
		logrus.Errorf("error loading recording '%s': %s\n", flag.Arg(0), err)
		return
	}
	defer inF.Close()

	stream, format, err := wav.Decode(inF)
	if err != nil {
		logrus.Errorf("error decoding recording '%s': %s\n", flag.Arg(0), err)
		return
	}
	defer stream.Close()

	sampler := NewSampler(stream)
	for i := 0; !sampler.EOF && sampler.Err() == nil; i++ {
		outFilename := fmt.Sprintf(flag.Arg(1), i)
		logrus.Infof("NEW FILE %s\n", outFilename)
		outF, err := os.Create(outFilename)
		if err != nil {
			logrus.Errorf("error opening output file '%s': %s\n", outFilename, err)
			return
		}
		if err := wav.Encode(outF, sampler, format); err != nil {
			logrus.Errorf("error writing output file '%s': %s\n", outFilename, err)
			return
		}
		outF.Close()
	}

	logrus.Tracef("Min: %f\n", sampler.Min)
	logrus.Tracef("Max: %f\n", sampler.Max)
	logrus.Tracef("Avg: %f\n", sampler.Sum/float64(sampler.N))
}
