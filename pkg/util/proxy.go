package util

import (
	"io"
	"sync"
)

// Proxy copies data between two ReadWriteClosers in both directions.
// It blocks until both directions are finished or an error occurs.
func Proxy(c1, c2 io.ReadWriteCloser) {
	defer c1.Close()
	defer c2.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(c1, c2)
	}()

	go func() {
		defer wg.Done()
		io.Copy(c2, c1)
	}()

	wg.Wait()
}
