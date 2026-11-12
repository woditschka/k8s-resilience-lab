//go:build !integration

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

// The test binary re-executes itself as a run under the command's signal
// handling, selected by this variable, since a signal's default handling can
// only be observed on a process it ends.
const (
	theChildRole                  = "LAB_TEST_CHILD_ROLE"
	aRunRemovingItsClustersSlowly = "a run removing its clusters slowly"
)

// The child reports on its standard output when it listens for a stop and
// when the stop has ended its run, so the parent signals at those moments
// rather than after a guessed delay.
const (
	listeningForAStop = "listening for a stop"
	removingClusters  = "removing its Simulated Clusters"
)

const (
	// Longer than any test waits, so only a signal ends the removal.
	aRemovalThatDoesNotEnd = time.Hour
	// Well past what the end of a process takes and well short of a removal.
	aSecondStopEndsTheRunWithin = 10 * time.Second
)

func init() {
	if os.Getenv(theChildRole) == aRunRemovingItsClustersSlowly {
		aRunRemovingItsClustersAfterTheStop()
	}
}

func aRunRemovingItsClustersAfterTheStop() {
	stopped, stopListening := stoppedBySignal(context.Background())
	defer stopListening()

	fmt.Println(listeningForAStop)
	<-stopped.Done()
	fmt.Println(removingClusters)
	time.Sleep(aRemovalThatDoesNotEnd)
	os.Exit(1)
}

type aChildRun struct {
	command *exec.Cmd
	lines   *bufio.Scanner
}

// aStoppedRunStillRemovingItsClusters returns once the child's run has taken
// the first stop and is removing its clusters.
func aStoppedRunStillRemovingItsClusters(t *testing.T) aChildRun {
	t.Helper()

	bounded, cancel := context.WithTimeout(t.Context(), aSecondStopEndsTheRunWithin)
	t.Cleanup(cancel)
	command := exec.CommandContext(bounded, os.Args[0], "-test.run=^$")
	command.Env = append(os.Environ(), theChildRole+"="+aRunRemovingItsClustersSlowly)
	output, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("read the child run's output: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start the child run: %v", err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	child := aChildRun{command: command, lines: bufio.NewScanner(output)}

	child.awaitLine(t, listeningForAStop)
	child.stop(t)
	child.awaitLine(t, removingClusters)

	return child
}

func (c aChildRun) awaitLine(t *testing.T, wanted string) {
	t.Helper()

	for c.lines.Scan() {
		if c.lines.Text() == wanted {
			return
		}
	}
	t.Fatalf("the child run ended before printing %q: %v", wanted, c.lines.Err())
}

func (c aChildRun) stop(t *testing.T) {
	t.Helper()

	if err := c.command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("stop the child run: %v", err)
	}
}

// endedBy is the signal that ended the child: the bound's kill where the
// second stop did not end it, and none where it exited of its own accord.
func (c aChildRun) endedBy(t *testing.T) syscall.Signal {
	t.Helper()

	exited, ok := errors.AsType[*exec.ExitError](c.command.Wait())
	if !ok {
		t.Fatal("the child run removes its clusters until a signal ends it, but it ended of its own accord")
	}
	status, _ := exited.Sys().(syscall.WaitStatus)
	if !status.Signaled() {
		return noSignal
	}

	return status.Signal()
}

const noSignal syscall.Signal = 0

func TestASecondStopWhileTheRunRemovesItsClusters_ShouldEndTheRunAtOnce(t *testing.T) {
	t.Parallel()
	removing := aStoppedRunStillRemovingItsClusters(t)

	removing.stop(t)

	expect.Expect(t, removing.endedBy(t)).To(expect.Equal(syscall.SIGINT))
}
