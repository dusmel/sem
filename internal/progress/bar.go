package progress

import (
	"os"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
)

// Bar wraps a progressbar.ProgressBar and provides a no-op mode
// when progress bars should be suppressed (verbose mode or non-TTY).
type Bar struct {
	bar   *progressbar.ProgressBar
	noop  bool
	total int
}

// New creates a progress bar. If noop is true, the returned Bar
// is a no-op — all Add/Set calls are silently ignored.
// Use this when verbose mode is enabled or stderr is not a TTY.
func New(total int, description string, noop bool) *Bar {
	if noop {
		return &Bar{noop: true, total: total}
	}

	bar := progressbar.NewOptions(total,
		progressbar.OptionSetDescription(description),
		progressbar.OptionShowCount(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionSetSpinnerChangeInterval(0), // Disable render throttle for immediate updates
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionOnCompletion(func() {
			// Move to next line when done
		}),
	)
	return &Bar{bar: bar, total: total}
}

// Add advances the progress bar by n. No-op if the bar is disabled.
func (b *Bar) Add(n int) {
	if b.noop {
		return
	}
	_ = b.bar.Add(n)
}

// Set sets the progress bar to an absolute value. No-op if disabled.
func (b *Bar) Set(n int) {
	if b.noop {
		return
	}
	_ = b.bar.Set(n)
}

// ShouldDisable returns true if progress bars should be suppressed.
// This is the case when verbose mode is enabled (debug logging goes to stderr)
// or when stderr is not a TTY (piped output).
func ShouldDisable(verbose bool) bool {
	if verbose {
		return true
	}
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return true
	}
	return false
}
