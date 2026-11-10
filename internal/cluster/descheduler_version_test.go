package cluster

import (
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

// The pairing names no Descheduler for a minor this old, which is all this
// version stands for.
const aVersionPairedWithNoDescheduler = "v1.10.4"

// A version naming its major alone has no minor to pair by.
const aVersionNamingNoMinor = "v1"

func TestTheDefaultKubernetesVersion_ShouldBePairedWithADescheduler(t *testing.T) {
	paired, err := DefaultKubernetesVersion().PairedDescheduler()
	if err != nil {
		t.Fatalf("pair a Descheduler with the default Kubernetes version: %v", err)
	}

	expect.Expect(t, paired.String() != "").To(expect.Equal(true))
}

func TestAKubernetesVersionWhoseMinorThePairingNamesNoDeschedulerFor_ShouldBeRefusedNamingThatVersion(t *testing.T) {
	unpaired := map[string]string{
		"a minor with no row":   aVersionPairedWithNoDescheduler,
		"a version of no minor": aVersionNamingNoMinor,
	}

	for named, written := range unpaired {
		t.Run(named, func(t *testing.T) {
			version, err := ParseKubernetesVersion(written)
			if err != nil {
				t.Fatalf("parse the Kubernetes version %s: %v", written, err)
			}

			paired, err := version.PairedDescheduler()

			if err == nil {
				t.Fatalf("the lab pairs no Descheduler with %s, but it paired %s", written, paired)
			}
			expect.Expect(t, strings.Contains(err.Error(), written)).To(expect.Equal(true))
		})
	}
}
