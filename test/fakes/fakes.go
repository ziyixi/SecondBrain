// Package fakes provides mock implementations of LLM, Notion (fact store), and
// Notion page fetcher for integration tests without external APIs.
package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/yourusername/secondbrain/internal/llm"
	"github.com/yourusername/secondbrain/internal/memory"
)

// ScriptedLLM is an llm.Client that returns a predefined sequence of responses (for integration tests).
type ScriptedLLM struct {
	mu        sync.Mutex
	responses []*llm.GenerateOutput
}

// NewScriptedLLM returns a client that pops the next response from the list on each Generate call.
func NewScriptedLLM(responses ...*llm.GenerateOutput) *ScriptedLLM {
	return &ScriptedLLM{responses: responses}
}

// Generate implements llm.Client.
func (s *ScriptedLLM) Generate(ctx context.Context, in llm.GenerateInput) (*llm.GenerateOutput, error) {
	s.mu.Lock()
	if len(s.responses) == 0 {
		s.mu.Unlock()
		return &llm.GenerateOutput{Content: "No scripted response left.", FinishReason: "stop"}, nil
	}
	out := s.responses[0]
	s.responses = s.responses[1:]
	s.mu.Unlock()
	return out, nil
}

// Embed implements llm.Client.
func (s *ScriptedLLM) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3, 0.4}, nil
}

// InMemoryFactStore implements memory.UserFactStore for tests (no real Notion).
type InMemoryFactStore struct {
	mu      sync.Mutex
	profile map[string][]string // userID -> list of fact strings
}

// NewInMemoryFactStore creates a fact store that keeps facts in memory.
func NewInMemoryFactStore() *InMemoryFactStore {
	return &InMemoryFactStore{profile: make(map[string][]string)}
}

// GetProfile implements memory.UserFactStore.
func (m *InMemoryFactStore) GetProfile(ctx context.Context, userID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	facts := m.profile[userID]
	var s string
	for _, f := range facts {
		if s != "" {
			s += "\n"
		}
		s += f
	}
	return s, nil
}

// AppendFact implements memory.UserFactStore.
func (m *InMemoryFactStore) AppendFact(ctx context.Context, userID string, fact string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profile[userID] = append(m.profile[userID], fact)
	return nil
}

// Facts returns the list of facts stored for a user (for assertions).
func (m *InMemoryFactStore) Facts(userID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.profile[userID]...)
}

// InMemoryPageFetcher implements memory.NotionPageFetcher for tests.
type InMemoryPageFetcher struct {
	mu    sync.Mutex
	pages map[string]string // pageID -> text
}

// NewInMemoryPageFetcher creates a fetcher that returns pre-seeded page text.
func NewInMemoryPageFetcher(pages map[string]string) *InMemoryPageFetcher {
	if pages == nil {
		pages = make(map[string]string)
	}
	return &InMemoryPageFetcher{pages: pages}
}

// GetPageText implements memory.NotionPageFetcher.
func (m *InMemoryPageFetcher) GetPageText(ctx context.Context, pageID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pages[pageID], nil
}

// FakeNotionKnowledgeStore implements memory.NotionKnowledgeWriter for tests (in-memory pages).
type FakeNotionKnowledgeStore struct {
	mu     sync.Mutex
	pages  map[string]string // pageID -> "title\ncontent" or full text
	nextID int
}

// NewFakeNotionKnowledgeStore creates a fake that stores pages in memory.
func NewFakeNotionKnowledgeStore() *FakeNotionKnowledgeStore {
	return &FakeNotionKnowledgeStore{pages: make(map[string]string), nextID: 1}
}

// CreatePage creates a fake page; returns a generated page ID (e.g. "page-1").
func (f *FakeNotionKnowledgeStore) CreatePage(ctx context.Context, title string, content string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := fmt.Sprintf("page-%d", f.nextID)
	f.nextID++
	if content != "" {
		f.pages[id] = title + "\n" + content
	} else {
		f.pages[id] = title
	}
	return id, nil
}

// AppendToPage appends content to an existing fake page.
func (f *FakeNotionKnowledgeStore) AppendToPage(ctx context.Context, pageID string, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages[pageID] = f.pages[pageID] + "\n" + content
	return nil
}

// GetPage returns the stored text for a page (for assertions).
func (f *FakeNotionKnowledgeStore) GetPage(pageID string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pages[pageID]
}

// PageIDs returns all page IDs (for assertions).
func (f *FakeNotionKnowledgeStore) PageIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, 0, len(f.pages))
	for id := range f.pages {
		ids = append(ids, id)
	}
	return ids
}

// GetPageText implements memory.NotionPageFetcher so the same fake can be used as fetcher for Search.
func (f *FakeNotionKnowledgeStore) GetPageText(ctx context.Context, pageID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pages[pageID], nil
}

// Ensure compile-time interface satisfaction.
var (
	_ llm.Client                   = (*ScriptedLLM)(nil)
	_ memory.UserFactStore         = (*InMemoryFactStore)(nil)
	_ memory.NotionPageFetcher     = (*InMemoryPageFetcher)(nil)
	_ memory.NotionKnowledgeWriter = (*FakeNotionKnowledgeStore)(nil)
	_ memory.NotionPageFetcher     = (*FakeNotionKnowledgeStore)(nil)
)
