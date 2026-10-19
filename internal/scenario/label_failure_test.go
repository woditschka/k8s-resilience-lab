package scenario_test

import (
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// twoLabelledZones is how many Zones the Topology below holds, each holding one
// Node labelled on-demand and one labelled spot.
const twoLabelledZones = 2

// How many of the Topology's Nodes an observation keeps, by the order the
// Topology derives them.
const (
	nodesBeforeTheSecondZonesSpotNode = 3
	onlyTheFirstZonesOnDemandNode     = 1
)

// aTopologyOfOnDemandAndSpotNodes is two Zones each holding a Node of a NodePool
// stating capacity-type=on-demand and a Node of one stating capacity-type=spot.
func aTopologyOfOnDemandAndSpotNodes(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := scenario.NewTopology(twoLabelledZones,
		labelledNodePool(t, onDemand, labelsOf(t, map[string]string{capacityType: onDemand})),
		labelledNodePool(t, spot, labelsOf(t, map[string]string{capacityType: spot})))
	if err != nil {
		t.Fatalf("build a Topology of on-demand and spot Nodes: %v", err)
	}

	return topology
}

// observedAsDeclared is the Placement observing every Node the Topology derives,
// carrying the labels its NodePools state, and no Pod.
func observedAsDeclared(topology scenario.Topology) scenario.Placement {
	return scenario.NewPlacement(topology.Nodes(), nil)
}

// A Failure by label stops every observed Node carrying the value, in every Zone,
// in the order the Topology derives the Nodes (docs/prd.md REQ-SIM-018).
func TestALabelFailure_ShouldStopEveryNodeCarryingThatValueInTheTopologysOrder(t *testing.T) {
	declared := aTopologyOfOnDemandAndSpotNodes(t)

	failure, err := scenario.NewLabelFailure(declared, observedAsDeclared(declared), map[string]string{capacityType: spot})

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, failure.Names()).To(expect.Equal([]string{"zone-1-node-2", "zone-2-node-2"}))
}

// A value is stopped as the control plane carries it, so a Node the observation
// no longer holds, such as one that already stopped, is not stopped again.
func TestALabelFailure_ShouldStopOnlyTheNodesTheObservationStillHolds(t *testing.T) {
	declared := aTopologyOfOnDemandAndSpotNodes(t)
	observed := scenario.NewPlacement(declared.Nodes()[:nodesBeforeTheSecondZonesSpotNode], nil)

	failure, err := scenario.NewLabelFailure(declared, observed, map[string]string{capacityType: spot})

	expect.Expect(t, err).To(expect.Equal[error](nil))
	expect.Expect(t, failure.Names()).To(expect.Equal([]string{"zone-1-node-2"}))
}

func TestALabelFailureNamingWhatNoNodePoolStatesOrNoNodeCarries_ShouldBeRefusedNamingIt(t *testing.T) {
	const (
		aValueNoNodePoolStates = "reserved"
		aKeyNoNodePoolStates   = "node-class"
	)
	declared := aTopologyOfOnDemandAndSpotNodes(t)
	withoutSpotNodes := scenario.NewPlacement(declared.Nodes()[:onlyTheFirstZonesOnDemandNode], nil)

	refusals := map[string]struct {
		observed scenario.Placement
		literal  map[string]string
		named    []string
	}{
		"a key no NodePool states": {
			observed: observedAsDeclared(declared),
			literal:  map[string]string{aKeyNoNodePoolStates: spot},
			named:    []string{aKeyNoNodePoolStates},
		},
		"a key the lab owns": {
			observed: observedAsDeclared(declared),
			literal:  map[string]string{scenario.NodePoolLabel: spot},
			named:    []string{scenario.NodePoolLabel},
		},
		"a value no NodePool states for the key": {
			observed: observedAsDeclared(declared),
			literal:  map[string]string{capacityType: aValueNoNodePoolStates},
			named:    []string{aValueNoNodePoolStates, capacityType},
		},
		"a value no observed Node still carries": {
			observed: withoutSpotNodes,
			literal:  map[string]string{capacityType: spot},
			named:    []string{spot, capacityType},
		},
		"no label": {
			observed: observedAsDeclared(declared),
			literal:  map[string]string{},
			named:    []string{"one label"},
		},
		"two labels": {
			observed: observedAsDeclared(declared),
			literal:  map[string]string{capacityType: spot, aKeyNoNodePoolStates: aValueNoNodePoolStates},
			named:    []string{"one label", aKeyNoNodePoolStates, capacityType},
		},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewLabelFailure(declared, refusal.observed, refusal.literal)

			if err == nil {
				t.Fatalf("%s is refused, but it was admitted", refused)
			}
			for _, named := range refusal.named {
				expect.Expect(t, refusalNaming(err, named)).To(expect.Equal(true))
			}
		})
	}
}
