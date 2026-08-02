package skill

import (
	"context"
	"strings"
	"testing"

	"aegis/capability"
)

func TestSourceLoadsVersionedInstructionAndDigest(t *testing.T) {
	loader := LoaderFunc(func(_ context.Context, name, version string) (Document, error) {
		if name != "go-service" || version != "v2" {
			t.Fatalf("load %s@%s", name, version)
		}
		return Document{Name: name, Version: version, Content: "# Go service\n\nRun tests.", Metadata: map[string]string{"origin": "database"}}, nil
	})
	registry := capability.NewRegistry()
	if err := Register(registry, "go-service", loader); err != nil {
		t.Fatal(err)
	}
	bundle, err := registry.Resolve(context.Background(), capability.ResolveContext{}, []capability.Ref{{Kind: capability.KindSkill, Name: "go-service", Version: "v2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Tools) != 0 || len(bundle.Instructions) != 1 || bundle.Instructions[0].Source != "skill/go-service" {
		t.Fatalf("bundle = %+v", bundle)
	}
	metadata := bundle.Snapshots[0].Metadata
	if metadata["origin"] != "database" || len(metadata["sha256"]) != 64 || strings.Contains(metadata["sha256"], " ") {
		t.Fatalf("metadata = %v", metadata)
	}
}
