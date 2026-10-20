// Package expect holds the assertion helper a Scenario and a go test suite
// share, so the lab asserts one way wherever it runs.
package expect

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
)

// T is satisfied by both *testing.T and the lab's Scenario runner, which lets
// one assertion run in a go test and in a Scenario.
type T interface {
	Helper()
	Errorf(format string, args ...any)
}

// Matcher returns the report a mismatch makes, and no report on a match.
type Matcher[V any] func(got V) (mismatch string, matched bool)

// Assertion holds a value until a matcher judges it, reporting a mismatch
// through the T it was made with.
type Assertion[V any] struct {
	t   T
	got V
}

func Expect[V any](t T, got V) Assertion[V] {
	return Assertion[V]{t: t, got: got}
}

func (a Assertion[V]) To(match Matcher[V]) {
	a.t.Helper()

	if mismatch, matched := match(a.got); !matched {
		a.t.Errorf("%s", mismatch)
	}
}

// Equal reports a go-cmp diff when the values differ.
func Equal[V any](want V) Matcher[V] {
	return func(got V) (string, bool) {
		diff := cmp.Diff(want, got)
		if diff == "" {
			return "", true
		}

		return "unexpected value (-want +got):\n" + diff, false
	}
}

func HaveLen[E any](want int) Matcher[[]E] {
	return func(got []E) (string, bool) {
		if len(got) == want {
			return "", true
		}

		return fmt.Sprintf("expected %d element(s), found %d", want, len(got)), false
	}
}
