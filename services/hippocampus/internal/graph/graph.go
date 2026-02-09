package graph

import (
	"sync"
)

// Triple represents a subject-predicate-object triple.
type Triple struct {
	Subject   string
	Predicate string
	Object    string
	Metadata  map[string]string
}

// Node represents a node in the knowledge graph.
type Node struct {
	ID         string
	Label      string
	Properties map[string]string
}

// Edge represents a directed edge in the knowledge graph.
type Edge struct {
	Source       string
	Target       string
	Relationship string
	Properties   map[string]string
}

// KnowledgeGraph is an in-memory directed graph for storing
// entity relationships (subject-predicate-object triples).
type KnowledgeGraph struct {
	mu    sync.RWMutex
	nodes map[string]Node
	edges []Edge
	adj   map[string][]int // node -> edge indices (outgoing)
	inAdj map[string][]int // node -> edge indices (incoming)
}

// New creates a new KnowledgeGraph.
func New() *KnowledgeGraph {
	return &KnowledgeGraph{
		nodes: make(map[string]Node),
		adj:   make(map[string][]int),
		inAdj: make(map[string][]int),
	}
}

// AddTriple adds a triple to the graph.
func (g *KnowledgeGraph) AddTriple(t Triple) string {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Ensure nodes exist
	if _, ok := g.nodes[t.Subject]; !ok {
		g.nodes[t.Subject] = Node{ID: t.Subject, Label: t.Subject, Properties: make(map[string]string)}
	}
	if _, ok := g.nodes[t.Object]; !ok {
		g.nodes[t.Object] = Node{ID: t.Object, Label: t.Object, Properties: make(map[string]string)}
	}

	edge := Edge{
		Source:       t.Subject,
		Target:       t.Object,
		Relationship: t.Predicate,
		Properties:   t.Metadata,
	}

	idx := len(g.edges)
	g.edges = append(g.edges, edge)
	g.adj[t.Subject] = append(g.adj[t.Subject], idx)
	g.inAdj[t.Object] = append(g.inAdj[t.Object], idx)

	return t.Subject + "-" + t.Predicate + "-" + t.Object
}

// Query performs a BFS from an entity up to maxHops with optional relationship filter.
func (g *KnowledgeGraph) Query(entity string, maxHops int, relationshipFilter string) ([]Node, []Edge) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, ok := g.nodes[entity]; !ok {
		return nil, nil
	}

	visited := make(map[string]bool)
	visited[entity] = true
	visitedEdges := make(map[int]bool)

	type queueItem struct {
		nodeID string
		depth  int
	}

	queue := []queueItem{{entity, 0}}
	var resultEdges []Edge

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.depth >= maxHops {
			continue
		}

		// Outgoing edges
		for _, idx := range g.adj[current.nodeID] {
			if visitedEdges[idx] {
				continue
			}
			edge := g.edges[idx]
			if relationshipFilter != "" && edge.Relationship != relationshipFilter {
				continue
			}
			visitedEdges[idx] = true
			resultEdges = append(resultEdges, edge)
			if !visited[edge.Target] {
				visited[edge.Target] = true
				queue = append(queue, queueItem{edge.Target, current.depth + 1})
			}
		}

		// Incoming edges
		for _, idx := range g.inAdj[current.nodeID] {
			if visitedEdges[idx] {
				continue
			}
			edge := g.edges[idx]
			if relationshipFilter != "" && edge.Relationship != relationshipFilter {
				continue
			}
			visitedEdges[idx] = true
			resultEdges = append(resultEdges, edge)
			if !visited[edge.Source] {
				visited[edge.Source] = true
				queue = append(queue, queueItem{edge.Source, current.depth + 1})
			}
		}
	}

	var resultNodes []Node
	for id := range visited {
		if n, ok := g.nodes[id]; ok {
			resultNodes = append(resultNodes, n)
		}
	}

	return resultNodes, resultEdges
}

// TriplesCount returns the number of edges.
func (g *KnowledgeGraph) TriplesCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.edges)
}

// NodesCount returns the number of nodes.
func (g *KnowledgeGraph) NodesCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}
