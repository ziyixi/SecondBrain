package graph

import (
	"testing"
)

func TestAddTripleAndQuery(t *testing.T) {
	g := New()

	id := g.AddTriple(Triple{
		Subject:   "PhaseNet-TF",
		Predicate: "extends",
		Object:    "PhaseNet",
	})

	if id != "PhaseNet-TF-extends-PhaseNet" {
		t.Errorf("unexpected triple ID: %q", id)
	}

	if g.NodesCount() != 2 {
		t.Errorf("expected 2 nodes, got %d", g.NodesCount())
	}

	if g.TriplesCount() != 1 {
		t.Errorf("expected 1 triple, got %d", g.TriplesCount())
	}
}

func TestQuerySingleHop(t *testing.T) {
	g := New()
	g.AddTriple(Triple{Subject: "A", Predicate: "connects", Object: "B"})
	g.AddTriple(Triple{Subject: "B", Predicate: "connects", Object: "C"})
	g.AddTriple(Triple{Subject: "C", Predicate: "connects", Object: "D"})

	nodes, edges := g.Query("A", 1, "")
	if len(nodes) != 2 { // A, B
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
	if len(edges) != 1 { // A->B
		t.Errorf("expected 1 edge, got %d", len(edges))
	}
}

func TestQueryMultiHop(t *testing.T) {
	g := New()
	g.AddTriple(Triple{Subject: "A", Predicate: "knows", Object: "B"})
	g.AddTriple(Triple{Subject: "B", Predicate: "knows", Object: "C"})
	g.AddTriple(Triple{Subject: "C", Predicate: "knows", Object: "D"})

	nodes, edges := g.Query("A", 3, "")
	if len(nodes) != 4 { // A, B, C, D
		t.Errorf("expected 4 nodes, got %d", len(nodes))
	}
	if len(edges) != 3 { // A->B, B->C, C->D
		t.Errorf("expected 3 edges, got %d", len(edges))
	}
}

func TestQueryWithRelationshipFilter(t *testing.T) {
	g := New()
	g.AddTriple(Triple{Subject: "Person", Predicate: "works_at", Object: "Google"})
	g.AddTriple(Triple{Subject: "Person", Predicate: "lives_in", Object: "NYC"})
	g.AddTriple(Triple{Subject: "Person", Predicate: "knows", Object: "Friend"})

	nodes, edges := g.Query("Person", 1, "works_at")
	if len(edges) != 1 {
		t.Errorf("expected 1 edge with filter, got %d", len(edges))
	}
	if len(nodes) != 2 { // Person, Google
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestQueryNonExistentEntity(t *testing.T) {
	g := New()
	g.AddTriple(Triple{Subject: "A", Predicate: "links", Object: "B"})

	nodes, edges := g.Query("Z", 2, "")
	if nodes != nil {
		t.Errorf("expected nil nodes, got %v", nodes)
	}
	if edges != nil {
		t.Errorf("expected nil edges, got %v", edges)
	}
}

func TestQueryBidirectional(t *testing.T) {
	g := New()
	g.AddTriple(Triple{Subject: "A", Predicate: "parent", Object: "B"})

	// Query from B should find A via incoming edge
	nodes, edges := g.Query("B", 1, "")
	if len(nodes) != 2 { // B, A
		t.Errorf("expected 2 nodes (bidirectional), got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(edges))
	}
}

func TestGraphCounters(t *testing.T) {
	g := New()

	if g.NodesCount() != 0 {
		t.Errorf("expected 0 nodes initially")
	}
	if g.TriplesCount() != 0 {
		t.Errorf("expected 0 triples initially")
	}

	g.AddTriple(Triple{Subject: "X", Predicate: "rel", Object: "Y"})
	g.AddTriple(Triple{Subject: "X", Predicate: "rel2", Object: "Z"})

	if g.NodesCount() != 3 {
		t.Errorf("expected 3 nodes, got %d", g.NodesCount())
	}
	if g.TriplesCount() != 2 {
		t.Errorf("expected 2 triples, got %d", g.TriplesCount())
	}
}
