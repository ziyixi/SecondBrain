package reasoning

import (
	"context"
	"sync"
)

// Router routes LLM requests to different providers based on model name.
// Each model name maps to a specific LLMProvider implementation.
// If a model is not registered, the fallback provider is used.
type Router struct {
	mu        sync.RWMutex
	providers map[string]LLMProvider // model name -> provider
	fallback  LLMProvider
}

// NewRouter creates a new provider router with a fallback provider.
func NewRouter(fallback LLMProvider) *Router {
	return &Router{
		providers: make(map[string]LLMProvider),
		fallback:  fallback,
	}
}

// Register associates a model name with a provider.
func (r *Router) Register(model string, provider LLMProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[model] = provider
}

// ListModels returns all registered model names.
func (r *Router) ListModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	models := make([]string, 0, len(r.providers))
	for m := range r.providers {
		models = append(models, m)
	}
	return models
}

// ForModel returns the provider for the given model, or the fallback.
func (r *Router) ForModel(model string) LLMProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.providers[model]; ok {
		return p
	}
	return r.fallback
}

// Generate routes to the fallback provider.
func (r *Router) Generate(ctx context.Context, prompt string) (string, error) {
	return r.fallback.Generate(ctx, prompt)
}

// Classify routes to the fallback provider.
func (r *Router) Classify(ctx context.Context, content string, categories []string) (string, float64, error) {
	return r.fallback.Classify(ctx, content, categories)
}

// GenerateWithModel routes to the provider registered for the given model.
func (r *Router) GenerateWithModel(ctx context.Context, model, prompt string) (string, error) {
	return r.ForModel(model).Generate(ctx, prompt)
}
