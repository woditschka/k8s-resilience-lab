// Command lab runs the k8s-resilience-lab Scenarios against Simulated Clusters it
// provisions itself, and reports where each Workload's Pods landed.
//
// It is the composition root: it parses the one flag, resolves the container
// engine once, and carries out every Scenario at once, each on a Simulated
// Cluster of its own that is removed when that Scenario ends.
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
	// The Timelines of a run collect under the directory it was started in, which
	// is the one place a run resolves from the process rather than from a flag.
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

// run is the whole run, with its output, its arguments and the directory its
// Timelines are filed under given rather than taken from the process, so that
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
	// The whole run reaches the engine through one client, so the run is what
	// releases it: every Scenario's provisioning and polling shares it.
	defer engine.Close()

	reported := &runReport{to: report}
	reported.printf("engine socket:      %s\n", engine.Socket())
	reported.printf("engine version:     %s\n", engine.EngineVersion())
	reported.printf("Kubernetes version: %s\n", asked.version)
	// Each control-plane setting the lab makes is reported against the default it
	// replaces, and no total is derived from them: what a run waits for after a
	// Failure is the control plane's to decide, and these three are only the parts
	// the lab sets (docs/adr/2026-10-05-node-failure-withholds-the-nodes-status.md).
	reported.printf("unresponsive Node:  its Pods leave after %s, where a default cluster waits %s\n",
		scenario.UnresponsiveNodeWait, scenario.DefaultUnresponsiveNodeWait)
	reported.printf("Node heartbeats:    at factor %s, where this cluster image runs them at factor %s\n",
		cluster.NodeHeartbeatFactor, cluster.DefaultNodeHeartbeatFactor)
	reported.printf("Node eviction rate: %s unresponsive Nodes of a Zone per second, where a default cluster taints %s\n",
		cluster.NodeEvictionRate, cluster.DefaultNodeEvictionRate)
	// The lab's own waiting time is no control-plane setting: it is the longest the
	// lab itself waits for a state of the cluster, and the longest time a Wait may
	// state, which is what a Wait stating more is refused against.
	reported.printf("lab waiting time:   at most %s for a state of the cluster, and the most a Wait may state\n",
		scenario.WaitingTime)
	// The lab's own Startup time is no control-plane setting either: it is the time
	// every placed Pod takes to become Ready where its Scenario states none
	// (docs/prd.md REQ-SIM-012).
	reported.printf("Pod startup time:   %s for every Pod whose Scenario states none\n",
		scenario.DefaultStartupTime)

	// The Scenarios the lab holds, in the order the list beside them names: the
	// command names no Scenario, so adding one touches the Scenarios' place alone.
	// Selecting one by name is REQ-SIM-003.
	//
	// They run at once and every one reaches its own outcome, so what one Scenario
	// found stops no other. The report is written here, after every Scenario has
	// ended and from this goroutine alone, in the order they are declared.
	runner := lab.NewRunner(engine, asked.version, timeline.NewFolder(timelinesRoot))
	carried, err := runner.RunAtOnce(ctx, scenarios.All())
	if err != nil {
		return errors.Join(err, reported.err)
	}

	return errors.Join(writeRun(reported, carried, asked), reported.err)
}

// runSetting is what the lab's arguments resolve to: the Kubernetes version
// every Simulated Cluster of the run mirrors, and whether a Scenario's whole
// Timeline is printed under its summary. The run passes it by value, so no step
// re-reads an argument (docs/architecture-principles.md § Building Blocks).
type runSetting struct {
	version       cluster.KubernetesVersion
	wholeTimeline bool
}

// resolveSetting parses the two flags the lab takes, resolving what the whole
// run mirrors and prints. One parse per run, so no later step re-reads it.
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

// resolveVersion is the Kubernetes version the run mirrors: the one its
// arguments named, or the lab's pinned default when they named none.
func resolveVersion(named string) (cluster.KubernetesVersion, error) {
	if named == "" {
		return cluster.DefaultKubernetesVersion(), nil
	}

	return cluster.ParseKubernetesVersion(named)
}

// writeRun reports every Scenario the run carried out, in the order handed
// over, with each Scenario's lines together. It names every failure among them,
// so the run exits non-zero for any of these: a Scenario the lab could not carry
// out, a Timeline it could not write, a Step left undone or a Finding. A Timeline
// the lab could not write leaves its Scenario's outcome as reported, per
// docs/adr/2026-10-05-timeline-from-the-watch-stream.md.
func writeRun(report *runReport, carried []lab.Carried, asked runSetting) error {
	var failures []error
	for _, each := range carried {
		writeCarried(report, each, asked)
		failures = append(failures, each.Err, notFiled(each.Outcome), didNotHold(each.Outcome))
	}

	return errors.Join(failures...)
}

// writeCarried reports one Scenario of the run: everything its Outcome holds
// where it reported one, and why it ended where it ended before reporting, such
// as a Simulated Cluster the engine could not start.
func writeCarried(report *runReport, carried lab.Carried, asked runSetting) {
	if !carried.Reported() {
		report.printf("\nScenario %s, ended before it reported: %v\n", carried.Outcome.Name, carried.Err)

		return
	}

	writeOutcome(report, carried.Outcome, asked)
}

// writeOutcome reports everything one Scenario left. A Scenario that ended at a
// Step reached no Settled Placement; its Timeline is reported all the same,
// because that record is what says how far its cluster got.
func writeOutcome(report *runReport, outcome lab.Outcome, asked runSetting) {
	report.printf("\nScenario %s, on %s\n", outcome.Name, outcome.Cluster)
	writeSteps(report, outcome)
	if !outcome.Ended() {
		writePlacement(report, outcome.Placement)
	}
	writeTimeline(report, outcome, asked)
}

// writeTimeline reports a Timeline as the summary the record reads down to,
// under the path it was filed at, and every entry beneath that summary when the
// run was asked for the whole Timeline. The summary's wording is the record's
// own (docs/adr/2026-10-05-timeline-summary-derived-from-the-record.md).
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

// timelineFiling is the line a Timeline's summary is reported under: how many
// changes the record holds and where it was filed, or why it could not be filed.
func timelineFiling(outcome lab.Outcome, summarised scenario.TimelineSummary) string {
	held := summarised.EntryCount()
	if outcome.NotFiled != nil {
		return fmt.Sprintf("Timeline, %d entries, filed nowhere: %v", held, outcome.NotFiled)
	}

	return fmt.Sprintf("Timeline, %d entries, filed at %s:", held, outcome.FiledAt)
}

// notFiled names the Timeline the lab could not write, so a run that lost a
// record exits non-zero while the Scenario's own outcome stands as reported.
func notFiled(outcome lab.Outcome) error {
	if outcome.NotFiled == nil {
		return nil
	}

	return fmt.Errorf("the Timeline of the Scenario %s was not filed: %w", outcome.Name, outcome.NotFiled)
}

// writeSteps reports every Step the Scenario carried out, in order, so the run
// reads the way the Scenario does, and every Finding under them.
func writeSteps(report *runReport, outcome lab.Outcome) {
	for _, step := range outcome.Steps {
		report.printf("  %s\n", step)
	}

	for _, finding := range outcome.Findings {
		report.printf("  Finding, %s: %s\n", finding.Step, finding.Mismatch)
	}
}

// didNotHold names what stopped a Scenario or what it found, so a run whose
// Scenario ended at a Step or reported a Finding exits non-zero.
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

// writePlacement reports a Settled Placement per Zone and per Node, in name
// order so two runs of the same Topology read alike.
func writePlacement(report *runReport, settled scenario.Placement) {
	report.printf("  Placement settled: %d Pods, %d Running, %d Pending\n",
		len(settled.Pods()), len(settled.PodsInPhase(scenario.PodRunning)), len(settled.PodsInPhase(scenario.PodPending)))

	report.printf("  Pods per Zone (Skew %d):\n", settled.Skew(scenario.SpreadOverZones))
	writeCounts(report, settled.PodsPerZone())

	report.printf("  Pods per Node (Skew %d):\n", settled.Skew(scenario.SpreadOverNodes))
	writeCounts(report, settled.PodsPerNode())
}

// writeCounts reports one count per name, in name order.
func writeCounts(report *runReport, counts map[string]int) {
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		report.printf("    %-16s %d\n", name, counts[name])
	}
}

// runReport is where a run writes what it found. It keeps the first write error
// and stops writing after it, so a closed stdout is reported once, by the run,
// rather than checked at every line or silently dropped.
type runReport struct {
	to  io.Writer
	err error
}

// printf writes one line of the report unless an earlier line already failed.
func (r *runReport) printf(format string, args ...any) {
	if r.err != nil {
		return
	}

	if _, err := fmt.Fprintf(r.to, format, args...); err != nil {
		r.err = fmt.Errorf("write the run's report: %w", err)
	}
}
