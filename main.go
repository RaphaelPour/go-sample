package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/faiface/beep/wav"
	"github.com/sirupsen/logrus"
)

var (
	BuildDate    string
	BuildVersion string

	Version  = flag.Bool("version", false, "Show build information")
	LogLevel = flag.String("log-level", "debug", "Level of the output log")
)

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
