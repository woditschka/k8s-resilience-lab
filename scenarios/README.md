# Scenarios

The lab's experiments. Each Scenario is one file here, holding one exported Go function whose every
line is a Given, When or Then Step. Nothing of the machinery that runs them lives here: the Step
surface and the runner are `internal/lab`.

Two files sit beside them. `scenarios.go` holds `All`, the list of the Scenarios a run carries out.
`workloads.go` holds the names of the Workloads they deploy, one constant each.

Run them all with `go run ./cmd/lab`.

## Resilient placement

`resilient_placement.go`, running as `resilient-placement`

| | |
|---|---|
| Given | three Zones of three Nodes |
| Given | the `Checkout` Workload of nine Pods, declaring Resilient placement |
| Then | every Pod is Running |
| Then | three Pods sit in every Zone |
| Then | one Pod sits on every Node |

The Scenario the lab starts from: with room on every Node for more than one Pod, an even Placement
is the scheduler honouring the specification the Workload declares rather than the Nodes running out
of space.

## Node loss

`node_loss.go`, running as `node-loss`

| | |
|---|---|
| Given | three Zones of three Nodes |
| Given | the `Checkout` Workload of nine Pods, declaring Resilient placement, keeping at least six Pods Running |
| Then | every Pod is Running |
| Then | three Pods sit in every Zone |
| When | one Node fails |
| Then | every Pod is Running |
| Then | three Pods sit in every Zone |
| Then | no Pod sits on the Nodes that failed |
| Then | the Required Capacity held: eight Pods at the moment of the Failure, nine once Settled |

The Resilient placement Scenario's world, and then one Node stops responding. The two Thens before
the When judge the Placement the Scenario starts from, so the world the Failure is injected into is
judged rather than assumed and a Finding there points at the starting Placement instead of at the
recovery. The When says how many Nodes fail rather than naming one: the lab picks the first Nodes the
declared Topology derives, the same pick on every run, and reports which in the Step's own line. The
Thens after it judge the Placement the control plane reached next.

The Workload declares the capacity it has to keep through the Failure, and the last Then judges that
minimum at two moments: the moment the Failure was injected, and the moment the Placement had settled
again. One Pod sits on every Node here, so eight are left Running at the first and nine at the
second, both above the six declared. The Step names the minimum and both counts whether they are met
or not, and a moment that misses is reported as a Finding naming it.

Nothing here moves a Pod. Noticing the Node, tainting it and replacing its Pods on the Nodes still
responding are the control plane's own doing, which is what makes the judged Placement an observation
about Kubernetes rather than about the lab. The lab does shorten one wait: a Pod leaves an
unresponsive Node after seconds rather than the five minutes a default cluster allows, and every run
reports that setting at its start.

## Zone loss

`zone_loss.go`, running as `zone-loss`

| | |
|---|---|
| Given | three Zones of three Nodes |
| Given | the `Checkout` Workload of nine Pods, declaring Resilient placement, keeping at least six Pods Running |
| Then | every Pod is Running |
| Then | three Pods sit in every Zone |
| When | one Zone fails |
| Then | every Pod is Running |
| Then | no Pod sits in the Zones that failed |
| Then | the Required Capacity held: six Pods at the moment of the Failure, nine once Settled |

The same world, and then every Node of one Zone stops responding at once. The When says how many
Zones fail rather than naming one: the lab picks the first Zones the declared Topology derives, the
same pick on every run, and reports which in the Step's own line.

What is judged after the Failure is the capacity and nothing else — nine Pods Running, none of them
in the Zone that stopped. No count per Zone is judged there, because two Zones take nine Pods: the
Placement is skewed by one Pod per Zone and two Nodes carry two. That is the impact on reliability a
lost Zone is accepted to have, and asserting an even spread would assert something Kubernetes cannot
deliver. Both spread constraints honour the taints the stopped Nodes carry, which is why the Zone
that failed leaves the spread calculation rather than holding Pods Pending.

The Workload declares the same minimum of six Pods, and a lost Zone is what tests it: the Zone takes
three Pods with it, so exactly six are left Running at the moment it stops and all nine once the
Placement has settled. That is the property the lab is for — the capacity survives a lost Zone — and
the last Then states both counts beside the minimum.

The lab lifts one more control-plane setting for this Scenario. A cluster taints the unresponsive
Nodes of one Zone at a tenth per second, which would stretch a Zone Failure over half a minute, so
every cluster the lab provisions runs at ten per second and each run reports that setting at its
start. What is left to wait for is the period the control plane takes to notice, which is Kubernetes'
own.

## Writing one

A Step declares or judges; it never handles an error. A Given constructs the Topology or the
Workload and names itself if that declaration is refused. A Then judges the Settled Placement, and
one it does not meet is reported as a Finding while the later Steps still run.

```go
func AWorkloadPlacedForResilience_ShouldPlaceThreePodsPerZoneAndOnePerNode(s *lab.Scenario) {
	s.Given().Topology(lab.Zones(3), lab.NodesPerZone(3))
	s.Given().Workload(Checkout, lab.Pods(9), lab.ResilientPlacement())
	s.Then().AllPodsRunning()
}
```

The function is named for the condition it runs under and the outcome it asserts,
`<Condition>_Should<Outcome>`. The Scenario runs under the stem of its file, with hyphens for
underscores, so renaming the file renames the Scenario. A new Scenario is a file here and one line
in `scenarios.go`.

A Workload declares one of the lab's predefined Pod specifications and no setting of its own:

| | |
|---|---|
| `lab.ResilientPlacement()` | its Pods spread as evenly over the Zones and over the Nodes as the control plane can place them |
| `lab.DefaultPlacement()` | no spread at all, so where its Pods land is the scheduler's own choice |

Each one is defined in `internal/lab/pod_specification.go` as the settings it sets, under
Kubernetes' own names, over a base that carries the lab's own — the tolerations that make a Pod leave
an unresponsive Node after the lab's waiting time. Open a definition to read which setting it sets
and to which value. A new specification is one function there, added with the Scenario that needs it.

A Scenario's mandatory values are passed as named arguments — `lab.Zones`, `lab.NodesPerZone`,
`lab.Pods`, `lab.WorkloadName` and the specification — so a swapped argument does not compile. An
optional setting is a further call on the complete declaration,
`Topology(lab.Zones(3), lab.NodesPerZone(3)).NodeCapacity(lab.Pods(2))` for the Pods a Node holds and
`Workload(Checkout, lab.Pods(9), lab.ResilientPlacement()).RequiredCapacity(lab.Pods(6))` for the
fewest Pods the Workload has to keep Running through a Failure. The lab's Scenarios name the
Workload constants of `workloads.go`, which are business capabilities of the domain they model — an
online shop in the Scenario above, and any domain the next one needs.

A Scenario has no test of its own: it is itself the check, and a test beside it could only repeat its
own Then Steps. The lab's Scenarios run in `cmd/lab`'s own suite, which fails when one reports a
Finding or ends at a Step.
