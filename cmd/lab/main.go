// Command lab runs the k8s-resilience-lab Scenarios against Simulated Clusters it
// provisions itself, and reports where each Workload's Pods landed.
//
// It is the composition root: it parses the one flag, resolves the container
// engine once, and provisions one Simulated Cluster per Scenario in turn,
// removing each when that Scenario ends.
package main

import (
	"context"
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
	"github.com/woditschka/k8s-resilience-lab/scenarios"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "lab: %v\n", err)
		os.Exit(1)
	}
}

// run is the whole run, with its output and its arguments given rather than
// taken from the process, so that main is the exit-status boundary alone.
func run(ctx context.Context, arguments []string, report, warnings io.Writer) error {
	version, err := resolveVersion(arguments, warnings)
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
	reported.printf("Kubernetes version: %s\n", version)

	// The Scenarios the lab holds, in the order the list beside them names: the
	// command names no Scenario, so adding one touches the Scenarios' place alone.
	// Selecting one by name is REQ-SIM-003.
	runner := lab.NewRunner(engine, version)
	for _, steps := range scenarios.All() {
		if err := runScenario(ctx, runner, steps, reported); err != nil {
			return err
		}
	}

	return reported.err
}

// resolveVersion parses the one flag the lab takes, resolving the Kubernetes
// version the whole run mirrors. One parse per run, so no later step re-reads it.
func resolveVersion(arguments []string, warnings io.Writer) (cluster.KubernetesVersion, error) {
	flags := flag.NewFlagSet("lab", flag.ContinueOnError)
	flags.SetOutput(warnings)
	named := flags.String("k8s-version", "", "the Kubernetes version each Simulated Cluster mirrors; the lab's pinned default when absent")
	if err := flags.Parse(arguments); err != nil {
		return cluster.KubernetesVersion{}, fmt.Errorf("read the lab's arguments: %w", err)
	}

	if *named == "" {
		return cluster.DefaultKubernetesVersion(), nil
	}

	return cluster.ParseKubernetesVersion(*named)
}

// runScenario runs one Scenario through the runner and reports the name it ran
// under, the Steps it carried out, what it found, and the Placement it settled
// at. The run name is the runner's to derive, so the command names no Scenario.
func runScenario(ctx context.Context, runner *lab.Runner, steps func(*lab.Scenario), report *runReport) error {
	outcome, err := runner.Run(ctx, steps)
	if err != nil {
		return err
	}

	report.printf("\nScenario %s, on %s\n", outcome.Name, outcome.Cluster)
	writeSteps(report, outcome)
	if !outcome.Ended() {
		writePlacement(report, outcome.Placement)
	}

	return didNotHold(outcome)
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
