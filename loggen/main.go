package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang/glog"
)

var (
	// FLAGS
	helpFlag         *bool
	bytesPerLine     *int64
	logID            *string
	timing           *int64
	totalRuntimeSecs *int64

	seq int64
)

func main() {
	// DEFAULTS
	dBPL, err := strconv.ParseInt(os.Getenv("BYTESPERLINE"), 10, 64)
	if err != nil || dBPL == 0 {
		dBPL = 10240 //10k
	}

	dLID := os.Getenv("LOGID")
	if dLID == "" {
		dLID = "LogGenerator"
	}

	dT, err := strconv.ParseInt(os.Getenv("TIMING"), 10, 64)
	if err != nil || dT == 0 {
		dT = 1000
	}

	dR, err := strconv.ParseInt(os.Getenv("RUNTIME"), 10, 64)
	if err != nil || dR == 0 {
		dR = 1800
	}

	// FLAGS
	helpFlag = flag.Bool("help", false, "")
	bytesPerLine = flag.Int64("bpl", dBPL, "Number of bytes per line, if this is to short the line identifier could be truncated. Generally you want number of bytes in the logid + 1 + 64 at least. Env: BYTESPERLINE or 512")
	logID = flag.String("logid", dLID, "String used to identify this particular log. Env: LOGID or LogGenerator")
	timing = flag.Int64("timing", dT, "Spit one log line every n milliseconds (approx.)... Env: TIMING of 1000")
	totalRuntimeSecs = flag.Int64("runtime", dR, "Total runtime for log producer")

	flag.Parse()

	// there are 42 bytes is the glog "line header"
	*bytesPerLine = *bytesPerLine - 42

	if *helpFlag {
		flag.PrintDefaults()
		fmt.Println("Turn up verbosity (-v) to 1 to get a few extra messages.")
		os.Exit(0)
	}

	glog.V(1).Infof("Printing one line of %d bytes every %d milliseconds for %d seconds", *bytesPerLine, *timing, *totalRuntimeSecs)

	ticker := time.NewTicker(time.Millisecond * time.Duration(*timing))

	runtimeMs := *totalRuntimeSecs * 1000
	go func() {
		for range ticker.C {
			line := fmt.Sprintf("%s:%d ", *logID, seq)
			line = pad(line)
			glog.V(0).Infoln(line)
			seq++
			runtimeMs -= *timing
			if runtimeMs <= 0 {
				glog.V(1).Infof("LogProducer finished at seq:%d\n", seq)
				for {
					glog.V(1).Infoln("Sleeping for 5 mins. kill me before I wakeup and make it quick")
					time.Sleep(5 * time.Minute)
				}
			}
		}
	}()

	select {}

}

func pad(s string) string {
	for {
		s += "DEADFEED5"
		if int64(len(s)) > (*bytesPerLine) {
			return s[0:*bytesPerLine]
		}
	}
}
