// Package cluster owns the Simulated Cluster: the container lifecycle on the
// machine's engine, every Kubernetes call, and the mapping between the lab's
// vocabulary and the control plane's.
//
// Kubernetes types stay inside this package. Only its mapper names both a
// Kubernetes type and a scenario type, per docs/system-design.md § Package
// Structure.
package cluster

import (
	"fmt"
	"regexp"
	"strings"
)

// aVersionAndNothingElse is a leading v with digits and dots alone. Every other
// value is refused, so no reference, path, scheme or shell character can survive
// into the image tag a run pulls, per docs/adr/2026-10-03-kubernetes-version-pin.md.
var aVersionAndNothingElse = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+)*$`)

// KubernetesVersion is the validated version a run mirrors. The type exists so
// that an unvalidated string cannot reach the engine: every call that names a
// version takes this type, never a string.
type KubernetesVersion struct {
	value string
}

// ParseKubernetesVersion validates a version a run named, normalising it to carry
// the leading v the image tag is composed with. It refuses every value that is
// not a version, before any request leaves the process.
func ParseKubernetesVersion(named string) (KubernetesVersion, error) {
	if !aVersionAndNothingElse.MatchString(named) {
		return KubernetesVersion{}, fmt.Errorf("a Kubernetes version is a leading v followed by digits and dots alone, so %q is refused", named)
	}

	return KubernetesVersion{value: "v" + strings.TrimPrefix(named, "v")}, nil
}

// String is the version as a run reports and mirrors it, carrying its leading v.
func (v KubernetesVersion) String() string {
	return v.value
}

// Equal reports whether two runs mirror the same version. It is the equality
// go-cmp compares through, since the type's field is unexported.
func (v KubernetesVersion) Equal(other KubernetesVersion) bool {
	return v.value == other.value
}
