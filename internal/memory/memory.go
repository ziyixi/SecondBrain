package memory

import (
	"context"
)

// WorkingMemory holds the last N messages of the current conversation.
type WorkingMemory interface {
	// Append adds a message and keeps only the last N.
	Append(role, content string)
	// Messages returns the current window of messages.
	Messages() []struct{ Role, Content string }
	// Clear resets the window (e.g. new conversation).
	Clear()
}

// UserFactStore persists user facts (e.g. to a Notion "User Profile" page).
type UserFactStore interface {
	// GetProfile returns the full text of the user profile (injected into system prompt).
	GetProfile(ctx context.Context, userID string) (string, error)
	// AppendFact appends a fact to the user's profile.
	AppendFact(ctx context.Context, userID string, fact string) error
}

// DocumentHit represents a retrieved document from the knowledge base.
type DocumentHit struct {
	NotionPageID string
	Text         string
	Score        float64
}

// KnowledgeBase provides hybrid search over documents (e.g. Qdrant + Notion).
type KnowledgeBase interface {
	// Search runs hybrid search (dense + sparse) and returns top hits; text may be truncated.
	Search(ctx context.Context, query string, limit int) ([]DocumentHit, error)
	// Upsert stores or updates a document by Notion page ID (embedding generated lazily on first search if needed).
	Upsert(ctx context.Context, notionPageID string, text string) error
}
