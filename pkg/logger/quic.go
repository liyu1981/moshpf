package logger

import (
	"context"

	"github.com/liyu1981/moshpf/pkg/util"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlog"
	"github.com/quic-go/quic-go/qlogwriter"
	"github.com/rs/zerolog/log"
)

type QuicTracer func(ctx context.Context, isClient bool, connID quic.ConnectionID) qlogwriter.Trace

func GetQuicTracer() QuicTracer {
	var tracerFn func(ctx context.Context, isClient bool, connID quic.ConnectionID) qlogwriter.Trace

	if util.IsDev() {
		tracerFn = func(ctx context.Context, isClient bool, connID quic.ConnectionID) qlogwriter.Trace {
			return qlogwriter.NewConnectionFileSeq(util.NopWriterCloser{Writer: &log.Logger}, isClient, connID, []string{qlog.EventSchema})
		}
	} else {
		tracerFn = nil
	}

	return tracerFn
}
