package board

import (
	"testing"

	platformapp "aegis/platform/application"
	"aegis/platform/dataspace"
)

func TestBoardRequiresExplicitPlatformRegistration(t *testing.T) {
	applications := platformapp.NewRegistry()
	spaces := dataspace.NewRegistry()
	if len(applications.Manifests()) != 0 || len(spaces.Descriptors()) != 0 {
		t.Fatal("platform unexpectedly contains Board before registration")
	}
	module := Module{}
	if err := applications.Register(module); err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range module.DataSpaces() {
		if err := spaces.Register(descriptor); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := applications.Resolve(AppID); !ok {
		t.Fatal("Board application was not registered")
	}
	if _, ok := spaces.Resolve(AppID, PrimaryDataSpace); !ok {
		t.Fatal("Board DataSpace was not registered")
	}
	manifest := module.Manifest()
	if len(manifest.AgentProfiles) == 0 || len(manifest.Prompts) == 0 || len(manifest.Controllers) != 1 {
		t.Fatalf("Board extensions were not declared: %+v", manifest)
	}
}
