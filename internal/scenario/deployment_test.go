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

	theFailureTolerance = 3
	toleratingNoLoss    = 0

	noToleranceDeclared = false
	noToleranceAtAll    = 0
)

func aDeployment(t *testing.T) scenario.Deployment {
	t.Helper()

	deployment, err := scenario.NewDeployment(someDeploymentName, someReplicas, aPodSpecification(t))
	if err != nil {
		t.Fatalf("build a Deployment of %d replicas: %v", someReplicas, err)
	}

	return deployment
}

func TestADeployment_ShouldKeepThePodSpecificationItDeclares(t *testing.T) {
	declared := aPodSpecification(t)

	deployment, err := scenario.NewDeployment(someDeploymentName, someReplicas, declared)
	if err != nil {
		t.Fatalf("build a Deployment of %d replicas: %v", someReplicas, err)
	}

	expect.Expect(t, deployment.PodSpecification()).To(expect.Equal(declared))
}

func TestADeploymentDeclaringAFailureTolerance_ShouldKeepItAsTheMostPodsItSurvivesLosing(t *testing.T) {
	tolerances := map[string]int{
		"of some of its replicas": theFailureTolerance,
		"of no Pod":               toleratingNoLoss,
	}

	for named, tolerance := range tolerances {
		t.Run(named, func(t *testing.T) {
			deployment, err := aDeployment(t).WithFailureTolerance(tolerance)
			if err != nil {
				t.Fatalf("declare a Failure tolerance of %d Pods: %v", tolerance, err)
			}

			most, declared := deployment.FailureTolerance()
			expect.Expect(t, most).To(expect.Equal(tolerance))
			expect.Expect(t, declared).To(expect.Equal(true))
		})
	}
}

func TestADeploymentDeclaringNoFailureTolerance_ShouldReportThatItDeclaredNone(t *testing.T) {
	most, declared := aDeployment(t).FailureTolerance()

	expect.Expect(t, most).To(expect.Equal(noToleranceAtAll))
	expect.Expect(t, declared).To(expect.Equal(noToleranceDeclared))
}

func TestAFailureToleranceNoDeploymentCouldJudge_ShouldBeRefused(t *testing.T) {
	refusals := map[string]struct {
		tolerance int
		named     string
	}{
		"of every replica":   {tolerance: someReplicas, named: "replicas"},
		"above the replicas": {tolerance: someReplicas + 1, named: "replicas"},
		"below zero":         {tolerance: -1, named: "zero"},
	}

	for refused, refusal := range refusals {
		t.Run(refused, func(t *testing.T) {
			_, err := aDeployment(t).WithFailureTolerance(refusal.tolerance)

			if err == nil {
				t.Fatalf("a Failure tolerance of %d Pods is refused, but it was admitted", refusal.tolerance)
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

	deployment, err := declaring.WithFailureTolerance(theFailureTolerance)
	if err != nil {
		t.Fatalf("declare a Failure tolerance of %d Pods: %v", theFailureTolerance, err)
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
