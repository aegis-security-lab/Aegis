// Package board owns the Aegis Board application declaration. Board is not a
// platform built-in: hosts must register this module explicitly.
package board

import (
	"aegis/apps/board/control"
	"aegis/apps/board/coordination"
	"aegis/apps/board/identity"
	platformcoordination "aegis/coordination"
	platformapp "aegis/platform/application"
	"aegis/platform/dataspace"
)

const (
	AppID            = identity.AppID
	PrimaryDataSpace = identity.PrimaryDataSpace
)

type Module struct{}

func (Module) Manifest() platformapp.Manifest {
	return platformapp.Manifest{
		ID: AppID, Version: "1.0.0", SDKVersion: "1", DisplayName: "Aegis Board",
		DataSpaces: []string{PrimaryDataSpace}, PhoneModules: []string{"aegis.board", "aegis.relay"},
		AgentProfiles: control.DefaultAgentProfileIDs(),
		Prompts:       []string{"board.agent.system", "board.issue.execution", "board.validation", "board.relay"},
		Controllers:   []string{"board_autonomy@1"},
	}
}

func (Module) DataSpaces() []dataspace.Descriptor {
	return []dataspace.Descriptor{{
		AppID: AppID, Name: PrimaryDataSpace, Version: "1", Kinds: []dataspace.Kind{dataspace.KindSQL, dataspace.KindBlob, dataspace.KindDocument},
	}}
}

func (Module) RegisterCoordination(registry *platformcoordination.Registry) error {
	return boardcoordination.Register(registry)
}
