package expect_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/woditschka/k8s-resilience-lab/internal/expect"
)

const (
	provisionedZone = "zone-1"
	anotherZone     = "zone-2"
	aThirdZone      = "zone-3"
)

// recordedReports keeps what a failing assertion reported instead of failing
// the suite, which is the only way to read expect's own output. It is a real
// implementation of the interface expect declares, not a stand-in for a
// collaborator.
type recordedReports struct {
	reports []string
}

func (*recordedReports) Helper() {}

func (r *recordedReports) Errorf(format string, args ...any) {
	r.reports = append(r.reports, fmt.Sprintf(format, args...))
}

func (r *recordedReports) joined() string {
	return strings.Join(r.reports, "\n")
}

func TestAMatchingValue_ShouldBeReportedNowhere(t *testing.T) {
	var reported recordedReports

	expect.Expect(&reported, provisionedZone).To(expect.Equal(provisionedZone))

	if len(reported.reports) > 0 {
		t.Errorf("a value equal to the expected one reports nothing, but got:\n%s", reported.joined())
	}
}

func TestADifferingValue_ShouldBeReportedAsADiffNamingBothValues(t *testing.T) {
	var reported recordedReports

	expect.Expect(&reported, provisionedZone).To(expect.Equal(anotherZone))

	report := reported.joined()
	if len(reported.reports) != 1 {
		t.Fatalf("one mismatch reports once, but got %d report(s):\n%s", len(reported.reports), report)
	}
	if !strings.Contains(report, provisionedZone) || !strings.Contains(report, anotherZone) {
		t.Errorf("the diff names the value found and the value expected, but got:\n%s", report)
	}
}

func TestASliceOfTheExpectedLength_ShouldBeReportedNowhere(t *testing.T) {
	provisionedZones := []string{provisionedZone, anotherZone, aThirdZone}
	var reported recordedReports

	expect.Expect(&reported, provisionedZones).To(expect.HaveLen[string](len(provisionedZones)))

	if len(reported.reports) > 0 {
		t.Errorf("a slice of the expected length reports nothing, but got:\n%s", reported.joined())
	}
}

func TestASliceOfAnotherLength_ShouldBeReportedWithBothLengths(t *testing.T) {
	provisionedZones := []string{provisionedZone, anotherZone, aThirdZone}
	oneZoneFewer := len(provisionedZones) - 1
	var reported recordedReports

	expect.Expect(&reported, provisionedZones).To(expect.HaveLen[string](oneZoneFewer))

	report := reported.joined()
	if !strings.Contains(report, strconv.Itoa(len(provisionedZones))) || !strings.Contains(report, strconv.Itoa(oneZoneFewer)) {
		t.Errorf("the report names the length found and the length expected, but got:\n%s", report)
	}
}
