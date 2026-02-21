package memory

import (
	"sync"

	"github.com/yourusername/secondbrain/internal/config"
)

// WorkingMemory holds the last N messages of the current conversation.
type workingMemory struct {
	mu      sync.Mutex
	n       int
	entries []struct{ Role, Content string }
}

// NewWorkingMemory creates a working memory that keeps the last n messages.
// If n <= 0, uses config (WORKING_MEMORY_SIZE, default 20).
func NewWorkingMemory(n int) WorkingMemory {
	if n <= 0 {
		n = config.WorkingMemorySize()
	}
	return &workingMemory{n: n, entries: make([]struct{ Role, Content string }, 0, n*2)}
}

func (w *workingMemory) Append(role, content string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = append(w.entries, struct{ Role, Content string }{role, content})
	if len(w.entries) > w.n {
		w.entries = w.entries[len(w.entries)-w.n:]
	}
}

func (w *workingMemory) Messages() []struct{ Role, Content string } {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]struct{ Role, Content string }, len(w.entries))
	copy(out, w.entries)
	return out
}

func (w *workingMemory) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = w.entries[:0]
}
