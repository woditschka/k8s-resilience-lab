package scenario_test

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// No test below turns on someDeploymentName or someReplicas.
	someDeploymentName = "web"
	someReplicas       = 9

	noToleranceDeclared = false
)

// Three Zones of three Nodes, so a Zone and a Node hold different shares of an
// even spread.
const (
	zonesOfTheTopology        = 3
	nodesPerZoneOfTheTopology = 3
	nodesOfTheTopology        = zonesOfTheTopology * nodesPerZoneOfTheTopology

	nineReplicas = 9
	// tenReplicas leave one Pod over an even spread, which a lost Zone may hold.
	tenReplicas = nineReplicas + 1

	oneLost     = 1
	noLossAtAll = 0
)

func aDeployment(t *testing.T) scenario.Deployment {
	t.Helper()

	return aDeploymentOf(t, someReplicas)
}

func aDeploymentOf(t *testing.T, replicas int) scenario.Deployment {
	t.Helper()

	deployment, err := scenario.NewDeployment(someDeploymentName, replicas, aPodSpecification(t))
	if err != nil {
		t.Fatalf("build a Deployment of %d replicas: %v", replicas, err)
	}

	return deployment
}

func aTopologyOfThreeZonesOfThreeNodes(t *testing.T) scenario.Topology {
	t.Helper()

	topology, err := topologyOfTheGeneralNodePool(zonesOfTheTopology, nodesPerZoneOfTheTopology, aCapacity(t))
	if err != nil {
		t.Fatalf("build a Topology of %d Zones of %d Nodes: %v", zonesOfTheTopology, nodesPerZoneOfTheTopology, err)
	}

	return topology
}

func TestADeployment_ShouldKeepThePodSpecificationItDeclares(t *testing.T) {
	declared := aPodSpecification(t)

	deployment, err := scenario.NewDeployment(someDeploymentName, someReplicas, declared)
	if err != nil {
		t.Fatalf("build a Deployment of %d replicas: %v", someReplicas, err)
	}

	expect.Expect(t, deployment.PodSpecification()).To(expect.Equal(declared))
}

func TestADeploymentSurvivingTheLossOfZonesOrNodes_ShouldKeepAFloorOfItsReplicasLessTheMostPodsThoseHoldEvenlySpread(t *testing.T) {
	cases := map[string]struct {
		replicas  int
		tolerance scenario.FailureTolerance
		floor     int
	}{
		"nine replicas surviving one Zone": {
			replicas:  nineReplicas,
			tolerance: scenario.FailureTolerance{Lost: scenario.ZonesLost, Count: oneLost},
			floor:     nineReplicas - nineReplicas/zonesOfTheTopology,
		},
		"nine replicas surviving one Node": {
			replicas:  nineReplicas,
			tolerance: scenario.FailureTolerance{Lost: scenario.NodesLost, Count: oneLost},
			floor:     nineReplicas - nineReplicas/nodesOfTheTopology,
		},
		"ten replicas surviving one Zone holding the Pod over": {
			replicas:  tenReplicas,
			tolerance: scenario.FailureTolerance{Lost: scenario.ZonesLost, Count: oneLost},
			floor:     tenReplicas - (tenReplicas/zonesOfTheTopology + 1),
		},
		"nine replicas surviving the loss of no Zone": {
			replicas:  nineReplicas,
			tolerance: scenario.FailureTolerance{Lost: scenario.ZonesLost, Count: noLossAtAll},
			floor:     nineReplicas,
		},
	}

	for named, one := range cases {
		t.Run(named, func(t *testing.T) {
			deployment, err := aDeploymentOf(t, one.replicas).
				WithFailureTolerance(one.tolerance, aTopologyOfThreeZonesOfThreeNodes(t))
			if err != nil {
				t.Fatalf("declare a Failure tolerance of %s: %v", one.tolerance.Reads(), err)
			}

			declared, declares := deployment.FailureTolerance()
			expect.Expect(t, deployment.Floor()).To(expect.Equal(one.floor))
			expect.Expect(t, declared).To(expect.Equal(one.tolerance))
			expect.Expect(t, declares).To(expect.Equal(true))
		})
	}
}

func TestADeploymentDeclaringNoFailureTolerance_ShouldKeepEveryReplicaAsItsFloorAndReportThatItDeclaredNone(t *testing.T) {
	deployment := aDeployment(t)

	_, declares := deployment.FailureTolerance()
	expect.Expect(t, deployment.Floor()).To(expect.Equal(someReplicas))
	expect.Expect(t, declares).To(expect.Equal(noToleranceDeclared))
}

func TestAFailureToleranceNoDeploymentCouldJudge_ShouldBeRefused(t *testing.T) {
	const reachingEveryZone, reachingEveryNode, belowZero = zonesOfTheTopology, nodesOfTheTopology, -1
	const aLossOfNeither scenario.ToleratedLoss = "Region"
	refusals := map[string]struct {
		tolerance scenario.FailureTolerance
		against   scenario.Topology
		named     string
	}{
		"every Zone of the Topology": {
			tolerance: scenario.FailureTolerance{Lost: scenario.ZonesLost, Count: reachingEveryZone},
			against:   aTopologyOfThreeZonesOfThreeNodes(t), named: "3 Zones",
		},
		"every Node of the Topology": {
			tolerance: scenario.FailureTolerance{Lost: scenario.NodesLost, Count: reachingEveryNode},
			against:   aTopologyOfThreeZonesOfThreeNodes(t), named: "9 Nodes",
		},
		"below zero": {
			tolerance: scenario.FailureTolerance{Lost: scenario.ZonesLost, Count: belowZero},
			against:   aTopologyOfThreeZonesOfThreeNodes(t), named: "zero",
		},
		"against no Topology": {
			tolerance: scenario.FailureTolerance{Lost: scenario.NodesLost, Count: noLossAtAll},
			against:   scenario.Topology{}, named: "the 0 its Topology holds",
		},
		"of neither Zones nor Nodes": {
			tolerance: scenario.FailureTolerance{Lost: aLossOfNeither, Count: oneLost},
			against:   aTopologyOfThreeZonesOfThreeNodes(t), named: "Zones or Nodes",
		},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := aDeployment(t).WithFailureTolerance(refusal.tolerance, refusal.against)

			if err == nil {
				t.Fatalf("a Failure tolerance of %s is refused, but it was admitted", refusal.tolerance.Reads())
			}
			if !strings.Contains(err.Error(), refusal.named) {
				t.Errorf("the refusal names %q, but reads: %v", refusal.named, err)
			}
		})
	}
}

// A Failure tolerance is part of what a Deployment declares.
func TestTwoDeploymentsDifferingInTheirFailureToleranceAlone_ShouldNotBeEqual(t *testing.T) {
	declaring := aDeployment(t)

	deployment, err := declaring.WithFailureTolerance(
		scenario.FailureTolerance{Lost: scenario.ZonesLost, Count: noLossAtAll}, aTopologyOfThreeZonesOfThreeNodes(t))
	if err != nil {
		t.Fatalf("declare a Failure tolerance of no Zone: %v", err)
	}

	expect.Expect(t, deployment.Equal(declaring)).To(expect.Equal(false))
}

func TestADeployment_ShouldRefuseADeclarationItCannotDeploy(t *testing.T) {
	refusals := map[string]struct {
		name      string
		replicas  int
		declaring scenario.PodSpecification
		named     string
	}{
		"no name":            {name: "", replicas: someReplicas, declaring: aPodSpecification(t), named: "name"},
		"no replica at all":  {name: someDeploymentName, replicas: 0, declaring: aPodSpecification(t), named: "replica"},
		"a negative replica": {name: someDeploymentName, replicas: -1, declaring: aPodSpecification(t), named: "replica"},
		// A constructed specification always holds the lab's tolerations, so
		// only the zero value reaches the guard against a specification setting
		// nothing.
		"a specification that sets nothing": {
			name: someDeploymentName, replicas: someReplicas,
			declaring: scenario.PodSpecification{}, named: "setting",
		},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewDeployment(refusal.name, refusal.replicas, refusal.declaring)

			if err == nil {
				t.Fatalf("a Deployment named %q of %d replicas is refused, but it was admitted", refusal.name, refusal.replicas)
			}
			if !strings.Contains(err.Error(), refusal.named) {
				t.Errorf("the refusal names %q, but reads: %v", refusal.named, err)
			}
		})
	}
}
