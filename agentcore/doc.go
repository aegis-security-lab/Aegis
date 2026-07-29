// Package agentcore implements a provider-neutral, Pi-style streaming agent loop.
//
// The package intentionally owns only the execution semantics: model turns,
// tool calls, event delivery, cancellation, and in-memory conversation state.
// Persistence, user interfaces, authentication, and provider selection belong
// to callers or adapter packages.
package agentcore
