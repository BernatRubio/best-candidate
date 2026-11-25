package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// retry runs fn up to attempts times, sleeping base, 2*base, 4*base, ... between
// tries while transient(err) holds. It stops early when ctx is cancelled and
// returns the last error otherwise. An error with a RetryAfter() duration above
// zero (a server's Retry-After) replaces the computed wait. notify (optional) is
// told before each wait.
func retry[T any](ctx context.Context, attempts int, base time.Duration, transient func(error) bool,
	notify func(err error, wait time.Duration), fn func() (T, error)) (T, error) {
	var zero T
	var err error
	for i := 0; i < attempts; i++ {
		var out T
		out, err = fn()
		if err == nil {
			return out, nil
		}
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		if !transient(err) || i == attempts-1 {
			return zero, err
		}
		wait := base << i
		var ra interface{ RetryAfter() time.Duration }
		if errors.As(err, &ra) && ra.RetryAfter() > 0 {
			wait = ra.RetryAfter()
		}
		if notify != nil {
			notify(err, wait)
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(wait):
		}
	}
	return zero, err
}

// stderrNotify prints a one-line retry notice for the command line user.
func stderrNotify(name string, attempts int) func(error, time.Duration) {
	n := 0
	return func(err error, wait time.Duration) {
		n++
		fmt.Fprintf(os.Stderr, "%s is busy (%v); retrying in %s (%d/%d)\n", name, shorten(err), wait, n, attempts-1)
	}
}

func shorten(err error) string {
	s := err.Error()
	if len(s) > 90 {
		s = s[:90] + "..."
	}
	return s
}
