package secret

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Ref string

func (r Ref) Validate() error {
	value := strings.TrimSpace(string(r))
	if value == "" || !strings.Contains(value, "://") {
		return errors.New("secret: invalid reference")
	}
	return nil
}

// Value renders as redacted and rejects JSON serialization. Consumers must
// explicitly request a detached byte slice.
type Value struct{ bytes []byte }

func NewValue(value []byte) Value { return Value{bytes: append([]byte(nil), value...)} }

func (v Value) Bytes() []byte { return append([]byte(nil), v.bytes...) }

func (v Value) Empty() bool { return len(v.bytes) == 0 }

func (Value) String() string { return "<redacted>" }

func (Value) GoString() string { return "secret.Value(<redacted>)" }

func (Value) MarshalJSON() ([]byte, error) {
	return nil, errors.New("secret: values cannot be serialized")
}

type Resolver interface {
	ResolveSecret(context.Context, Ref) (Value, error)
}

type ResolverFunc func(context.Context, Ref) (Value, error)

func (f ResolverFunc) ResolveSecret(ctx context.Context, ref Ref) (Value, error) { return f(ctx, ref) }

// EnvironmentResolver maps explicit refs to environment variable names. It
// never lets an untrusted capability config choose an arbitrary env key.
type EnvironmentResolver struct {
	References map[Ref]string
	LookupEnv  func(string) (string, bool)
}

func (r EnvironmentResolver) ResolveSecret(ctx context.Context, ref Ref) (Value, error) {
	if err := ref.Validate(); err != nil {
		return Value{}, err
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return Value{}, ctx.Err()
		default:
		}
	}
	name, ok := r.References[ref]
	if !ok || strings.TrimSpace(name) == "" {
		return Value{}, fmt.Errorf("secret: reference %s is not allowed", ref)
	}
	lookup := r.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	value, exists := lookup(name)
	if !exists || value == "" {
		return Value{}, fmt.Errorf("secret: environment value for %s is unavailable", ref)
	}
	return NewValue([]byte(value)), nil
}
