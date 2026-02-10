package hybrid

import (
	"testing"
)

func TestReciprocalRankFusionBasic(t *testing.T) {
	bm25Results := []RankedResult{
		{ID: "doc1", Score: 1.0, Content: "doc1 content"},
		{ID: "doc2", Score: 0.8, Content: "doc2 content"},
		{ID: "doc3", Score: 0.5, Content: "doc3 content"},
	}

	vectorResults := []RankedResult{
		{ID: "doc2", Score: 0.95, Content: "doc2 content"},
		{ID: "doc4", Score: 0.7, Content: "doc4 content"},
		{ID: "doc1", Score: 0.6, Content: "doc1 content"},
	}

	results := ReciprocalRankFusion(
		[][]RankedResult{bm25Results, vectorResults},
		[]float64{1.0, 1.0},
		60,
	)

	if len(results) == 0 {
		t.Fatal("expected results")
	}

	// doc1 and doc2 appear in both lists, should score highest
	topIDs := make(map[string]bool)
	for i := 0; i < 2 && i < len(results); i++ {
		topIDs[results[i].ID] = true
	}
	if !topIDs["doc1"] || !topIDs["doc2"] {
		t.Errorf("expected doc1 and doc2 in top 2, got %v and %v", results[0].ID, results[1].ID)
	}
}

func TestReciprocalRankFusionWithWeights(t *testing.T) {
	list1 := []RankedResult{
		{ID: "doc1", Score: 1.0, Content: "a"},
	}
	list2 := []RankedResult{
		{ID: "doc2", Score: 1.0, Content: "b"},
	}

	// Give list1 double the weight
	results := ReciprocalRankFusion(
		[][]RankedResult{list1, list2},
		[]float64{2.0, 1.0},
		60,
	)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// doc1 should score higher due to higher weight
	if results[0].ID != "doc1" {
		t.Errorf("expected doc1 first with higher weight, got %q", results[0].ID)
	}
}

func TestReciprocalRankFusionTopRankBonus(t *testing.T) {
	// doc1 is #1 in list, should get +0.05 bonus
	list := []RankedResult{
		{ID: "doc1", Score: 1.0, Content: "a"},
		{ID: "doc2", Score: 0.9, Content: "b"},
		{ID: "doc3", Score: 0.8, Content: "c"},
		{ID: "doc4", Score: 0.7, Content: "d"},
	}

	results := ReciprocalRankFusion(
		[][]RankedResult{list},
		[]float64{1.0},
		60,
	)

	// doc1 should have higher score than others due to #1 bonus
	if len(results) < 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	if results[0].ID != "doc1" {
		t.Errorf("expected doc1 first, got %q", results[0].ID)
	}
}

func TestReciprocalRankFusionEmpty(t *testing.T) {
	results := ReciprocalRankFusion(nil, nil, 60)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestReciprocalRankFusionDefaultWeights(t *testing.T) {
	list := []RankedResult{
		{ID: "doc1", Score: 1.0, Content: "a"},
	}

	results := ReciprocalRankFusion(
		[][]RankedResult{list},
		nil, // no weights provided
		60,
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestNormalizeScores(t *testing.T) {
	results := []RankedResult{
		{ID: "a", Score: 10},
		{ID: "b", Score: 5},
		{ID: "c", Score: 2.5},
	}

	normalized := NormalizeScores(results)

	if len(normalized) != 3 {
		t.Fatalf("expected 3 results, got %d", len(normalized))
	}

	if normalized[0].Score != 1.0 {
		t.Errorf("expected max score 1.0, got %f", normalized[0].Score)
	}
	if normalized[1].Score != 0.5 {
		t.Errorf("expected score 0.5, got %f", normalized[1].Score)
	}
	if normalized[2].Score != 0.25 {
		t.Errorf("expected score 0.25, got %f", normalized[2].Score)
	}
}

func TestNormalizeScoresEmpty(t *testing.T) {
	results := NormalizeScores(nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
