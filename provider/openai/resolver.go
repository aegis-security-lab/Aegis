package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"aegis/agenthost"
	"aegis/secret"
	"github.com/z3r2ne/agentcore"
)

type ProviderConfig struct {
	BaseURL    string
	APIKeyRef  secret.Ref
	HTTPClient *http.Client
	Headers    map[string]string
}

// Resolver maps durable provider names to runtime endpoint and credential
// configuration. The API key itself never appears in agenthost.ModelRef.
type Resolver struct {
	Providers map[string]ProviderConfig
	Secrets   secret.Resolver
}

func (r Resolver) ResolveModel(ctx context.Context, ref agenthost.ModelRef) (agentcore.Model, error) {
	provider := strings.ToLower(strings.TrimSpace(ref.Provider))
	config, ok := r.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("provider/openai: provider %s is not configured", ref.Provider)
	}
	apiKey := ""
	if config.APIKeyRef != "" {
		if r.Secrets == nil {
			return nil, errors.New("provider/openai: secret resolver is required")
		}
		value, err := r.Secrets.ResolveSecret(ctx, config.APIKeyRef)
		if err != nil {
			return nil, fmt.Errorf("provider/openai: resolve credential: %w", err)
		}
		bytes := value.Bytes()
		apiKey = string(bytes)
		for index := range bytes {
			bytes[index] = 0
		}
	}
	return NewModel(Config{BaseURL: config.BaseURL, APIKey: apiKey, HTTPClient: config.HTTPClient, Headers: config.Headers}, ref.Model)
}

var _ agenthost.ModelResolver = Resolver{}
