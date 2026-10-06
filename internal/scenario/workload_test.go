package scenario_test

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

const (
	// someWorkloadName and somePodCount are required to build a Workload but no
	// test below turns on either value.
	someWorkloadName = "web"
	somePodCount     = 9

	// theRequiredCapacity is a minimum such a Workload could keep Running: fewer
	// Pods than it declares, so it is one the constructor admits.
	theRequiredCapacity = somePodCount - 3

	// noMinimumDeclared is what a Workload that declared none reports, and
	// noMinimumAtAll the Pod count that goes with it.
	noMinimumDeclared = false
	noMinimumAtAll    = 0
)

// aWorkload is the Workload every test below starts from: nothing about its name,
// its Pods or its specification matters, and it declares no Required Capacity.
func aWorkload(t *testing.T) scenario.Workload {
	t.Helper()

	workload, err := scenario.NewWorkload(someWorkloadName, somePodCount, aPodSpecification())
	if err != nil {
		t.Fatalf("build a Workload of %d Pods: %v", somePodCount, err)
	}

	return workload
}

func TestAWorkload_ShouldKeepThePodSpecificationItDeclares(t *testing.T) {
	declared := aPodSpecification()

	workload, err := scenario.NewWorkload(someWorkloadName, somePodCount, declared)
	if err != nil {
		t.Fatalf("build a Workload of %d Pods: %v", somePodCount, err)
	}

	expect.Expect(t, workload.PodSpecification()).To(expect.Equal(declared))
}

func TestAWorkloadDeclaringARequiredCapacity_ShouldKeepItAsTheFewestPodsItMustKeepRunning(t *testing.T) {
	declaring := aWorkload(t)

	workload, err := declaring.WithRequiredCapacity(theRequiredCapacity)
	if err != nil {
		t.Fatalf("declare a Required Capacity of %d Pods: %v", theRequiredCapacity, err)
	}

	minimum, declared := workload.RequiredCapacity()
	expect.Expect(t, minimum).To(expect.Equal(theRequiredCapacity))
	expect.Expect(t, declared).To(expect.Equal(true))
}

func TestAWorkloadDeclaringNoRequiredCapacity_ShouldReportThatItDeclaredNone(t *testing.T) {
	minimum, declared := aWorkload(t).RequiredCapacity()

	expect.Expect(t, minimum).To(expect.Equal(noMinimumAtAll))
	expect.Expect(t, declared).To(expect.Equal(noMinimumDeclared))
}

// A minimum the Workload could never keep is refused where it is declared, which
// is docs/prd.md REQ-SIM-010 edge cases 1 and 2.
func TestARequiredCapacityNoWorkloadCouldKeep_ShouldBeRefused(t *testing.T) {
	refusals := map[string]struct {
		minimum int
		named   string
	}{
		"above the Pods the Workload declares": {minimum: somePodCount + 1, named: "Pods"},
		"of no Pod at all":                     {minimum: 0, named: "Pod"},
		"of a negative Pod":                    {minimum: -1, named: "Pod"},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := aWorkload(t).WithRequiredCapacity(refusal.minimum)

			if err == nil {
				t.Fatalf("a Required Capacity of %d Pods is refused, but it was admitted", refusal.minimum)
			}
			if !strings.Contains(err.Error(), refusal.named) {
				t.Errorf("the refusal names %q, but reads: %v", refusal.named, err)
			}
		})
	}
}

// Two Workloads differing in the minimum alone are not the same Workload, because
// a Required Capacity is part of what one declares.
func TestTwoWorkloadsDifferingInTheirRequiredCapacityAlone_ShouldNotBeEqual(t *testing.T) {
	declaring := aWorkload(t)

	workload, err := declaring.WithRequiredCapacity(theRequiredCapacity)
	if err != nil {
		t.Fatalf("declare a Required Capacity of %d Pods: %v", theRequiredCapacity, err)
	}

	expect.Expect(t, workload.Equal(declaring)).To(expect.Equal(false))
}

func TestAWorkload_ShouldRefuseADeclarationItCannotDeploy(t *testing.T) {
	refusals := map[string]struct {
		name      string
		pods      int
		declaring scenario.PodSpecification
		named     string
	}{
		"no name":        {name: "", pods: somePodCount, declaring: aPodSpecification(), named: "name"},
		"no Pod at all":  {name: someWorkloadName, pods: 0, declaring: aPodSpecification(), named: "Pod"},
		"a negative Pod": {name: someWorkloadName, pods: -1, declaring: aPodSpecification(), named: "Pod"},
		"a specification that sets nothing": {
			name: someWorkloadName, pods: somePodCount,
			declaring: scenario.NewPodSpecification(someSpecificationName), named: "setting",
		},
		"no specification at all": {
			name: someWorkloadName, pods: somePodCount,
			declaring: scenario.PodSpecification{}, named: "setting",
		},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := scenario.NewWorkload(refusal.name, refusal.pods, refusal.declaring)

			if err == nil {
				t.Fatalf("a Workload named %q of %d Pods is refused, but it was admitted", refusal.name, refusal.pods)
			}
			if !strings.Contains(err.Error(), refusal.named) {
				t.Errorf("the refusal names %q, but reads: %v", refusal.named, err)
			}
		})
	}
}
