// Command lab runs the k8s-resilience-lab Scenarios, each on a Simulated
// Cluster of its own that is removed when the Scenario ends, and reports where
// each Workload's Pods landed.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/woditschka/k8s-resilience-lab/internal/cluster"
	"github.com/woditschka/k8s-resilience-lab/internal/lab"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
	"github.com/woditschka/k8s-resilience-lab/internal/timeline"
	"github.com/woditschka/k8s-resilience-lab/scenarios"
)

func main() {
	// The working directory is the one input a run takes from the process
	// rather than from a flag.
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "lab: resolve the directory this run files its Timelines under: %v\n", err)
		os.Exit(1)
	}

	if err := run(context.Background(), os.Args[1:], root, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "lab: %v\n", err)
		os.Exit(1)
	}
}

// run takes its output, arguments and Timeline directory as parameters so that
// main is the exit-status boundary alone.
func run(ctx context.Context, arguments []string, timelinesRoot string, report, warnings io.Writer) error {
	asked, err := resolveSetting(arguments, warnings)
	if err != nil {
		return err
	}

	engine, err := cluster.DetectEngine(ctx, slog.New(slog.NewTextHandler(warnings, nil)))
	if err != nil {
		return err
	}
	// Every Scenario's provisioning and polling shares this one client.
	defer engine.Close()

	reported := &runReport{to: report}
	var opening reportPart
	opening.addf("engine socket:      %s", engine.Socket())
	opening.addf("engine version:     %s", engine.EngineVersion())
	opening.addf("Kubernetes version: %s", asked.version)
	// No total is derived from these settings: what a run waits for after a
	// Failure is the control plane's to decide, and these are only the parts
	// the lab sets.
	opening.addf("unresponsive Node:  its Pods leave after %s, where a default cluster waits %s",
		scenario.UnresponsiveNodeWait, scenario.DefaultUnresponsiveNodeWait)
	opening.addf("Node heartbeats:    at factor %s, where this cluster image runs them at factor %s",
		cluster.NodeHeartbeatFactor, cluster.DefaultNodeHeartbeatFactor)
	opening.addf("Node eviction rate: %s unresponsive Nodes of a Zone per second, where a default cluster taints %s",
		cluster.NodeEvictionRate, cluster.DefaultNodeEvictionRate)
	opening.addf("lab waiting time:   at most %s for a state of the cluster, and the most a Wait may state",
		scenario.WaitingTime)
	// The opening lines print before any Scenario runs, so a reader sees the
	// settings while the run is under way.
	reported.printPart(opening)

	// The command names no Scenario, so adding one touches the Scenarios' place
	// alone.
	//
	// The Scenarios run at once and one Scenario's result stops no other. The
	// report is written after all have ended, from this goroutine alone, so it
	// needs no locking.
	runner := lab.NewRunner(engine, asked.version, timeline.NewFolder(timelinesRoot))
	carried, err := runner.RunAtOnce(ctx, scenarios.All())
	if err != nil {
		return errors.Join(err, reported.err)
	}

	return errors.Join(writeRun(reported, carried, asked), reported.err)
}

type runSetting struct {
	version       cluster.KubernetesVersion
	wholeTimeline bool
}

func resolveSetting(arguments []string, warnings io.Writer) (runSetting, error) {
	flags := flag.NewFlagSet("lab", flag.ContinueOnError)
	flags.SetOutput(warnings)
	named := flags.String("k8s-version", "", "the Kubernetes version each Simulated Cluster mirrors; the lab's pinned default when absent")
	whole := flags.Bool("whole-timeline", false, "print every entry of each Scenario's Timeline under its filing line")
	if err := flags.Parse(arguments); err != nil {
		return runSetting{}, fmt.Errorf("read the lab's arguments: %w", err)
	}

	version, err := resolveVersion(*named)
	if err != nil {
		return runSetting{}, err
	}

	return runSetting{version: version, wholeTimeline: *whole}, nil
}

// resolveVersion falls back to the lab's pinned default when no version is
// named.
func resolveVersion(named string) (cluster.KubernetesVersion, error) {
	if named == "" {
		return cluster.DefaultKubernetesVersion(), nil
	}

	return cluster.ParseKubernetesVersion(named)
}

// writeRun returns an error for any Scenario that was not carried out, left a
// Step undone, reported a Finding or lost its Timeline, so the run exits
// non-zero.
func writeRun(report *runReport, carried []lab.Carried, asked runSetting) error {
	var failures []error
	for _, each := range carried {
		var part reportPart
		writeCarried(&part, each, asked)
		report.printBelowAHeavierSeparator(part)
		failures = append(failures, each.Err, notFiled(each.Outcome), didNotHold(each.Outcome))
	}

	return errors.Join(failures...)
}

func writeCarried(part *reportPart, carried lab.Carried, asked runSetting) {
	if !carried.Reported() {
		part.addf("Scenario %s, ended before it reported: %v", carried.Outcome.Name, carried.Err)

		return
	}

	writeOutcome(part, carried.Outcome, asked)
}

// A Scenario that ended at a Step still has its Timeline reported, because that
// record shows how far its cluster got.
func writeOutcome(part *reportPart, outcome lab.Outcome, asked runSetting) {
	part.addf("Scenario %s, on %s", outcome.Name, outcome.Cluster)
	writeSteps(part, outcome)
	if outcome.HoldsASettledPlacement() {
		part.addf("  Skew of %s: %d over Zones, %d over Nodes", outcome.JudgedWorkload,
			outcome.Placement.Skew(scenario.SpreadOverZones), outcome.Placement.Skew(scenario.SpreadOverNodes))
	}
	writeTimeline(part, outcome, asked)
}

func writeTimeline(part *reportPart, outcome lab.Outcome, asked runSetting) {
	filing := timelineFiling(outcome)
	part.addf("  %s", separatorOf(utf8.RuneCountInString(filing)))
	part.addf("  %s", filing)
	if !asked.wholeTimeline {
		return
	}

	for _, entry := range outcome.Timeline.Entries() {
		part.addf("    %s", entry.Line())
	}
}

func timelineFiling(outcome lab.Outcome) string {
	held := len(outcome.Timeline.Entries())
	if outcome.NotFiled != nil {
		return fmt.Sprintf("Timeline, %d entries, filed nowhere: %v", held, outcome.NotFiled)
	}

	return fmt.Sprintf("Timeline, %d entries, filed at %s", held, outcome.FiledAt)
}

// A lost Timeline fails the run while the Scenario's own outcome stands as
// reported.
func notFiled(outcome lab.Outcome) error {
	if outcome.NotFiled == nil {
		return nil
	}

	return fmt.Errorf("the Timeline of the Scenario %s was not filed: %w", outcome.Name, outcome.NotFiled)
}

// A Workload's settings are reported one level deeper, so what a Named setting
// stands for stays visible.
func writeSteps(part *reportPart, outcome lab.Outcome) {
	for _, step := range outcome.Steps {
		part.addf("  %s", step)
		for _, setting := range step.Settings {
			part.addf("    %s", setting)
		}
		if step.Kind == lab.WaitStep {
			writeBoard(part, step)
		}
	}

	for _, finding := range outcome.Findings {
		part.addf("  Finding, %s: %s", finding.Step, finding.Mismatch)
	}
}

// The narrative stands above the separator, which spans the Wait's line, the
// narrative and the board, so it reads as one rule under all three. The second
// separator closes the board whatever follows, so a board never runs into the
// next Step.
func writeBoard(part *reportPart, wait lab.Step) {
	for _, line := range wait.Narrative.Lines() {
		part.addf("  %s", line)
	}
	separator := separatorOf(max(utf8.RuneCountInString(wait.String()), wait.Narrative.Width(), wait.Board.Width()))
	part.addf("  %s", separator)
	if wait.BoardNotObserved != nil {
		part.addf("  no Placement board: %v", wait.BoardNotObserved)
	}
	for _, line := range wait.Board.Lines() {
		part.addf("  %s", line)
	}
	part.addf("  %s", separator)
}

const (
	separatorRune = "-"
	// The heavier separator divides whole parts of the run, so it takes a rune
	// the separators inside a Scenario do not.
	heavierSeparatorRune = "="
)

func separatorOf(width int) string {
	return strings.Repeat(separatorRune, width)
}

func didNotHold(outcome lab.Outcome) error {
	if outcome.Held() {
		return nil
	}
	if outcome.Ended() {
		return fmt.Errorf("the Scenario %s ended at %s: %w", outcome.Name, outcome.EndedAt, outcome.Cause)
	}

	found := make([]string, 0, len(outcome.Findings))
	for _, finding := range outcome.Findings {
		found = append(found, fmt.Sprintf("%s: %s", finding.Step, finding.Mismatch))
	}

	return fmt.Errorf("the Scenario %s did not hold:\n  %s", outcome.Name, strings.Join(found, "\n  "))
}

// reportPart is one part the heavier separator divides: the run's opening lines
// or one Scenario. A part is held whole until it is printed, because the
// separator above it must reach its widest line.
type reportPart []string

func (p *reportPart) addf(format string, args ...any) {
	*p = append(*p, fmt.Sprintf(format, args...))
}

func (p reportPart) width() int {
	widest := 0
	for _, line := range p {
		widest = max(widest, utf8.RuneCountInString(line))
	}

	return widest
}

// runReport keeps the first write error and stops writing after it, so a closed
// stdout is reported once rather than checked at every line or silently
// dropped.
type runReport struct {
	to  io.Writer
	err error
	// lastWidth is the widest line of the part printed last, which the next
	// heavier separator must reach as well as its own part's.
	lastWidth int
}

func (r *runReport) printPart(part reportPart) {
	for _, line := range part {
		r.printf("%s\n", line)
	}
	r.lastWidth = part.width()
}

func (r *runReport) printBelowAHeavierSeparator(part reportPart) {
	r.printf("%s\n", strings.Repeat(heavierSeparatorRune, max(r.lastWidth, part.width())))
	r.printPart(part)
}

func (r *runReport) printf(format string, args ...any) {
	if r.err != nil {
		return
	}

	if _, err := fmt.Fprintf(r.to, format, args...); err != nil {
		r.err = fmt.Errorf("write the run's report: %w", err)
	}
}
