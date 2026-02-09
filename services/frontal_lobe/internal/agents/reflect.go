package agents

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ziyixi/SecondBrain/services/frontal_lobe/internal/reasoning"
)

// WeeklyReviewResult holds the output of the Reflect agent.
type WeeklyReviewResult struct {
	ReportMarkdown      string
	StalledProjects     []string
	SuggestedNextActions []string
	DormantIdeas        []string
}

// ReflectAgent implements the "Reflect" agent for weekly reviews (PRD §6.2).
// It gathers data, synthesizes, and generates a review report.
type ReflectAgent struct {
	llm reasoning.LLMProvider
}

// NewReflectAgent creates a new ReflectAgent.
func NewReflectAgent(llm reasoning.LLMProvider) *ReflectAgent {
	return &ReflectAgent{llm: llm}
}

// GenerateWeeklyReview creates a weekly review report.
func (a *ReflectAgent) GenerateWeeklyReview(
	ctx context.Context,
	startDate, endDate time.Time,
	completedTasks, activeTasks, blockedTasks []string,
) (*WeeklyReviewResult, error) {

	// Build review prompt
	var sb strings.Builder
	sb.WriteString("Generate a weekly review report.\n\n")
	sb.WriteString(fmt.Sprintf("Period: %s to %s\n\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02")))

	sb.WriteString("Completed Tasks:\n")
	for _, t := range completedTasks {
		sb.WriteString(fmt.Sprintf("- %s\n", t))
	}

	sb.WriteString("\nActive Tasks:\n")
	for _, t := range activeTasks {
		sb.WriteString(fmt.Sprintf("- %s\n", t))
	}

	sb.WriteString("\nBlocked Tasks:\n")
	for _, t := range blockedTasks {
		sb.WriteString(fmt.Sprintf("- %s\n", t))
	}

	report, err := a.llm.Generate(ctx, sb.String())
	if err != nil {
		return nil, fmt.Errorf("generating review: %w", err)
	}

	// Identify stalled projects (blocked tasks indicate stalling)
	var stalled []string
	for _, task := range blockedTasks {
		stalled = append(stalled, task)
	}

	// Suggest next actions
	var nextActions []string
	if len(blockedTasks) > 0 {
		nextActions = append(nextActions, "Review and unblock stalled tasks")
	}
	if len(activeTasks) > 5 {
		nextActions = append(nextActions, "Consider prioritizing — too many active tasks")
	}
	nextActions = append(nextActions, "Review the Weekly Report and confirm next week's priorities")

	// Surface dormant ideas (exploration per PRD §7.2)
	dormant := []string{
		"Consider revisiting archived research on weak signal detection",
		"Review dormant project ideas from last quarter",
	}

	return &WeeklyReviewResult{
		ReportMarkdown:       report,
		StalledProjects:      stalled,
		SuggestedNextActions: nextActions,
		DormantIdeas:         dormant,
	}, nil
}
