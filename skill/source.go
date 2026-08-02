package skill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"aegis/capability"
)

// Document is a detached, versioned Skill loaded from any backing store.
type Document struct {
	Name     string
	Version  string
	Content  string
	Metadata map[string]string
}

// Loader owns persistence and version selection for Skill documents.
type Loader interface {
	LoadSkill(context.Context, string, string) (Document, error)
}

type LoaderFunc func(context.Context, string, string) (Document, error)

func (f LoaderFunc) LoadSkill(ctx context.Context, name, version string) (Document, error) {
	return f(ctx, name, version)
}

// Source turns a durable skill Ref into one trusted Agent instruction.
type Source struct{ Loader Loader }

func (s Source) Resolve(ctx context.Context, _ capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
	if s.Loader == nil {
		return capability.Resolved{}, errors.New("skill: loader is required")
	}
	document, err := s.Loader.LoadSkill(ctx, ref.Name, ref.Version)
	if err != nil {
		return capability.Resolved{}, fmt.Errorf("skill: load %s: %w", ref.Name, err)
	}
	content := strings.TrimSpace(document.Content)
	if content == "" {
		return capability.Resolved{}, fmt.Errorf("skill: %s has empty content", ref.Name)
	}
	name := fallback(strings.TrimSpace(document.Name), ref.Name)
	version := fallback(strings.TrimSpace(document.Version), ref.Version)
	digest := sha256.Sum256([]byte(content))
	metadata := cloneMetadata(document.Metadata)
	metadata["sha256"] = hex.EncodeToString(digest[:])
	return capability.Resolved{
		Instructions: []capability.Instruction{{Source: "skill/" + name, Content: content}},
		Snapshot:     capability.Snapshot{Kind: capability.KindSkill, Name: name, Version: version, Metadata: metadata},
	}, nil
}

// Register installs a named Skill Source. Registering each durable skill name
// explicitly keeps capability allowlisting in the composition root.
func Register(registry *capability.Registry, name string, loader Loader) error {
	if registry == nil {
		return errors.New("skill: nil capability registry")
	}
	return registry.Register(capability.KindSkill, name, Source{Loader: loader})
}

func cloneMetadata(source map[string]string) map[string]string {
	result := make(map[string]string, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func fallback(value, alternative string) string {
	if value != "" {
		return value
	}
	return alternative
}
