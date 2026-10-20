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
	"maps"
	"os"
	"slices"
	"strings"

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
	reported.printf("engine socket:      %s\n", engine.Socket())
	reported.printf("engine version:     %s\n", engine.EngineVersion())
	reported.printf("Kubernetes version: %s\n", asked.version)
	// No total is derived from these settings: what a run waits for after a
	// Failure is the control plane's to decide, and these are only the parts
	// the lab sets.
	reported.printf("unresponsive Node:  its Pods leave after %s, where a default cluster waits %s\n",
		scenario.UnresponsiveNodeWait, scenario.DefaultUnresponsiveNodeWait)
	reported.printf("Node heartbeats:    at factor %s, where this cluster image runs them at factor %s\n",
		cluster.NodeHeartbeatFactor, cluster.DefaultNodeHeartbeatFactor)
	reported.printf("Node eviction rate: %s unresponsive Nodes of a Zone per second, where a default cluster taints %s\n",
		cluster.NodeEvictionRate, cluster.DefaultNodeEvictionRate)
	reported.printf("lab waiting time:   at most %s for a state of the cluster, and the most a Wait may state\n",
		scenario.WaitingTime)

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
	whole := flags.Bool("whole-timeline", false, "print every entry of each Scenario's Timeline under its summary")
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
		writeCarried(report, each, asked)
		failures = append(failures, each.Err, notFiled(each.Outcome), didNotHold(each.Outcome))
	}

	return errors.Join(failures...)
}

func writeCarried(report *runReport, carried lab.Carried, asked runSetting) {
	if !carried.Reported() {
		report.printf("\nScenario %s, ended before it reported: %v\n", carried.Outcome.Name, carried.Err)

		return
	}

	writeOutcome(report, carried.Outcome, asked)
}

// A Scenario that ended at a Step still has its Timeline reported, because that
// record shows how far its cluster got.
func writeOutcome(report *runReport, outcome lab.Outcome, asked runSetting) {
	report.printf("\nScenario %s, on %s\n", outcome.Name, outcome.Cluster)
	writeSteps(report, outcome)
	if !outcome.Ended() {
		writePlacement(report, outcome.Placement)
	}
	writeTimeline(report, outcome, asked)
}

func writeTimeline(report *runReport, outcome lab.Outcome, asked runSetting) {
	summarised := outcome.Timeline.Summarise()
	report.printf("  %s\n", timelineFiling(outcome, summarised))
	for _, line := range summarised.Lines() {
		report.printf("    %s\n", line)
	}

	if !asked.wholeTimeline {
		return
	}

	for _, entry := range outcome.Timeline.Entries() {
		report.printf("    %s\n", entry.Line())
	}
}

func timelineFiling(outcome lab.Outcome, summarised scenario.TimelineSummary) string {
	held := summarised.EntryCount()
	if outcome.NotFiled != nil {
		return fmt.Sprintf("Timeline, %d entries, filed nowhere: %v", held, outcome.NotFiled)
	}

	return fmt.Sprintf("Timeline, %d entries, filed at %s:", held, outcome.FiledAt)
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
func writeSteps(report *runReport, outcome lab.Outcome) {
	for _, step := range outcome.Steps {
		report.printf("  %s\n", step)
		for _, setting := range step.Settings {
			report.printf("    %s\n", setting)
		}
	}

	for _, finding := range outcome.Findings {
		report.printf("  Finding, %s: %s\n", finding.Step, finding.Mismatch)
	}
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

// Names are sorted so two runs of the same Topology read alike.
func writePlacement(report *runReport, settled scenario.Placement) {
	report.printf("  Placement settled: %d Pods, %d Running, %d Pending\n",
		len(settled.Pods()), len(settled.PodsInPhase(scenario.PodRunning)), len(settled.PodsInPhase(scenario.PodPending)))

	report.printf("  Pods per Zone (Skew %d):\n", settled.Skew(scenario.SpreadOverZones))
	writeCounts(report, settled.PodsPerZone())

	report.printf("  Pods per Node (Skew %d):\n", settled.Skew(scenario.SpreadOverNodes))
	writeCounts(report, settled.PodsPerNode())
}

func writeCounts(report *runReport, counts map[string]int) {
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		report.printf("    %-16s %d\n", name, counts[name])
	}
}

// runReport keeps the first write error and stops writing after it, so a closed
// stdout is reported once rather than checked at every line or silently
// dropped.
type runReport struct {
	to  io.Writer
	err error
}

func (r *runReport) printf(format string, args ...any) {
	if r.err != nil {
		return
	}

	if _, err := fmt.Fprintf(r.to, format, args...); err != nil {
		r.err = fmt.Errorf("write the run's report: %w", err)
	}
}
