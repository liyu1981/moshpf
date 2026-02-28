package util

import "io"

type NopWriterCloser struct {
	io.Writer
}

func (NopWriterCloser) Close() error { return nil }
