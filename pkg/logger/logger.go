package logger

import (
	"log/syslog"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if os.Getenv("APP_ENV") == "dev" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
		return
	}

	// Non-dev mode: try syslog
	w, err := syslog.New(syslog.LOG_INFO|syslog.LOG_USER, "moshpf")
	if err == nil {
		// Direct zerolog output to syslog writer with level mapping
		log.Logger = zerolog.New(zerolog.SyslogLevelWriter(w))
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	} else {
		// Fallback to a quiet console output if syslog fails.
		// We use WarnLevel to keep the CLI clean of Info/Debug logs.
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}
}
