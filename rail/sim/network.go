// Package sim is the engine-independent rail simulation core.
//
// It must never import ebiten or any rendering package: everything here runs
// headless at a fixed tick so it can be replayed and asserted in tests.
package sim

import (
	"fmt"
	"math"
)

type NodeID int
type EdgeID int

type Vec2 struct{ X, Y float64 }

// Level is the vertical layer a piece of track is built on. Only Ground is
// buildable today; the others are reserved so metro and elevated light rail
// can share the same network, pathfinding and signalling later. Track on
// different levels never connects except through an explicit ramp/portal
// edge (whose end nodes have different levels).
type Level int

const (
	Ground Level = iota
	Elevated
	Underground
)

func (l Level) String() string {
	switch l {
	case Elevated:
		return "elevated"
	case Underground:
		return "underground"
	}
	return "ground"
}

func (a Vec2) Lerp(b Vec2, t float64) Vec2 {
	return Vec2{a.X + (b.X-a.X)*t, a.Y + (b.Y-a.Y)*t}
}

// Node is a point where track pieces meet (platform end, junction, bend).
type Node struct {
	ID    NodeID
	Name  string
	Pos   Vec2 // metres
	Level Level
}

// Edge is a straight piece of track between two nodes. In this first version
// every edge is also exactly one signalling block.
type Edge struct {
	ID     EdgeID
	A, B   NodeID
	Length float64 // metres
	Level  Level   // level of the A end; a ramp has a B node on another level
}

type Network struct {
	Nodes []Node
	Edges []Edge

	block []int // per edge: signalling block ID (nil = one block per edge)
}

// Block returns the signalling block an edge belongs to. Block IDs are edge
// IDs (the lowest edge in the block), so they index per-edge arrays.
func (n *Network) Block(e EdgeID) int {
	if int(e) < len(n.block) {
		return n.block[e]
	}
	return int(e)
}

// GroupJunctions puts every edge touching a junction (a node with three or
// more edges, including flat crossings) into one block with the junction's
// other edges, so only one train can be inside a junction area at a time.
// Call it after the network is complete.
func (n *Network) GroupJunctions() {
	parent := make([]int, len(n.Edges))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	incident := make([][]int, len(n.Nodes))
	for _, e := range n.Edges {
		incident[e.A] = append(incident[e.A], int(e.ID))
		incident[e.B] = append(incident[e.B], int(e.ID))
	}
	for _, es := range incident {
		if len(es) < 3 {
			continue
		}
		for _, e := range es[1:] {
			a, b := find(es[0]), find(e)
			if a > b {
				a, b = b, a
			}
			parent[b] = a
		}
	}
	n.block = make([]int, len(n.Edges))
	for i := range n.block {
		n.block[i] = find(i)
	}
}

func (n *Network) AddNode(name string, x, y float64) NodeID {
	return n.AddNodeAt(name, Vec2{x, y}, Ground)
}

func (n *Network) AddNodeAt(name string, p Vec2, level Level) NodeID {
	id := NodeID(len(n.Nodes))
	n.Nodes = append(n.Nodes, Node{ID: id, Name: name, Pos: p, Level: level})
	return id
}

// Dir is the unit direction of travel along a step.
func (n *Network) Dir(s Step) Vec2 {
	a, b := n.Nodes[n.StepFrom(s)].Pos, n.Nodes[n.StepTo(s)].Pos
	l := math.Hypot(b.X-a.X, b.Y-a.Y)
	if l == 0 {
		return Vec2{}
	}
	return Vec2{(b.X - a.X) / l, (b.Y - a.Y) / l}
}

// AddEdge joins two nodes; its length is the straight-line distance.
func (n *Network) AddEdge(a, b NodeID) EdgeID {
	pa, pb := n.Nodes[a].Pos, n.Nodes[b].Pos
	id := EdgeID(len(n.Edges))
	n.Edges = append(n.Edges, Edge{ID: id, A: a, B: b, Length: math.Hypot(pb.X-pa.X, pb.Y-pa.Y), Level: n.Nodes[a].Level})
	return id
}

// Step is an edge traversed in one direction (Forward = from A to B).
type Step struct {
	Edge    EdgeID
	Forward bool
}

func (s Step) Reverse() Step { return Step{Edge: s.Edge, Forward: !s.Forward} }

func (n *Network) StepLength(s Step) float64 { return n.Edges[s.Edge].Length }

func (n *Network) StepFrom(s Step) NodeID {
	if s.Forward {
		return n.Edges[s.Edge].A
	}
	return n.Edges[s.Edge].B
}

func (n *Network) StepTo(s Step) NodeID {
	if s.Forward {
		return n.Edges[s.Edge].B
	}
	return n.Edges[s.Edge].A
}

// PointOnStep returns the position d metres from the step's entry node.
func (n *Network) PointOnStep(s Step, d float64) Vec2 {
	from, to := n.Nodes[n.StepFrom(s)].Pos, n.Nodes[n.StepTo(s)].Pos
	l := n.StepLength(s)
	if l <= 0 {
		return from
	}
	return from.Lerp(to, d/l)
}

// Path converts a node sequence into steps, failing if two consecutive nodes
// are not directly connected.
func (n *Network) Path(nodes ...NodeID) ([]Step, error) {
	steps := make([]Step, 0, len(nodes))
	for i := 0; i+1 < len(nodes); i++ {
		a, b := nodes[i], nodes[i+1]
		found := false
		for _, e := range n.Edges {
			if e.A == a && e.B == b {
				steps = append(steps, Step{Edge: e.ID, Forward: true})
				found = true
				break
			}
			if e.A == b && e.B == a {
				steps = append(steps, Step{Edge: e.ID, Forward: false})
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no edge between %s and %s", n.Nodes[a].Name, n.Nodes[b].Name)
		}
	}
	return steps, nil
}

func (n *Network) MustPath(nodes ...NodeID) []Step {
	steps, err := n.Path(nodes...)
	if err != nil {
		panic(err)
	}
	return steps
}
