package api

import (
	"context"
	"io"
	"time"
)

// stallTimeout bounds the gap between successive body-transfer progress
// events. The client has no overall timeout (it would cap transfer size), so
// without this a stalled-but-alive connection would hang a worker forever.
var stallTimeout = 2 * time.Minute

type progressWatchdog struct {
	timer *time.Timer
}

func newProgressWatchdog(cancel context.CancelFunc) *progressWatchdog {
	return &progressWatchdog{timer: time.AfterFunc(stallTimeout, func() { cancel() })}
}

func (w *progressWatchdog) Touch() {
	w.timer.Reset(stallTimeout)
}

func (w *progressWatchdog) Stop() {
	w.timer.Stop()
}

type watchdogReader struct {
	reader   io.Reader
	watchdog *progressWatchdog
}

func (wr watchdogReader) Read(p []byte) (int, error) {
	n, err := wr.reader.Read(p)
	if n > 0 {
		wr.watchdog.Touch()
	}
	return n, err
}

type watchdogWriter struct {
	writer   io.Writer
	watchdog *progressWatchdog
}

func (ww watchdogWriter) Write(p []byte) (int, error) {
	n, err := ww.writer.Write(p)
	if n > 0 {
		ww.watchdog.Touch()
	}
	return n, err
}
