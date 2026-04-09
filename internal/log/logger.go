package log

import (
	"fmt"
	"io"
	"os"
)

// Logger provides conditional debug logging.
// When verbose is false, all Debug calls are no-ops.
type Logger struct {
	verbose bool
	w       io.Writer
}

// New creates a Logger that writes to stderr.
func New(verbose bool) *Logger {
	return &Logger{
		verbose: verbose,
		w:       os.Stderr,
	}
}

// NewWithWriter creates a Logger that writes to the given writer.
func NewWithWriter(verbose bool, w io.Writer) *Logger {
	return &Logger{
		verbose: verbose,
		w:       w,
	}
}

// Debug prints a debug line prefixed with [DEBUG] only when verbose is true.
func (l *Logger) Debug(format string, args ...interface{}) {
	if !l.verbose {
		return
	}
	fmt.Fprintf(l.w, "[DEBUG] "+format+"\n", args...)
}
