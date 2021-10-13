package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/faiface/beep"
	"github.com/faiface/beep/wav"
)

const (
	LEFT = iota
	RIGHT

	SILENCE = 0.0000003
)

var (
	BuildDate    = "hello"
	BuildVersion string
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
		fmt.Printf("Using buffer from last time with length %d\n", len(s.buffer))

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
		fmt.Println("EOF reached")
		s.EOF = true
		return n, ok
	}

	var silenceDetected bool
	var end int
	fmt.Printf("Cycling through %d samples\n", len(samples))
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
				fmt.Printf("detected silence @%d\n", i)
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

	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("BuildVersion: ", BuildVersion)
		fmt.Println("BuildDate: ", BuildDate)
		return
	}

	if len(os.Args) != 3 {
		fmt.Println("usage: go-sample <recording> <out>")
		return
	}

	if !strings.Contains(os.Args[2], "%d") {
		fmt.Println("output file needs %d formatter")
		return
	}

	inF, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Printf("error loading recording '%s': %s\n", os.Args[1], err)
		return
	}
	defer inF.Close()

	stream, format, err := wav.Decode(inF)
	if err != nil {
		fmt.Printf("error decoding recording '%s': %s\n", os.Args[1], err)
		return
	}
	defer stream.Close()

	sampler := NewSampler(stream)
	for i := 0; !sampler.EOF && sampler.Err() == nil; i++ {
		outFilename := fmt.Sprintf(os.Args[2], i)
		fmt.Printf("NEW FILE %s\n", outFilename)
		outF, err := os.Create(outFilename)
		if err != nil {
			fmt.Printf("error opening output file '%s': %s\n", outFilename, err)
			return
		}
		if err := wav.Encode(outF, sampler, format); err != nil {
			fmt.Printf("error writing output file '%s': %s\n", outFilename, err)
			return
		}
		outF.Close()
	}

	fmt.Printf("Min: %f\n", sampler.Min)
	fmt.Printf("Max: %f\n", sampler.Max)
	fmt.Printf("Avg: %f\n", sampler.Sum/float64(sampler.N))

	fmt.Println("ok")
}
