package application

import (
	"testing"

	"aegis/platform/dataspace"
)

type testModule struct{ manifest Manifest }

func (m testModule) Manifest() Manifest { return m.manifest }

func TestRegistryHasNoBuiltInApplications(t *testing.T) {
	registry := NewRegistry()
	if got := registry.Manifests(); len(got) != 0 {
		t.Fatalf("platform registry unexpectedly contains product apps: %+v", got)
	}
}

func TestRegistryRequiresExplicitApplicationRegistration(t *testing.T) {
	registry := NewRegistry()
	module := testModule{manifest: Manifest{ID: "aegis.board", Version: "1.0.0", SDKVersion: "1", DisplayName: "Aegis Board", DataSpaces: []string{"primary"}}}
	if err := registry.Register(module); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Resolve("aegis.board"); !ok {
		t.Fatal("registered application was not resolved")
	}
	if err := registry.Register(module); err == nil {
		t.Fatal("duplicate application was accepted")
	}
}

type dataModule struct{ testModule }

func (m dataModule) DataSpaces() []dataspace.Descriptor {
	manifest := m.Manifest()
	return []dataspace.Descriptor{{AppID: manifest.ID, Name: "primary", Version: "1", Kinds: []dataspace.Kind{dataspace.KindSQL}}}
}

func TestCatalogRegistersApplicationAndOwnedDataSpacesTogether(t *testing.T) {
	catalog := NewCatalog()
	module := dataModule{testModule{manifest: Manifest{ID: "aegis.board", Version: "1.0.0", SDKVersion: "1", DisplayName: "Aegis Board", DataSpaces: []string{"primary"}}}}
	if err := catalog.Register(module); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Applications()) != 1 || len(catalog.DataSpaces()) != 1 {
		t.Fatalf("apps=%+v spaces=%+v", catalog.Applications(), catalog.DataSpaces())
	}
}

func TestManifestRejectsDuplicateExtensionRegistrations(t *testing.T) {
	manifest := Manifest{ID: "example.media", Version: "1", SDKVersion: "1", DisplayName: "Media", Prompts: []string{"script", "script"}}
	if err := manifest.Validate(); err == nil {
		t.Fatal("duplicate prompt registration was accepted")
	}
}
