//go:build !unit

package cluster

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// A constraint action the Descheduler validates and refuses, so a pass under
// it exits non-zero before it evicts anything.
const anActionTheDeschedulerRefuses = "NeverSchedule"

func aPolicyTheDeschedulerAccepts(t *testing.T) scenario.DeschedulerPolicy {
	t.Helper()

	return aPolicyBalancingTheSpreadOf(t, aConstraintAction)
}

func aPolicyTheDeschedulerRefuses(t *testing.T) scenario.DeschedulerPolicy {
	t.Helper()

	return aPolicyBalancingTheSpreadOf(t, anActionTheDeschedulerRefuses)
}

func aPolicyBalancingTheSpreadOf(t *testing.T, action string) scenario.DeschedulerPolicy {
	t.Helper()

	return aDeschedulerPolicyOf(t, anyLimitPerNode, aDeschedulerProfileOf(t, someProfileName,
		[]scenario.DeschedulerPluginConfig{scenario.NewDeschedulerPluginConfig(scenario.RemovePodsViolatingTopologySpreadConstraint,
			scenario.NewDeschedulerArgument(theConstraintsArgument, []string{action}))},
		scenario.NewDeschedulerPluginSet(scenario.BalanceExtensionPoint, scenario.RemovePodsViolatingTopologySpreadConstraint)))
}

// The pairing builds no such version; only this package's tests can, to stand
// for an image the registry never published.
const aTagTheRegistryNeverPublished = "v0.0.0-unpublished"

func aDeschedulerTheRegistryNeverPublished() DeschedulerVersion {
	return DeschedulerVersion{tag: aTagTheRegistryNeverPublished}
}

// A registry the engine cannot reach must not hold the test for the pull's own
// timeout.
const aRegistryAnswersWithin = 2 * time.Minute

func TestADeschedulerImageTheLabCannotObtain_ShouldNotBeObtainedAndShouldBeNamed(t *testing.T) {
	t.Parallel()
	engine := requireEngine(t)
	unpublished := aDeschedulerTheRegistryNeverPublished()
	obtaining, cancel := context.WithTimeout(t.Context(), aRegistryAnswersWithin)
	defer cancel()

	err := ObtainDescheduler(obtaining, engine, unpublished)

	if err == nil {
		t.Fatalf("no Descheduler image is published for %s, but it was obtained", unpublished)
	}
	expect.Expect(t, strings.Contains(err.Error(), aTagTheRegistryNeverPublished)).To(expect.Equal(true))
}

// theDeschedulerObtained makes the engine hold the paired Descheduler's image,
// as a run declaring a policy does before any Scenario starts, since a pass
// fetches none.
func theDeschedulerObtained(t *testing.T, engine Endpoint) DeschedulerVersion {
	t.Helper()

	paired := theDeschedulerPairedByDefault(t)
	if err := ObtainDescheduler(t.Context(), engine, paired); err != nil {
		t.Fatalf("obtain the Descheduler %s: %v", paired, err)
	}

	return paired
}

// The engine refuses the pass's creation for want of its image, which only a
// pass that fetched nothing first meets.
func TestADeschedulerPassInsideAScenario_ShouldFetchNoImage(t *testing.T) {
	t.Parallel()
	provisioned, engine := aProvisionedCluster(t)
	passing, cancel := context.WithTimeout(t.Context(), aRegistryAnswersWithin)
	defer cancel()

	_, err := provisioned.RunDeschedulerPass(passing, aPolicyTheDeschedulerAccepts(t), aDeschedulerTheRegistryNeverPublished())

	expect.Expect(t, refusedAtTheCreation{
		ForWantOfTheImage: isNoLongerHeld(err),
		AtTheCreation:     strings.Contains(fmt.Sprint(err), theCreationOfAPass),
		LeftOnTheEngine:   stillOnTheEngine(t, engine, deschedulerPassNameFor(provisioned.Name())),
	}).To(expect.Equal(refusedAtTheCreation{ForWantOfTheImage: true, AtTheCreation: true, LeftOnTheEngine: false}))
}

// A creation's refusal names the step it refused, as every step of a pass's
// errors does.
const theCreationOfAPass = "create the Descheduler pass"

// refusedAtTheCreation is how a pass on an image the engine lacks ended.
type refusedAtTheCreation struct {
	ForWantOfTheImage bool
	AtTheCreation     bool
	LeftOnTheEngine   bool
}

// The endings run in order, so the third is stopped a moment into its pass
// rather than before it.
//
// A Scenario that stops while its pass's container is created leaves the
// container the engine created with no answer reaching the lab, so that row
// starts from such a container and a Scenario already stopped.
func TestAScenarioThatRanADeschedulerPass_ShouldLeaveNothingOfItsDeschedulerWhenItEnds(t *testing.T) {
	t.Parallel()
	endings := []struct {
		ending string
		left   func(*testing.T, *SimulatedCluster)
		policy func(*testing.T) scenario.DeschedulerPolicy
		within func(context.Context) (context.Context, context.CancelFunc)
	}{
		{ending: "a pass that exits zero", left: nothingOfAnEarlierPass, policy: aPolicyTheDeschedulerAccepts, within: context.WithCancel},
		{ending: "a pass that exits non-zero", left: nothingOfAnEarlierPass, policy: aPolicyTheDeschedulerRefuses, within: context.WithCancel},
		{ending: "a pass whose Scenario stops while it runs", left: nothingOfAnEarlierPass, policy: aPolicyTheDeschedulerAccepts, within: stoppedAMomentIntoThePass},
		{ending: "a pass whose Scenario stops while its container is created", left: aPassContainerWhoseCreationWentUnanswered, policy: aPolicyTheDeschedulerAccepts, within: alreadyStopped},
	}
	provisioned, engine := aProvisionedCluster(t)
	paired := theDeschedulerObtained(t, engine)

	for _, passing := range endings {
		t.Run(passing.ending, func(t *testing.T) {
			passing.left(t, provisioned)
			within, stop := passing.within(t.Context())
			defer stop()

			_, _ = provisioned.RunDeschedulerPass(within, passing.policy(t), paired)

			expect.Expect(t, stillOnTheEngine(t, engine, deschedulerPassNameFor(provisioned.Name()))).To(expect.Equal(false))
		})
	}
}

// The engine answering the creation only after the lab stopped waiting leaves
// the same container behind as a Scenario stopping while it is created, so the
// test starts from that container and runs the next pass beside it.
func TestADeschedulerPassWhoseCreationTheEngineAnswersLate_ShouldLeaveNothingOfItsDeschedulerForTheNextPass(t *testing.T) {
	t.Parallel()
	provisioned, engine := aProvisionedCluster(t)
	aPassContainerWhoseCreationWentUnanswered(t, provisioned)

	_, err := provisioned.RunDeschedulerPass(t.Context(), aPolicyTheDeschedulerAccepts(t), theDeschedulerPairedByDefault(t))

	if err != nil {
		t.Fatalf("run the Descheduler pass after one whose creation was answered late: %v", err)
	}
	expect.Expect(t, stillOnTheEngine(t, engine, deschedulerPassNameFor(provisioned.Name()))).To(expect.Equal(false))
}

// The moment is past creating the container and short of a pass ending.
const aMomentIntoThePass = 500 * time.Millisecond

func stoppedAMomentIntoThePass(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, aMomentIntoThePass)
}

func alreadyStopped(ctx context.Context) (context.Context, context.CancelFunc) {
	stopped, stop := context.WithCancel(ctx)
	stop()

	return stopped, stop
}

func nothingOfAnEarlierPass(*testing.T, *SimulatedCluster) {}

// aPassContainerWhoseCreationWentUnanswered creates the pass's container and
// drops its handle, the state a creation leaves when the lab stopped waiting
// for the engine's answer. Its removal runs before the cluster's, so the
// container goes even while the lab leaves it behind. The removal is not
// guarded by a read under t.Context, which is cancelled before cleanups run.
func aPassContainerWhoseCreationWentUnanswered(t *testing.T, provisioned *SimulatedCluster) {
	t.Helper()

	image := deschedulerImage(theDeschedulerObtained(t, provisioned.container.endpoint))
	unanswered, err := provisioned.container.createDeschedulerPass(t.Context(), image)
	if err != nil {
		t.Fatalf("create the Descheduler pass of %s: %v", provisioned.Name(), err)
	}
	t.Cleanup(func() {
		err := removeContainer(context.WithoutCancel(t.Context()), unanswered.endpoint, unanswered.identifier, "the Descheduler pass "+unanswered.name)
		if err != nil && !isNoLongerHeld(err) {
			t.Errorf("remove the Descheduler pass %s: %v", unanswered.name, err)
		}
	})
}
