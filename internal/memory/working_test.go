package memory

import (
	"testing"
)

func TestNewWorkingMemory(t *testing.T) {
	w := NewWorkingMemory(3)
	if w == nil {
		t.Fatal("NewWorkingMemory(3) returned nil")
	}
	msgs := w.Messages()
	if len(msgs) != 0 {
		t.Errorf("new working memory should have 0 messages, got %d", len(msgs))
	}
}

func TestWorkingMemory_AppendAndMessages(t *testing.T) {
	w := NewWorkingMemory(3)
	w.Append("user", "Hello")
	w.Append("assistant", "Hi there")
	msgs := w.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Hello" {
		t.Errorf("first message = %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "Hi there" {
		t.Errorf("second message = %+v", msgs[1])
	}
}

func TestWorkingMemory_KeepsLastN(t *testing.T) {
	w := NewWorkingMemory(2)
	w.Append("user", "1")
	w.Append("assistant", "2")
	w.Append("user", "3")
	msgs := w.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (last N), got %d", len(msgs))
	}
	if msgs[0].Content != "2" || msgs[1].Content != "3" {
		t.Errorf("expected last two messages 2 and 3, got %+v", msgs)
	}
}

func TestWorkingMemory_Clear(t *testing.T) {
	w := NewWorkingMemory(5)
	w.Append("user", "a")
	w.Append("assistant", "b")
	w.Clear()
	msgs := w.Messages()
	if len(msgs) != 0 {
		t.Errorf("after Clear expected 0 messages, got %d", len(msgs))
	}
}
