// Package coordination implements durable, pluggable multi-agent coordination.
//
// A Mode is a pure decision component: it receives one durable Event and a
// read-only Snapshot, then returns Effects. The Engine persists those effects
// atomically with event completion. Separate effect workers perform the actual
// scheduling, messaging, session delivery, application mutation, and delayed
// wakeup.
// This keeps collaboration policy independent from the agent loop,
// transports, and databases.
package coordination
