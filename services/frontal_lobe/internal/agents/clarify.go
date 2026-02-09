package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/reasoning"
)

// State represents a state in the agent state machine.
type State string

const (
	StateClassify  State = "CLASSIFY"
	StateExtract   State = "EXTRACT"
	StateSummarize State = "SUMMARIZE"
	StateRoute     State = "ROUTE"
	StateExecute   State = "EXECUTE"
	StateRepair    State = "REPAIR"
	StateDelete    State = "DELETE"
	StateEnd       State = "END"
)

// ClarifyResult holds the output of the Clarify agent.
type ClarifyResult struct {
	Classification    string
	SuggestedProject  string
	SuggestedArea     string
	Priority          string
	ExtractedMetadata map[string]string
	Confidence        float64
	ThoughtChain      []string
}

// ClarifyAgent implements the "Clarify" agent state machine from PRD §6.1.
// It processes inbox items through: CLASSIFY → EXTRACT/SUMMARIZE/DELETE → ROUTE → EXECUTE.
type ClarifyAgent struct {
	llm reasoning.LLMProvider
}

// NewClarifyAgent creates a new ClarifyAgent.
func NewClarifyAgent(llm reasoning.LLMProvider) *ClarifyAgent {
	return &ClarifyAgent{llm: llm}
}

// Process runs the state machine on the given content.
func (a *ClarifyAgent) Process(ctx context.Context, content, source string, metadata map[string]string) (*ClarifyResult, error) {
	result := &ClarifyResult{
		ExtractedMetadata: make(map[string]string),
		ThoughtChain:      make([]string, 0),
	}

	state := StateClassify

	for state != StateEnd {
		switch state {
		case StateClassify:
			result.ThoughtChain = append(result.ThoughtChain, "Analyzing content for classification...")

			classification, confidence, err := a.llm.Classify(ctx, content, []string{"ACTIONABLE", "REFERENCE", "TRASH"})
			if err != nil {
				return nil, fmt.Errorf("classification failed: %w", err)
			}

			result.Classification = classification
			result.Confidence = confidence
			result.ThoughtChain = append(result.ThoughtChain,
				fmt.Sprintf("Classified as %s with confidence %.2f", classification, confidence))

			switch classification {
			case "ACTIONABLE":
				state = StateExtract
			case "REFERENCE":
				state = StateSummarize
			case "TRASH":
				state = StateDelete
			default:
				state = StateSummarize
			}

		case StateExtract:
			result.ThoughtChain = append(result.ThoughtChain, "Extracting structured metadata...")

			prompt := fmt.Sprintf("Extract key metadata from this %s content: %s", source, truncate(content, 500))
			extracted, err := a.llm.Generate(ctx, prompt)
			if err != nil {
				return nil, fmt.Errorf("extraction failed: %w", err)
			}

			result.ExtractedMetadata["extracted"] = extracted
			result.Priority = determinePriority(content)
			state = StateRoute

		case StateSummarize:
			result.ThoughtChain = append(result.ThoughtChain, "Summarizing reference content...")

			prompt := fmt.Sprintf("Summarize this content: %s", truncate(content, 500))
			summary, err := a.llm.Generate(ctx, prompt)
			if err != nil {
				return nil, fmt.Errorf("summarization failed: %w", err)
			}

			result.ExtractedMetadata["summary"] = summary
			result.Priority = "NORMAL"
			state = StateRoute

		case StateRoute:
			result.ThoughtChain = append(result.ThoughtChain, "Determining destination area...")

			result.SuggestedArea = determineArea(content, source)
			result.SuggestedProject = determineProject(content)
			result.ThoughtChain = append(result.ThoughtChain,
				fmt.Sprintf("Routing to area: %s, project: %s", result.SuggestedArea, result.SuggestedProject))
			state = StateExecute

		case StateExecute:
			result.ThoughtChain = append(result.ThoughtChain, "Filing item to destination...")
			state = StateEnd

		case StateDelete:
			result.ThoughtChain = append(result.ThoughtChain, "Marking item for deletion...")
			result.Priority = "LOW"
			state = StateEnd

		case StateRepair:
			result.ThoughtChain = append(result.ThoughtChain, "Attempting repair after error...")
			state = StateEnd

		default:
			state = StateEnd
		}
	}

	return result, nil
}

func determinePriority(content string) string {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "urgent") || strings.Contains(lower, "asap") {
		return "URGENT"
	}
	if strings.Contains(lower, "important") || strings.Contains(lower, "deadline") {
		return "IMPORTANT"
	}
	return "NORMAL"
}

func determineArea(content, source string) string {
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "finance") || strings.Contains(lower, "bank") || strings.Contains(lower, "payment"):
		return "Financial Health"
	case strings.Contains(lower, "research") || strings.Contains(lower, "paper") || strings.Contains(lower, "study"):
		return "Academic Publishing"
	case strings.Contains(lower, "lease") || strings.Contains(lower, "rent") || strings.Contains(lower, "housing"):
		return "Housing"
	case strings.Contains(lower, "code") || strings.Contains(lower, "bug") || strings.Contains(lower, "deploy"):
		return "Engineering"
	default:
		return "General"
	}
}

func determineProject(content string) string {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "phasenet") || strings.Contains(lower, "seismic") {
		return "PhaseNet-TF Extensions"
	}
	if strings.Contains(lower, "second brain") || strings.Contains(lower, "cognitive") {
		return "Second Brain Development"
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
