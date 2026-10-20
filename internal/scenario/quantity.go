package scenario

import "strconv"

// Holding the units a Kubernetes quantity carries without loss, millicores and
// mebibytes, lets the lab's quantities round-trip through the control plane
// exactly.
const (
	millicoresPerCore    = 1000
	mebibytesPerNextUnit = 1024
)

// Kubernetes spells a memory amount in the largest unit it is a whole number
// of, so the lab does the same.
var memoryUnits = []string{"Mi", "Gi", "Ti", "Pi", "Ei"}

// CPU is held in millicores, so a whole core is 1000.
type CPU int64

// Memory is held in mebibytes, so a gibibyte is 1024.
type Memory int64

// String spells whole cores where it can and millicores otherwise. The mapping
// test pins this to the client library's own spelling.
func (c CPU) String() string {
	if c%millicoresPerCore == 0 {
		return strconv.FormatInt(int64(c/millicoresPerCore), 10)
	}

	return strconv.FormatInt(int64(c), 10) + "m"
}

// String uses the largest binary unit the amount is a whole number of, pinned
// to the client library's own spelling.
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
