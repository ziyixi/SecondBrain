package metrics

import (
	"math"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	store := NewStore()
	summary := store.Summary()
	if summary.TotalInteractions != 0 {
		t.Errorf("expected 0 interactions, got %d", summary.TotalInteractions)
	}
	if summary.KnowledgeCoverage != 0 {
		t.Errorf("expected 0 knowledge coverage, got %f", summary.KnowledgeCoverage)
	}
}

func TestStoreRecord(t *testing.T) {
	store := NewStore()

	store.Record(InteractionRecord{
		SessionID:        "s1",
		Timestamp:        time.Now(),
		Query:            "test query",
		ResponseQuality:  0.8,
		ContextRelevance: 0.9,
		Feedback:         FeedbackPositive,
		TopicDistribution: map[string]float64{
			"machine_learning": 0.7,
			"go_programming":   0.3,
		},
	})

	summary := store.Summary()
	if summary.TotalInteractions != 1 {
		t.Errorf("expected 1 interaction, got %d", summary.TotalInteractions)
	}
	if summary.AvgResponseQuality != 0.8 {
		t.Errorf("expected avg quality 0.8, got %f", summary.AvgResponseQuality)
	}
	if summary.AvgContextRelevance != 0.9 {
		t.Errorf("expected avg relevance 0.9, got %f", summary.AvgContextRelevance)
	}
	if summary.UserSatisfactionRate != 1.0 {
		t.Errorf("expected satisfaction rate 1.0, got %f", summary.UserSatisfactionRate)
	}
}

func TestUserSatisfactionRate(t *testing.T) {
	store := NewStore()

	// 3 positive, 1 negative, 1 correction = 3/5 = 0.6
	for i := 0; i < 3; i++ {
		store.Record(InteractionRecord{
			Feedback: FeedbackPositive,
		})
	}
	store.Record(InteractionRecord{Feedback: FeedbackNegative})
	store.Record(InteractionRecord{Feedback: FeedbackCorrection})

	summary := store.Summary()
	if math.Abs(summary.UserSatisfactionRate-0.6) > 0.001 {
		t.Errorf("expected satisfaction rate ~0.6, got %f", summary.UserSatisfactionRate)
	}
}

func TestKnowledgeCoverageUniform(t *testing.T) {
	store := NewStore()

	// Equal distribution across 4 topics = max entropy = 1.0
	topics := []string{"ml", "systems", "databases", "networks"}
	for _, topic := range topics {
		store.Record(InteractionRecord{
			TopicDistribution: map[string]float64{topic: 1.0},
		})
	}

	summary := store.Summary()
	if math.Abs(summary.KnowledgeCoverage-1.0) > 0.01 {
		t.Errorf("expected knowledge coverage ~1.0 for uniform distribution, got %f", summary.KnowledgeCoverage)
	}
}

func TestKnowledgeCoverageSkewed(t *testing.T) {
	store := NewStore()

	// Heavily skewed: 10 interactions on "ml", 1 on "systems"
	for i := 0; i < 10; i++ {
		store.Record(InteractionRecord{
			TopicDistribution: map[string]float64{"ml": 1.0},
		})
	}
	store.Record(InteractionRecord{
		TopicDistribution: map[string]float64{"systems": 1.0},
	})

	summary := store.Summary()
	// Should be low, around 0.44 for 10:1 ratio over 2 topics
	if summary.KnowledgeCoverage > 0.6 {
		t.Errorf("expected low knowledge coverage for skewed distribution, got %f", summary.KnowledgeCoverage)
	}
	if summary.KnowledgeCoverage <= 0 {
		t.Errorf("expected positive knowledge coverage, got %f", summary.KnowledgeCoverage)
	}
}

func TestKnowledgeCoverageSingleTopic(t *testing.T) {
	store := NewStore()

	store.Record(InteractionRecord{
		TopicDistribution: map[string]float64{"ml": 1.0},
	})

	summary := store.Summary()
	// Single topic = 0 entropy
	if summary.KnowledgeCoverage != 0 {
		t.Errorf("expected 0 knowledge coverage for single topic, got %f", summary.KnowledgeCoverage)
	}
}

func TestRecentQualityTrend(t *testing.T) {
	store := NewStore()

	// Add 5 records with increasing quality
	qualities := []float64{0.2, 0.4, 0.6, 0.8, 1.0}
	for _, q := range qualities {
		store.Record(InteractionRecord{ResponseQuality: q})
	}

	// Last 2: (0.8 + 1.0) / 2 = 0.9
	trend := store.RecentQualityTrend(2)
	if math.Abs(trend-0.9) > 0.001 {
		t.Errorf("expected recent trend ~0.9, got %f", trend)
	}

	// Last 5: (0.2+0.4+0.6+0.8+1.0)/5 = 0.6
	trend = store.RecentQualityTrend(5)
	if math.Abs(trend-0.6) > 0.001 {
		t.Errorf("expected recent trend ~0.6, got %f", trend)
	}

	// More than available: same as all
	trend = store.RecentQualityTrend(100)
	if math.Abs(trend-0.6) > 0.001 {
		t.Errorf("expected recent trend ~0.6, got %f", trend)
	}
}

func TestRecentQualityTrendEmpty(t *testing.T) {
	store := NewStore()
	trend := store.RecentQualityTrend(5)
	if trend != 0 {
		t.Errorf("expected 0 for empty store, got %f", trend)
	}
}

func TestConcurrentAccess(t *testing.T) {
	store := NewStore()
	done := make(chan bool, 10)

	// Concurrent writers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				store.Record(InteractionRecord{
					ResponseQuality: 0.5,
					Feedback:        FeedbackPositive,
					TopicDistribution: map[string]float64{
						"topic": 1.0,
					},
				})
			}
			done <- true
		}()
	}

	// Concurrent readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = store.Summary()
				_ = store.RecentQualityTrend(10)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	summary := store.Summary()
	if summary.TotalInteractions != 500 {
		t.Errorf("expected 500 interactions, got %d", summary.TotalInteractions)
	}
}
