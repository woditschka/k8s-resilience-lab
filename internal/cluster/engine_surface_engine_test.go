//go:build !unit

package cluster

import (
	"net/http"
	"strings"
	"testing"
)

// Only containers the lab did not create going away is asserted, not the whole
// inventory: another invocation of the lab may create and remove its own
// Simulated Clusters meanwhile, which is the supported concurrent case. That
// the lab cannot reach a foreign container is asserted from declarations by the
// engine surface's tests needing no engine; this one is the observable outcome on a real machine.
//
// The inventory is read and never written.
func TestARun_ShouldTouchNoContainerBesidesItsOwnSimulatedCluster(t *testing.T) {
	t.Parallel()
	engine := requireEngine(t)
	standingBy := containersOnTheEngine(t, engine)

	provisioned, err := Provision(t.Context(), engine, DefaultKubernetesVersion(), resilientPlacementScenario)
	if err != nil {
		t.Fatalf("provision a Simulated Cluster beside whatever else runs here: %v", err)
	}
	own := provisioned.Name()

	duringTheRun := containersOnTheEngine(t, engine)
	if _, created := duringTheRun[own]; !created {
		t.Errorf("the run creates its own Simulated Cluster %s, but the engine does not hold it", own)
	}
	assertUntouched(t, standingBy, duringTheRun)

	if err := provisioned.Close(t.Context()); err != nil {
		t.Fatalf("remove the Simulated Cluster %s: %v", own, err)
	}

	afterTheRun := containersOnTheEngine(t, engine)
	if _, left := afterTheRun[own]; left {
		t.Errorf("the run removes its own Simulated Cluster %s, but the engine still holds it", own)
	}
	assertUntouched(t, standingBy, afterTheRun)
}

// A lab-named container standing by is another invocation's Simulated Cluster,
// which may be removed inside this run's window. That each invocation removes
// only its own cluster is asserted by
// TestConcurrentRuns_ShouldEachRemoveOnlyTheClusterTheyCreated.
func assertUntouched(t *testing.T, before, now map[string]string) {
	t.Helper()

	for name := range before {
		if _, present := now[name]; present {
			continue
		}
		if strings.HasPrefix(name, containerNamePrefix) {
			continue
		}
		t.Errorf("the run removes no container but its own, yet %s is no longer on the engine", name)
	}
}

// Listing lives in a test because the lab itself declares no call that could
// list a container it did not create.
func containersOnTheEngine(t *testing.T, endpoint Endpoint) map[string]string {
	t.Helper()

	var listed []struct {
		Names []string
		State string
	}
	if err := engineRequest(t.Context(), endpoint, http.MethodGet, "/containers/json?all=true", nil, &listed); err != nil {
		t.Fatalf("read which containers the engine holds: %v", err)
	}

	// The engine reports a container's name with a leading slash; the lab names
	// its own without one.
	held := make(map[string]string, len(listed))
	for _, container := range listed {
		for _, name := range container.Names {
			held[strings.TrimPrefix(name, "/")] = container.State
		}
	}

	return held
}
