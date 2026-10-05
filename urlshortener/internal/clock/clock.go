// Package clock provides a small abstraction over time.Now so that components
// depending on the current time can be tested deterministically.
package clock

import "time"

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// realClock delegates to time.Now.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// New returns a Clock backed by the system clock.
func New() Clock { return realClock{} }

// Fixed returns a Clock that always reports t. Intended for tests.
func Fixed(t time.Time) Clock { return fixedClock{t: t} }

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }
