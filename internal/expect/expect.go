// Package expect holds the assertion helper a Scenario and a go test suite
// share, so the lab asserts one way wherever it runs.
//
// Every assertion in this project goes through Expect, per
// docs/system-design.md § Dependency Policy § Prohibited: no assertion library
// is in the module graph.
package expect

import (
	"fmt"

	"github.com/google/go-cmp/cmp"
)

// T is the reporting surface an assertion needs. Both *testing.T and the lab
// command's Scenario runner satisfy it, which is what lets one Scenario run as a
// go test and as a step of the lab command.
type T interface {
	Helper()
	Errorf(format string, args ...any)
}

// Matcher decides whether an observed value is the expected one. A mismatch
// returns the report the assertion makes; a match returns no report.
type Matcher[V any] func(got V) (mismatch string, matched bool)

// Assertion is one observed value waiting for the Matcher to judge it.
type Assertion[V any] struct {
	t   T
	got V
}

// Expect begins an assertion over got, reported through t.
func Expect[V any](t T, got V) Assertion[V] {
	return Assertion[V]{t: t, got: got}
}

// To judges the observed value with match and reports a mismatch through T.
func (a Assertion[V]) To(match Matcher[V]) {
	a.t.Helper()

	if mismatch, matched := match(a.got); !matched {
		a.t.Errorf("%s", mismatch)
	}
}

// Equal matches a value equal to want, reporting a go-cmp diff when it is not.
func Equal[V any](want V) Matcher[V] {
	return func(got V) (string, bool) {
		diff := cmp.Diff(want, got)
		if diff == "" {
			return "", true
		}

		return "unexpected value (-want +got):\n" + diff, false
	}
}

// HaveLen matches a slice holding want elements, reporting both lengths when it
// does not.
func HaveLen[E any](want int) Matcher[[]E] {
	return func(got []E) (string, bool) {
		if len(got) == want {
			return "", true
		}

		return fmt.Sprintf("expected %d element(s), found %d", want, len(got)), false
	}
}
