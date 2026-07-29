package agentcore

import "errors"

var (
	// ErrModelRequired indicates an Agent was configured without a model.
	ErrModelRequired = errors.New("agentcore: model is required")
	// ErrMaxTurns indicates the configured model-turn limit was reached.
	ErrMaxTurns = errors.New("agentcore: maximum turns exceeded")
)
