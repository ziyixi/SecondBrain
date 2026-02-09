package metrics

import (
	"math"
	"sync"
	"time"
)

// FeedbackType represents the type of user feedback.
type FeedbackType string

const (
	FeedbackPositive   FeedbackType = "positive"
	FeedbackNegative   FeedbackType = "negative"
	FeedbackCorrection FeedbackType = "correction"
)

// InteractionRecord captures a single interaction for metrics computation.
type InteractionRecord struct {
	SessionID        string
	Timestamp        time.Time
	Query            string
	ResponseQuality  float64      // [0,1] estimated quality based on context relevance
	ContextRelevance float64      // [0,1] how relevant the retrieved context was
	Feedback         FeedbackType // user feedback if available
	TopicDistribution map[string]float64 // topic -> weight, for entropy calculation
}

// Store tracks feedback metrics and computes knowledge coverage indicators.
type Store struct {
	mu          sync.RWMutex
	records     []InteractionRecord
	topicCounts map[string]int
	feedbackCounts map[FeedbackType]int
	totalInteractions int
}

// NewStore creates a new metrics store.
func NewStore() *Store {
	return &Store{
		records:        make([]InteractionRecord, 0),
		topicCounts:    make(map[string]int),
		feedbackCounts: make(map[FeedbackType]int),
	}
}

// Record adds a new interaction record.
func (s *Store) Record(rec InteractionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = append(s.records, rec)
	s.totalInteractions++

	if rec.Feedback != "" {
		s.feedbackCounts[rec.Feedback]++
	}

	for topic, weight := range rec.TopicDistribution {
		if weight > 0 {
			s.topicCounts[topic]++
		}
	}
}

// Summary returns the current metrics summary.
func (s *Store) Summary() MetricsSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := MetricsSummary{
		TotalInteractions: s.totalInteractions,
		FeedbackCounts:    make(map[FeedbackType]int),
		TopicCoverage:     make(map[string]int),
	}

	for k, v := range s.feedbackCounts {
		summary.FeedbackCounts[k] = v
	}
	for k, v := range s.topicCounts {
		summary.TopicCoverage[k] = v
	}

	// Compute aggregate scores
	if len(s.records) > 0 {
		var totalQuality, totalRelevance float64
		for _, rec := range s.records {
			totalQuality += rec.ResponseQuality
			totalRelevance += rec.ContextRelevance
		}
		n := float64(len(s.records))
		summary.AvgResponseQuality = totalQuality / n
		summary.AvgContextRelevance = totalRelevance / n
	}

	// User satisfaction rate: positive / (positive + negative + correction)
	totalFeedback := s.feedbackCounts[FeedbackPositive] +
		s.feedbackCounts[FeedbackNegative] +
		s.feedbackCounts[FeedbackCorrection]
	if totalFeedback > 0 {
		summary.UserSatisfactionRate = float64(s.feedbackCounts[FeedbackPositive]) / float64(totalFeedback)
	}

	// Knowledge coverage score (normalized entropy of topic distribution)
	summary.KnowledgeCoverage = s.computeKnowledgeCoverage()

	return summary
}

// MetricsSummary provides aggregated metrics.
type MetricsSummary struct {
	TotalInteractions    int                  `json:"total_interactions"`
	AvgResponseQuality   float64              `json:"avg_response_quality"`
	AvgContextRelevance  float64              `json:"avg_context_relevance"`
	UserSatisfactionRate float64              `json:"user_satisfaction_rate"`
	KnowledgeCoverage    float64              `json:"knowledge_coverage"`
	FeedbackCounts       map[FeedbackType]int `json:"feedback_counts"`
	TopicCoverage        map[string]int       `json:"topic_coverage"`
}

// computeKnowledgeCoverage calculates the normalized Shannon entropy of the
// topic distribution across all interactions. This is an information-theoretic
// measure of how evenly the system's knowledge is distributed across topics.
//
// H_norm = -sum(p_i * log2(p_i)) / log2(N)
//
// where p_i is the proportion of interactions involving topic i, and N is the
// total number of distinct topics. A value close to 1.0 means the system has
// broad, even coverage; a value close to 0 means it is concentrated on a few
// topics. This metric helps detect "degenerate feedback loops" (per Chip Huyen)
// where the system over-specializes.
func (s *Store) computeKnowledgeCoverage() float64 {
	n := len(s.topicCounts)
	if n <= 1 {
		return 0
	}

	total := 0
	for _, count := range s.topicCounts {
		total += count
	}
	if total == 0 {
		return 0
	}

	var entropy float64
	totalF := float64(total)
	for _, count := range s.topicCounts {
		if count > 0 {
			p := float64(count) / totalF
			entropy -= p * math.Log2(p)
		}
	}

	// Normalize by max possible entropy (uniform distribution)
	maxEntropy := math.Log2(float64(n))
	if maxEntropy == 0 {
		return 0
	}

	return entropy / maxEntropy
}

// RecentQualityTrend returns the average response quality for the last n
// interactions, useful for tracking whether the system is improving.
func (s *Store) RecentQualityTrend(n int) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.records) == 0 {
		return 0
	}

	start := len(s.records) - n
	if start < 0 {
		start = 0
	}

	var total float64
	count := 0
	for _, rec := range s.records[start:] {
		total += rec.ResponseQuality
		count++
	}

	if count == 0 {
		return 0
	}
	return total / float64(count)
}
