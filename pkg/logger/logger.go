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
	} else {
		w, err := syslog.New(syslog.LOG_INFO|syslog.LOG_USER, "moshpf")
		if err != nil {
			// Fallback to stderr if syslog fails
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
			return
		}
		log.Logger = zerolog.New(zerolog.SyslogLevelWriter(w))
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}
