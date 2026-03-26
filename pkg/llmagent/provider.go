package llmagent

import (
	"fmt"
	"os"
	"sort"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/google"
	"charm.land/fantasy/providers/openai"
)

const (
	anthropicProviderKey = "anthropic"
	openaiProviderKey    = "openai"
	geminiProviderKey    = "gemini"
	googleProviderKey    = "google"

	anthropicUseVertexEnvVar  = "ANTHROPIC_USE_VERTEX"
	anthropicApiKeyEnvVar     = "ANTHROPIC_API_KEY"
	geminiUseVertexEnvVar     = "GEMINI_USE_VERTEX"
	googleUseVertexEnvVar     = "GOOGLE_USE_VERTEX"
	geminiApiKeyEnvVar        = "GEMINI_API_KEY"
	googleApiKeyEnvVar        = "GOOGLE_API_KEY"
	googleCloudProjectEnvVar  = "GOOGLE_CLOUD_PROJECT"
	googleCloudLocationEnvVar = "GOOGLE_CLOUD_LOCATION"
	openaiApiKeyEnvVar        = "OPENAI_API_KEY"
	openaiBaseUrlEnvVar       = "OPENAI_BASE_URL"
)

func ResolveProvider(providerName string) (fantasy.Provider, error) {
	def, ok := providerBuilders[providerName]
	if !ok {
		supported := make([]string, 0, len(providerBuilders))
		for k := range providerBuilders {
			supported = append(supported, k)
		}
		sort.Strings(supported)

		return nil, fmt.Errorf("unsupported provider %q, supported: %v", providerName, supported)
	}

	return def.Build()
}

// providerBuilder knows how to create a fantasy.Provider from env vars
type providerBuilder interface {
	Build() (fantasy.Provider, error)
}

var providerBuilders = map[string]providerBuilder{
	anthropicProviderKey: &anthropicProviderBuilder{},
	geminiProviderKey:    &googleProviderBuilder{providerName: geminiProviderKey},
	googleProviderKey:    &googleProviderBuilder{providerName: googleProviderKey},
	openaiProviderKey:    &openaiProviderBuilder{},
}

type anthropicProviderBuilder struct{}

func (p *anthropicProviderBuilder) Build() (fantasy.Provider, error) {
	opts := []anthropic.Option{}

	useVertex := os.Getenv(anthropicUseVertexEnvVar)
	if useVertex == "1" {
		project := os.Getenv(googleCloudProjectEnvVar)
		if project == "" {
			return nil, fmt.Errorf(
				"provider anthropic requires env var %q to be set when %q is set to 1",
				googleCloudProjectEnvVar,
				anthropicUseVertexEnvVar,
			)
		}
		location := os.Getenv(googleCloudLocationEnvVar)
		if location == "" {
			return nil, fmt.Errorf(
				"provider anthropic requires env var %q to be set when %q is set to 1",
				googleCloudLocationEnvVar,
				anthropicUseVertexEnvVar,
			)
		}

		opts = append(opts, anthropic.WithVertex(project, location))
	} else {
		key := os.Getenv(anthropicApiKeyEnvVar)
		if key == "" {
			return nil, fmt.Errorf(
				"provider anthropic requires env var %q to be set (or enable Vertex AI with %q=1)",
				anthropicApiKeyEnvVar,
				anthropicUseVertexEnvVar,
			)
		}
		opts = append(opts, anthropic.WithAPIKey(key))
	}

	return anthropic.New(opts...)
}

type googleProviderBuilder struct {
	providerName string
}

func (p *googleProviderBuilder) Build() (fantasy.Provider, error) {
	opts := []google.Option{}

	useVertex := os.Getenv(geminiUseVertexEnvVar) == "1" || os.Getenv(googleUseVertexEnvVar) == "1"
	if useVertex {
		project := os.Getenv(googleCloudProjectEnvVar)
		if project == "" {
			return nil, fmt.Errorf(
				"provider %s requires env var %q to be set when %q or %q is set to 1",
				p.providerName,
				googleCloudProjectEnvVar,
				googleUseVertexEnvVar,
				geminiUseVertexEnvVar,
			)
		}
		location := os.Getenv(googleCloudLocationEnvVar)
		if location == "" {
			return nil, fmt.Errorf(
				"provider %s requires env var %q to be set when %q or %q is set to 1",
				p.providerName,
				googleCloudLocationEnvVar,
				googleUseVertexEnvVar,
				geminiUseVertexEnvVar,
			)
		}

		opts = append(opts, google.WithVertex(project, location))
	} else {
		key := os.Getenv(geminiApiKeyEnvVar)
		if key == "" {
			key = os.Getenv(googleApiKeyEnvVar)
		}

		if key == "" {
			return nil, fmt.Errorf(
				"provider %s requires env var %q or %q to be set (or enable Vertex AI with %q=1 or %q=1)",
				p.providerName,
				googleApiKeyEnvVar,
				geminiApiKeyEnvVar,
				googleUseVertexEnvVar,
				geminiUseVertexEnvVar,
			)
		}
		opts = append(opts, google.WithGeminiAPIKey(key))
	}

	return google.New(opts...)
}

type openaiProviderBuilder struct{}

func (p *openaiProviderBuilder) Build() (fantasy.Provider, error) {
	opts := []openai.Option{}

	key := os.Getenv(openaiApiKeyEnvVar)
	if key != "" {
		opts = append(opts, openai.WithAPIKey(key))
	}

	baseUrl := os.Getenv(openaiBaseUrlEnvVar)
	if baseUrl != "" {
		opts = append(opts, openai.WithBaseURL(baseUrl))
	}

	return openai.New(opts...)
}
