package dataspace

import "testing"

func TestRegistryIsolatesSpacesByApplication(t *testing.T) {
	registry := NewRegistry()
	for _, descriptor := range []Descriptor{
		{AppID: "aegis.board", Name: "primary", Version: "1", Kinds: []Kind{KindSQL, KindBlob}},
		{AppID: "media.shortvideo", Name: "primary", Version: "1", Kinds: []Kind{KindDocument, KindBlob}},
	} {
		if err := registry.Register(descriptor); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := registry.Resolve("aegis.board", "primary"); !ok {
		t.Fatal("Board DataSpace was not registered")
	}
	if _, ok := registry.Resolve("communication.calling", "primary"); ok {
		t.Fatal("an unregistered application resolved another app's DataSpace")
	}
	if got := registry.Descriptors(); len(got) != 2 || got[0].AppID != "aegis.board" || got[1].AppID != "media.shortvideo" {
		t.Fatalf("descriptors=%+v", got)
	}
}

func TestRegistryRejectsDuplicateAndInvalidDescriptors(t *testing.T) {
	registry := NewRegistry()
	descriptor := Descriptor{AppID: "aegis.board", Name: "primary", Version: "1", Kinds: []Kind{KindSQL}}
	if err := registry.Register(descriptor); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(descriptor); err == nil {
		t.Fatal("duplicate DataSpace was accepted")
	}
	if err := registry.Register(Descriptor{AppID: "aegis.board", Name: "empty", Version: "1"}); err == nil {
		t.Fatal("descriptor without storage kinds was accepted")
	}
}
