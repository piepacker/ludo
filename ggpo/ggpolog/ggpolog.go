package ggpolog

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func init() {
	date := time.Now().Format("2006_01_02_15.04.05")

	f, err := os.OpenFile("logs/ggpologfile_"+date+".log", os.O_WRONLY|os.O_CREATE, 0755)
	if err != nil {
		log.Println(err)
	}
	mw := io.MultiWriter(os.Stdout, f)
	logrus.SetOutput(mw)

	logrus.SetReportCaller(true)
	formatter := &logrus.TextFormatter{
		TimestampFormat:        "02-01-2006 15:04:05", // the "time" field configuration
		FullTimestamp:          true,
		DisableLevelTruncation: true, // log level field configuration
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			return "", fmt.Sprintf("%s:%d", formatFilePath(f.File), f.Line)
		},
	}
	logrus.SetFormatter(formatter)
}

func formatFilePath(path string) string {
	arr := strings.Split(path, "/")
	return arr[len(arr)-1]
}
