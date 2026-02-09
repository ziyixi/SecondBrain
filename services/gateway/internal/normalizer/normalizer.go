package normalizer

import (
	"regexp"
	"strings"
)

// Normalizer converts raw payloads from different sources into
// a standardized format suitable for the InboxItem Protobuf message.
type Normalizer struct{}

// New creates a new Normalizer.
func New() *Normalizer {
	return &Normalizer{}
}

var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

// StripHTML removes HTML tags from a string, returning plain text.
func (n *Normalizer) StripHTML(html string) string {
	text := htmlTagRegex.ReplaceAllString(html, "")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	// Collapse whitespace
	spaceRegex := regexp.MustCompile(`\s+`)
	text = spaceRegex.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// NormalizeEmail extracts clean text content from an email payload.
func (n *Normalizer) NormalizeEmail(subject, body string, isHTML bool) (string, map[string]string) {
	content := body
	if isHTML {
		content = n.StripHTML(body)
	}

	metadata := map[string]string{
		"subject": subject,
		"type":    "email",
	}

	return content, metadata
}

// NormalizeSlackMessage normalizes a Slack message payload.
func (n *Normalizer) NormalizeSlackMessage(text, channel, user string) (string, map[string]string) {
	metadata := map[string]string{
		"channel": channel,
		"user":    user,
		"type":    "slack",
	}

	return text, metadata
}

// NormalizeGitHubWebhook normalizes a GitHub webhook payload.
func (n *Normalizer) NormalizeGitHubWebhook(eventType string, payload map[string]interface{}) (string, map[string]string) {
	metadata := map[string]string{
		"event_type": eventType,
		"type":       "github",
	}

	// Extract relevant content based on event type
	var content string
	switch eventType {
	case "push":
		if commits, ok := payload["commits"].([]interface{}); ok && len(commits) > 0 {
			if commit, ok := commits[0].(map[string]interface{}); ok {
				content = getString(commit, "message")
			}
		}
	case "issues":
		if issue, ok := payload["issue"].(map[string]interface{}); ok {
			content = getString(issue, "title") + ": " + getString(issue, "body")
		}
	case "pull_request":
		if pr, ok := payload["pull_request"].(map[string]interface{}); ok {
			content = getString(pr, "title") + ": " + getString(pr, "body")
		}
	default:
		content = eventType + " event received"
	}

	return content, metadata
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
