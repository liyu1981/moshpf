package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/liyu1981/moshpf/pkg/util"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

func Init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if util.IsDev() {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
		return
	}

	// Non-dev mode: use file-based logging with rotation in ~/.mpf/log
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get user home directory: %v\n", err)
		os.Exit(1)
	}

	logDir := filepath.Join(home, ".mpf", "log")
	_ = os.MkdirAll(logDir, 0700)

	logFile := filepath.Join(logDir, fmt.Sprintf("agent-%d.log", os.Getuid()))

	lumberjackLogger := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    20, // 20 megabytes
		MaxBackups: 3,
		LocalTime:  true,
		Compress:   true,
	}


	// We use InfoLevel as the base for the file log.
	// We use MultiLevelWriter to log to both file and stderr if needed,
	// but here we just log to file. If we want silence on console, we only set file as output.
	log.Logger = zerolog.New(lumberjackLogger).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}
