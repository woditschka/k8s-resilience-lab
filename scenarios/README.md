# Scenarios

The lab's experiments. Each Scenario is one file here, holding one exported Go function whose every
line is a Given, When or Then Step. Nothing of the machinery that runs them lives here: the Step
surface and the runner are `internal/lab`.

Two files sit beside them. `scenarios.go` holds `All`, the list of the Scenarios a run carries out.
`workloads.go` holds the names of the Workloads they deploy, one constant each.

Run them all with `go run ./cmd/lab`.

## Even spread

`even_spread.go`, running as `even-spread`

| | |
|---|---|
| Given | three Zones of three Nodes |
| Given | the `Checkout` Workload of nine Pods, spread evenly over the Zones and over the Nodes |
| Then | every Pod is Running |
| Then | three Pods sit in every Zone |
| Then | one Pod sits on every Node |

The Scenario the lab starts from: with room on every Node for more than one Pod, an even Placement
is the scheduler honouring the Workload's declared spread rather than the Nodes running out of
space.

## Writing one

A Step declares or judges; it never handles an error. A Given constructs the Topology or the
Workload and names itself if that declaration is refused. A Then judges the Settled Placement, and
one it does not meet is reported as a Finding while the later Steps still run.

```go
func AnEvenlySpreadWorkload_ShouldPlaceThreePodsPerZoneAndOnePerNode(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), lab.NodesPerZone(3))
	s.Then().AllPodsRunning()
}
```

The function is named for the condition it runs under and the outcome it asserts,
`<Condition>_Should<Outcome>`. The Scenario runs under the stem of its file, with hyphens for
underscores, so renaming the file renames the Scenario. A new Scenario is a file here and one line
in `scenarios.go`.

A Scenario's mandatory values are passed as named arguments — `lab.Zones`, `lab.NodesPerZone`,
`lab.Pods` and `lab.WorkloadName` — so a swapped argument does not compile. An optional setting is a
further call on the complete declaration,
`Topology(lab.Zones(3), lab.NodesPerZone(3)).NodeCapacity(lab.Pods(2))`. The lab's Scenarios name the
Workload constants of `workloads.go`, which are business capabilities of the domain they model — an
online shop in the Scenario above, and any domain the next one needs.

A Scenario has no test of its own: it is itself the check, and a test beside it could only repeat its
own Then Steps. The lab's Scenarios run in `cmd/lab`'s own suite, which fails when one reports a
Finding or ends at a Step.
