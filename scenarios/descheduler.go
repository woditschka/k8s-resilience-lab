// The Descheduler policies the lab's own Scenarios declare. The lab defines
// none, so a Scenario declares one of these.

package scenarios

import "github.com/woditschka/k8s-resilience-lab/internal/lab/descheduler"

// EvenSpreadRestored evicts a Pod standing past a topology spread constraint
// its Deployment declares as DoNotSchedule, one Pod per pass, so the scheduler
// places it again where the spread holds while every other Pod stays Ready.
func EvenSpreadRestored() descheduler.Policy {
	return descheduler.Policy{
		MaxNoOfPodsToEvictTotal: 1,
		Profiles: []descheduler.Profile{{
			Name: "even-spread-restored",
			PluginConfig: []descheduler.PluginConfig{
				{Name: descheduler.DefaultEvictor, Args: descheduler.DefaultEvictorArgs{NodeFit: true}},
				{
					Name: descheduler.RemovePodsViolatingTopologySpreadConstraint,
					Args: descheduler.RemovePodsViolatingTopologySpreadConstraintArgs{
						Constraints: []descheduler.UnsatisfiableConstraintAction{descheduler.DoNotSchedule},
					},
				},
			},
			Plugins: descheduler.Plugins{
				Balance: descheduler.PluginSet{Enabled: []descheduler.PluginName{descheduler.RemovePodsViolatingTopologySpreadConstraint}},
			},
		}},
	}
}
