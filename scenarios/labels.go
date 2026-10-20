// The keys of the Node labels the lab's own Scenarios state. Each key has one
// home, so a NodePool stating a label, a spread over it and a Then counting
// Pods per its value name the same key.

package scenarios

// CapacityType marks a Node on-demand or spot. It is unprefixed, a key private
// to the user by Kubernetes' convention, so it implies no vendor's meaning.
const CapacityType = "capacity-type"
