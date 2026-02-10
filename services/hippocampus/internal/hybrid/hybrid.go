package hybrid

import (
	"sort"
)

// RankedResult represents a search result from any search backend.
type RankedResult struct {
	ID       string
	Score    float64
	Content  string
	Metadata map[string]string
}

// ReciprocalRankFusion combines multiple ranked result lists using RRF.
// Inspired by qmd's hybrid search pipeline that fuses BM25 + vector results.
//
// The RRF formula: score(d) = Σ weight_i / (k + rank_i(d))
// where k is a constant (typically 60) and rank starts at 1.
//
// Parameters:
//   - rankedLists: multiple result lists from different search backends
//   - weights: weight for each list (e.g., 2.0 for original query, 1.0 for expanded)
//   - k: ranking constant (typically 60)
func ReciprocalRankFusion(rankedLists [][]RankedResult, weights []float64, k float64) []RankedResult {
	if k <= 0 {
		k = 60
	}

	// Fill default weights if not provided
	if len(weights) == 0 {
		weights = make([]float64, len(rankedLists))
		for i := range weights {
			weights[i] = 1.0
		}
	}

	// Accumulate RRF scores per document ID
	type docInfo struct {
		id       string
		score    float64
		content  string
		metadata map[string]string
		bestRank int // best rank across all lists
	}

	docs := make(map[string]*docInfo)

	for listIdx, list := range rankedLists {
		weight := 1.0
		if listIdx < len(weights) {
			weight = weights[listIdx]
		}

		for rank, result := range list {
			rrfScore := weight / (k + float64(rank+1))

			if existing, ok := docs[result.ID]; ok {
				existing.score += rrfScore
				if rank+1 < existing.bestRank {
					existing.bestRank = rank + 1
					existing.content = result.Content
					existing.metadata = result.Metadata
				}
			} else {
				docs[result.ID] = &docInfo{
					id:       result.ID,
					score:    rrfScore,
					content:  result.Content,
					metadata: result.Metadata,
					bestRank: rank + 1,
				}
			}
		}
	}

	// Apply top-rank bonus (inspired by qmd)
	// #1 in any list gets +0.05, #2-3 gets +0.02
	for _, list := range rankedLists {
		for rank, result := range list {
			if doc, ok := docs[result.ID]; ok {
				switch {
				case rank == 0:
					doc.score += 0.05
				case rank <= 2:
					doc.score += 0.02
				}
			}
		}
	}

	// Sort by RRF score
	var results []RankedResult
	for _, doc := range docs {
		results = append(results, RankedResult{
			ID:       doc.id,
			Score:    doc.score,
			Content:  doc.content,
			Metadata: doc.metadata,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

// NormalizeScores normalizes scores in a result list to [0, 1] range.
func NormalizeScores(results []RankedResult) []RankedResult {
	if len(results) == 0 {
		return results
	}

	maxScore := results[0].Score
	for _, r := range results {
		if r.Score > maxScore {
			maxScore = r.Score
		}
	}

	if maxScore <= 0 {
		return results
	}

	normalized := make([]RankedResult, len(results))
	for i, r := range results {
		normalized[i] = RankedResult{
			ID:       r.ID,
			Score:    r.Score / maxScore,
			Content:  r.Content,
			Metadata: r.Metadata,
		}
	}
	return normalized
}
