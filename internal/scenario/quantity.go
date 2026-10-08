package scenario

import "strconv"

// The units the two quantities below are held in, which are the units a
// Kubernetes quantity carries each of them in without loss: a cpu amount in
// thousandths of a core, a memory amount in mebibytes. Holding the unit
// Kubernetes holds is what lets a quantity of the lab's own round-trip through
// the control plane exactly.
const (
	millicoresPerCore    = 1000
	mebibytesPerNextUnit = 1024
)

// memoryUnits are the binary units a memory amount is spelled in, from the one
// it is held in upwards. Kubernetes spells such an amount in the largest of them
// the amount is a whole number of, so the lab spells it the same way.
var memoryUnits = []string{"Mi", "Gi", "Ti", "Pi", "Ei"}

// CPU is an amount of processor time: what one Node has, or what one Pod asks
// for. It is held in the millicores Kubernetes' own quantity carries, so a whole
// core is 1000 of them.
type CPU int64

// Memory is an amount of memory: what one Node has, or what one Pod asks for. It
// is held in the mebibytes Kubernetes' own quantity carries, so a gibibyte is
// 1024 of them.
type Memory int64

// String is this cpu amount as Kubernetes spells it: the whole cores where it is
// a whole number of them, and the millicores otherwise. The mapping test pins
// this spelling to the client library's own, since the lab spells it itself.
func (c CPU) String() string {
	if c%millicoresPerCore == 0 {
		return strconv.FormatInt(int64(c/millicoresPerCore), 10)
	}

	return strconv.FormatInt(int64(c), 10) + "m"
}

// String is this memory amount as Kubernetes spells it: the largest binary unit
// the amount is a whole number of, and no unit at all for none, pinned to the
// client library's own spelling the way a cpu amount is.
func (m Memory) String() string {
	if m == 0 {
		return strconv.FormatInt(0, 10)
	}

	amount, unit := int64(m), 0
	for unit < len(memoryUnits)-1 && amount%mebibytesPerNextUnit == 0 {
		amount /= mebibytesPerNextUnit
		unit++
	}

	return strconv.FormatInt(amount, 10) + memoryUnits[unit]
}
