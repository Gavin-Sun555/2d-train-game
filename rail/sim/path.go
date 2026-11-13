package sim

import (
	"container/heap"
	"math"
)

// MaxTurnDeg is the sharpest change of direction a vehicle can make at a node.
// Straight segments meeting at a node stand in for curves for now, so a
// right-angle street corner is allowed but reversing or acute turns are not.
const MaxTurnDeg = 90

// TurnOK reports whether a vehicle on step a may continue onto step b.
func (n *Network) TurnOK(a, b Step, maxTurnDeg float64) bool {
	if a.Edge == b.Edge {
		return false
	}
	da, db := n.Dir(a), n.Dir(b)
	cos := da.X*db.X + da.Y*db.Y
	return cos >= math.Cos(maxTurnDeg*math.Pi/180)-1e-9
}

// Route finds the shortest path of steps from node `from` to node `to`
// respecting the turn limit. If prev is non-nil the first step must also be a
// legal continuation of prev (the train arrives on prev and does not reverse).
// Paths never change level except through an edge that is itself a ramp.
func (n *Network) Route(prev *Step, from, to NodeID, maxTurnDeg float64) ([]Step, bool) {
	if from == to {
		return nil, false
	}
	dist := map[Step]float64{}
	parent := map[Step]Step{}
	hasParent := map[Step]bool{}
	pq := &stepQueue{}
	push := func(s Step, d float64, p *Step) {
		if old, ok := dist[s]; ok && old <= d {
			return
		}
		dist[s] = d
		if p != nil {
			parent[s], hasParent[s] = *p, true
		} else {
			delete(parent, s)
			hasParent[s] = false
		}
		heap.Push(pq, stepItem{s, d})
	}
	for _, s := range n.stepsFrom(from) {
		if prev == nil || n.TurnOK(*prev, s, maxTurnDeg) {
			push(s, n.StepLength(s), nil)
		}
	}
	for pq.Len() > 0 {
		it := heap.Pop(pq).(stepItem)
		if it.dist > dist[it.step] {
			continue
		}
		if n.StepTo(it.step) == to {
			var path []Step
			for s := it.step; ; s = parent[s] {
				path = append(path, s)
				if !hasParent[s] {
					break
				}
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path, true
		}
		cur := it.step
		for _, s := range n.stepsFrom(n.StepTo(cur)) {
			if n.TurnOK(cur, s, maxTurnDeg) {
				push(s, it.dist+n.StepLength(s), &cur)
			}
		}
	}
	return nil, false
}

// stepsFrom lists steps leaving node id in edge order (deterministic).
func (n *Network) stepsFrom(id NodeID) []Step {
	var out []Step
	for _, e := range n.Edges {
		if e.A == id {
			out = append(out, Step{Edge: e.ID, Forward: true})
		}
		if e.B == id {
			out = append(out, Step{Edge: e.ID, Forward: false})
		}
	}
	return out
}

// ReversePath returns the same track run in the opposite direction.
func ReversePath(p []Step) []Step {
	out := make([]Step, len(p))
	for i, s := range p {
		out[len(p)-1-i] = s.Reverse()
	}
	return out
}

type stepItem struct {
	step Step
	dist float64
}

type stepQueue []stepItem

func (q stepQueue) Len() int { return len(q) }
func (q stepQueue) Less(i, j int) bool {
	if q[i].dist != q[j].dist {
		return q[i].dist < q[j].dist
	}
	if q[i].step.Edge != q[j].step.Edge {
		return q[i].step.Edge < q[j].step.Edge
	}
	return q[i].step.Forward && !q[j].step.Forward
}
func (q stepQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *stepQueue) Push(x any)   { *q = append(*q, x.(stepItem)) }
func (q *stepQueue) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}
