package scenario_test

import (
	"fmt"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
	"github.com/woditschka/k8s-resilience-lab/internal/scenario"
)

// One amount per spelling the two quantity types decide between.
const (
	fourCores            = scenario.CPU(4000)
	oneAndAHalfCores     = scenario.CPU(1500)
	sixteenGibibytes     = scenario.Memory(16384)
	oneTebibyte          = scenario.Memory(1048576)
	fiveHundredMebibytes = scenario.Memory(500)
	noMemoryHeld         = scenario.Memory(0)
)

// A capacity is read off a Step line the lab spells and off a Timeline the
// client library spells, so one amount stated two ways would read as two.
// internal/cluster's mapping test pins these spellings to the client library's
// output.
func TestAQuantityTheLabHolds_ShouldBeSpelledAsKubernetesSpellsThatAmount(t *testing.T) {
	held := map[string]struct {
		quantity fmt.Stringer
		spelled  string
	}{
		"a whole number of cores":                                 {quantity: fourCores, spelled: "4"},
		"a cpu amount that is not a whole number of cores":        {quantity: oneAndAHalfCores, spelled: "1500m"},
		"a whole number of gibibytes":                             {quantity: sixteenGibibytes, spelled: "16Gi"},
		"a whole number of tebibytes":                             {quantity: oneTebibyte, spelled: "1Ti"},
		"a memory amount that is not a whole number of gibibytes": {quantity: fiveHundredMebibytes, spelled: "500Mi"},
		"no memory at all":                                        {quantity: noMemoryHeld, spelled: "0"},
	}

	for amount, stated := range held {
		t.Run(amount, func(t *testing.T) {
			expect.Expect(t, stated.quantity.String()).To(expect.Equal(stated.spelled))
		})
	}
}
