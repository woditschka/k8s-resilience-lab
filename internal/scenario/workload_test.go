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
)

func TestAWorkload_ShouldKeepThePodSpecificationItDeclares(t *testing.T) {
	declared := aPodSpecification()

	workload, err := scenario.NewWorkload(someWorkloadName, somePodCount, declared)
	if err != nil {
		t.Fatalf("build a Workload of %d Pods: %v", somePodCount, err)
	}

	expect.Expect(t, workload.PodSpecification()).To(expect.Equal(declared))
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
