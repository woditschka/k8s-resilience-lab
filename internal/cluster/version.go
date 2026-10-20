// Package cluster owns the Simulated Cluster: the container lifecycle on the
// machine's engine, every Kubernetes call, and the mapping between the lab's
// vocabulary and the control plane's. Kubernetes types stay inside this
// package.
package cluster

import (
	"fmt"
	"regexp"
	"strings"
)

// Every value other than a version is refused, so no reference, path, scheme or
// shell character can reach the image tag a run pulls.
var aVersionAndNothingElse = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+)*$`)

// KubernetesVersion exists so that an unvalidated string cannot reach the
// engine.
type KubernetesVersion struct {
	value string
}

// ParseKubernetesVersion adds the leading v the image tag is composed with, and
// refuses every non-version before any request leaves the process.
func ParseKubernetesVersion(named string) (KubernetesVersion, error) {
	if !aVersionAndNothingElse.MatchString(named) {
		return KubernetesVersion{}, fmt.Errorf("a Kubernetes version is a leading v followed by digits and dots alone, so %q is refused", named)
	}

	return KubernetesVersion{value: "v" + strings.TrimPrefix(named, "v")}, nil
}

func (v KubernetesVersion) String() string {
	return v.value
}

// Equal is the equality go-cmp compares through, since the type's field is
// unexported.
func (v KubernetesVersion) Equal(other KubernetesVersion) bool {
	return v.value == other.value
}
