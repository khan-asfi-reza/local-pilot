package graph

// RankOpts configures PageRank.
type RankOpts struct {
	Damping     float64
	Iters       int
	Personalize map[NodeID]float64 // task-relevant seeds (grounding targets, changed files)
}

// Rank runs weighted PageRank over the call/reference/import edges and returns a
// score per node. Personalization biases the ranking toward the code the current
// task touches, so the digest foregrounds relevant symbols (aider's approach).
func (g *Graph) Rank(opts RankOpts) map[NodeID]float64 {
	if opts.Damping == 0 {
		opts.Damping = 0.85
	}
	if opts.Iters == 0 {
		opts.Iters = 25
	}
	ids := make([]NodeID, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	n := len(ids)
	if n == 0 {
		return map[NodeID]float64{}
	}

	// Personalization vector p (normalized), used for teleport + dangling mass.
	p := make(map[NodeID]float64, n)
	var psum float64
	for id, w := range opts.Personalize {
		if _, ok := g.Nodes[id]; ok && w > 0 {
			p[id] = w
			psum += w
		}
	}
	if psum == 0 {
		for _, id := range ids {
			p[id] = 1.0 / float64(n)
		}
	} else {
		for id := range p {
			p[id] /= psum
		}
	}

	// Weighted out-degree per node.
	outW := make(map[NodeID]float64, n)
	for _, e := range g.Edges {
		outW[e.From] += edgeWeight(e.Kind)
	}

	rank := make(map[NodeID]float64, n)
	for _, id := range ids {
		rank[id] = 1.0 / float64(n)
	}

	d := opts.Damping
	for it := 0; it < opts.Iters; it++ {
		next := make(map[NodeID]float64, n)
		// teleport
		for _, id := range ids {
			next[id] = (1 - d) * p[id]
		}
		// dangling mass (nodes with no out-edges) redistributed by p
		var dangling float64
		for _, id := range ids {
			if outW[id] == 0 {
				dangling += rank[id]
			}
		}
		for _, id := range ids {
			next[id] += d * dangling * p[id]
		}
		// edge contributions
		for _, e := range g.Edges {
			w := edgeWeight(e.Kind)
			if outW[e.From] > 0 {
				next[e.To] += d * (w / outW[e.From]) * rank[e.From]
			}
		}
		rank = next
	}
	return rank
}

// edgeWeight weights edge kinds: a direct call is the strongest structural signal.
func edgeWeight(k EdgeKind) float64 {
	switch k {
	case EdgeCalls:
		return 1.0
	case EdgeReferences:
		return 0.8
	case EdgeImports:
		return 0.5
	default: // contains
		return 0.1
	}
}
