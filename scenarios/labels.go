// The keys of the Node labels the lab's own Scenarios state on their NodePools,
// one exported constant each. It is the one home of each key, so a NodePool
// stating a label, a spread over it and a Then counting Pods per its value name
// the same key rather than writing the string again. The lab owns none of these
// keys: each is the Scenarios' own, per
// docs/adr/2026-10-18-a-nodepool-states-its-own-node-labels-beside-the-labs.md.

package scenarios

// CapacityType marks a Node on-demand or spot, the two ways a cloud sells the
// same machine. It is unprefixed, a key private to the user by Kubernetes'
// convention, so it implies no vendor's meaning.
const CapacityType = "capacity-type"
