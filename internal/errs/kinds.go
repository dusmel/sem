package errs

import (
	"errors"
)

var (
	ErrNotInitialized     = errors.New("sem is not initialized")
	ErrAlreadyInitialized = errors.New("sem is already initialized")
	ErrNoSources          = errors.New("no sources configured")
	ErrSourceExists       = errors.New("source already exists")
	ErrSourceNotFound     = errors.New("source not found")
	ErrIndexNotFound      = errors.New("index not found")
	ErrModelMismatch      = errors.New("embedding model mismatch, full rebuild required")
	ErrConfigChanged      = errors.New("chunking configuration changed, full rebuild required")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

func Format(err error) string {
	switch {
	case errors.Is(err, ErrNotInitialized):
		return "sem is not initialized. Run `sem init` first."
	case errors.Is(err, ErrAlreadyInitialized):
		return "sem is already initialized. Use `sem init --force` to recreate the default files."
	case errors.Is(err, ErrNoSources):
		return "no sources configured. Add one with `sem source add <path>`."
	case errors.Is(err, ErrSourceExists):
		return "source already exists. Use a different name or remove the existing source first."
	case errors.Is(err, ErrSourceNotFound):
		return "source not found. Check `sem source list` and try again."
	case errors.Is(err, ErrIndexNotFound):
		return "index not found. Run `sem index` first."
	default:
		return err.Error()
	}
}
