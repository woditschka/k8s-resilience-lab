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
	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
	"github.com/woditschka/k8s-resilience-lab/internal/scenarios"
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

	// One Scenario today. Running every Scenario of the lab, and selecting one by
	// name, are REQ-SIM-002 and REQ-SIM-003.
	if err := runScenario(ctx, engine, version, scenarios.EvenSpreadName, scenarios.EvenSpread, reported); err != nil {
		return err
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

// scenarioFunc is one Scenario: a function over a Simulated Cluster provisioned
// for it, reporting what it asserted and returning the Placement it observed.
type scenarioFunc func(context.Context, *cluster.SimulatedCluster, expect.T) (scenario.Placement, error)

// runScenario provisions a Simulated Cluster for one Scenario, runs it, and
// removes that cluster whether the Scenario passed, failed or returned an error.
func runScenario(ctx context.Context, engine cluster.Endpoint, version cluster.KubernetesVersion, name string, steps scenarioFunc, report *runReport) error {
	provisioned, err := cluster.Provision(ctx, engine, version, name)
	if err != nil {
		return err
	}
	report.printf("\nScenario %s, on %s\n", name, provisioned.Name())

	// The removal is deferred here rather than left to the Scenario, so a Scenario
	// that fails part-way through its steps still loses its cluster, per
	// docs/adr/2026-10-04-cluster-per-scenario.md. The context is detached from
	// ctx so that a cancelled run still removes what it created.
	defer func() {
		if err := provisioned.Close(context.WithoutCancel(ctx)); err != nil {
			report.printf("could not remove the Simulated Cluster: %v\n", err)
		}
	}()

	var reported scenarioReport
	settled, err := steps(ctx, provisioned, &reported)
	if err != nil {
		return fmt.Errorf("the Scenario %s: %w", name, err)
	}

	writePlacement(report, settled)

	if failed := reported.failures; len(failed) > 0 {
		return fmt.Errorf("the Scenario %s did not hold:\n  %s", name, strings.Join(failed, "\n  "))
	}

	return nil
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

// scenarioReport collects what a Scenario's assertions reported, so a Scenario
// that does not hold is named with what failed and the run exits non-zero. It is
// the Scenario-runner side of the interface *testing.T satisfies in a go test.
type scenarioReport struct {
	failures []string
}

func (*scenarioReport) Helper() {}

func (r *scenarioReport) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}
