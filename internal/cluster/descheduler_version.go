package cluster

import (
	"fmt"
	"strings"
)

// The Descheduler releases one minor per Kubernetes minor, but a patch of
// either is released on its own, so the pairing is a reviewed row per minor
// rather than a rule computing a tag.
func deschedulerTagPairedWith(minor string) (string, bool) {
	switch minor {
	case "v1.36":
		return "v0.36.0", true
	default:
		return "", false
	}
}

const theVersionSeparator = "."

// DeschedulerVersion is built by the pairing alone, so a run never starts a
// Descheduler no row pairs with its Kubernetes version.
type DeschedulerVersion struct {
	tag string
}

func (v DeschedulerVersion) String() string {
	return v.tag
}

// PairedDescheduler refuses a version whose minor no row names, so a new
// minor needs a row before a run declaring a Descheduler policy runs on it.
func (v KubernetesVersion) PairedDescheduler() (DeschedulerVersion, error) {
	tag, paired := deschedulerTagPairedWith(v.minor())
	if !paired {
		return DeschedulerVersion{}, fmt.Errorf("the lab pairs no Descheduler with Kubernetes %s, "+
			"since its pairing holds no row for that minor", v)
	}

	return DeschedulerVersion{tag: tag}, nil
}

// A version naming its major alone has no minor and pairs with nothing.
func (v KubernetesVersion) minor() string {
	major, rest, namesAMinor := strings.Cut(v.value, theVersionSeparator)
	if !namesAMinor {
		return ""
	}
	minor, _, _ := strings.Cut(rest, theVersionSeparator)

	return major + theVersionSeparator + minor
}
