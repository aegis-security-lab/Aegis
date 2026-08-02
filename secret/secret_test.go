package secret

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestEnvironmentResolverRequiresExplicitReferenceAndRedactsValue(t *testing.T) {
	resolver := EnvironmentResolver{
		References: map[Ref]string{"env://web-search": "TEST_WEB_KEY"},
		LookupEnv: func(name string) (string, bool) {
			if name != "TEST_WEB_KEY" {
				t.Fatalf("name = %s", name)
			}
			return "super-secret", true
		},
	}
	value, err := resolver.ResolveSecret(context.Background(), "env://web-search")
	if err != nil {
		t.Fatal(err)
	}
	if string(value.Bytes()) != "super-secret" || fmt.Sprint(value) != "<redacted>" {
		t.Fatalf("value=%v bytes=%q", value, value.Bytes())
	}
	if _, err := json.Marshal(value); err == nil {
		t.Fatal("secret value must reject JSON serialization")
	}
	if _, err := resolver.ResolveSecret(context.Background(), "env://arbitrary"); err == nil {
		t.Fatal("unlisted reference must be denied")
	}
}

func TestValueBytesAreDetached(t *testing.T) {
	source := []byte("secret")
	value := NewValue(source)
	source[0] = 'X'
	copy := value.Bytes()
	copy[0] = 'Y'
	if string(value.Bytes()) != "secret" {
		t.Fatalf("value = %q", value.Bytes())
	}
}
