package normalizer

import (
	"testing"
)

func TestStripHTML(t *testing.T) {
	n := New()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic HTML",
			input:    "<p>Hello <b>World</b></p>",
			expected: "Hello World",
		},
		{
			name:     "HTML entities",
			input:    "Hello&nbsp;World &amp; &lt;Friends&gt;",
			expected: "Hello World & <Friends>",
		},
		{
			name:     "nested tags",
			input:    "<div><span>Text</span></div>",
			expected: "Text",
		},
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text",
			input:    "Just plain text",
			expected: "Just plain text",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := n.StripHTML(tc.input)
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	n := New()

	content, meta := n.NormalizeEmail("Subject", "<p>Body</p>", true)
	if content != "Body" {
		t.Errorf("expected 'Body', got %q", content)
	}
	if meta["subject"] != "Subject" {
		t.Errorf("expected subject in metadata")
	}
	if meta["type"] != "email" {
		t.Errorf("expected type=email in metadata")
	}
}

func TestNormalizeEmailPlainText(t *testing.T) {
	n := New()

	content, _ := n.NormalizeEmail("Subject", "Plain body", false)
	if content != "Plain body" {
		t.Errorf("expected 'Plain body', got %q", content)
	}
}

func TestNormalizeSlackMessage(t *testing.T) {
	n := New()

	content, meta := n.NormalizeSlackMessage("Hello!", "#general", "user123")
	if content != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", content)
	}
	if meta["channel"] != "#general" {
		t.Errorf("expected channel=#general")
	}
	if meta["user"] != "user123" {
		t.Errorf("expected user=user123")
	}
}

func TestNormalizeGitHubWebhookPush(t *testing.T) {
	n := New()

	payload := map[string]interface{}{
		"commits": []interface{}{
			map[string]interface{}{
				"message": "Fix bug #42",
			},
		},
	}

	content, meta := n.NormalizeGitHubWebhook("push", payload)
	if content != "Fix bug #42" {
		t.Errorf("expected 'Fix bug #42', got %q", content)
	}
	if meta["event_type"] != "push" {
		t.Errorf("expected event_type=push")
	}
}

func TestNormalizeGitHubWebhookIssues(t *testing.T) {
	n := New()

	payload := map[string]interface{}{
		"issue": map[string]interface{}{
			"title": "Bug Report",
			"body":  "Steps to reproduce",
		},
	}

	content, _ := n.NormalizeGitHubWebhook("issues", payload)
	if content != "Bug Report: Steps to reproduce" {
		t.Errorf("unexpected content: %q", content)
	}
}
