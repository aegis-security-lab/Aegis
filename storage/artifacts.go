package storage

import (
	"context"
	"io"
	"time"
)

type PutArtifact struct {
	ExecutionID string
	Name        string
	MediaType   string
	Reader      io.Reader
}

type Artifact struct {
	ID          string    `json:"id"`
	ExecutionID string    `json:"executionId"`
	Name        string    `json:"name"`
	MediaType   string    `json:"mediaType,omitempty"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	URI         string    `json:"uri"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ArtifactStore keeps large outputs outside Coordination execution JSON. Open must
// return a fresh reader owned by the caller.
type ArtifactStore interface {
	PutArtifact(context.Context, PutArtifact) (Artifact, error)
	OpenArtifact(context.Context, string) (Artifact, io.ReadCloser, error)
	DeleteArtifact(context.Context, string) error
}
