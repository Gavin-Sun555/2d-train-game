package build

import (
	"fmt"

	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
)

// Compiled is a snapshot of the plan as a simulation network.
type Compiled struct {
	Plan   *Plan
	Net    *sim.Network
	NodeOf map[NodeID]sim.NodeID
	EdgeOf map[SegID]sim.EdgeID
}

func (p *Plan) Compile() *Compiled {
	c := &Compiled{Plan: p, Net: &sim.Network{}, NodeOf: map[NodeID]sim.NodeID{}, EdgeOf: map[SegID]sim.EdgeID{}}
	for _, n := range p.Nodes() {
		name := ""
		if st := p.stopAt(n.ID); st != nil {
			name = st.Name
		}
		c.NodeOf[n.ID] = c.Net.AddNodeAt(name, n.Pos, n.Level)
	}
	for _, s := range p.Segments() {
		c.EdgeOf[s.ID] = c.Net.AddEdge(c.NodeOf[s.A], c.NodeOf[s.B])
	}
	return c
}

// route turns a line into a shuttle route: out through every stop, then
// back the same way, reversing at both termini.
func (c *Compiled) route(l *Line) (sim.Route, error) {
	var out []sim.Leg
	var prev *sim.Step
	for i := 0; i+1 < len(l.Stops); i++ {
		a, b := c.Plan.stops[l.Stops[i]], c.Plan.stops[l.Stops[i+1]]
		if a == nil || b == nil {
			return sim.Route{}, fmt.Errorf("line uses a removed stop")
		}
		steps, ok := c.Net.Route(prev, c.NodeOf[a.Node], c.NodeOf[b.Node], sim.MaxTurnDeg)
		if !ok {
			return sim.Route{}, fmt.Errorf("no track from %s to %s (turns sharper than %d° are not allowed)", a.Name, b.Name, sim.MaxTurnDeg)
		}
		out = append(out, sim.Leg{Steps: steps, Station: b.Name, Dwell: StopDwell})
		prev = &steps[len(steps)-1]
	}
	legs := append([]sim.Leg(nil), out...)
	for i := len(out) - 1; i >= 0; i-- {
		station := c.Plan.stops[l.Stops[i]].Name
		legs = append(legs, sim.Leg{Steps: sim.ReversePath(out[i].Steps), Station: station, Dwell: StopDwell})
	}
	return sim.Route{Legs: legs}, nil
}

// LineStatus reports whether a line got a vehicle in the compiled world.
type LineStatus struct {
	Line  LineID
	Train int // index into World.Trains, -1 if none
	Err   error
}

// World builds a simulation with one vehicle per line, starting at the
// line's first stop (or its last stop if the first is occupied).
func (c *Compiled) World() (*sim.World, []LineStatus) {
	w := sim.NewWorld(c.Net)
	var status []LineStatus
	for _, l := range c.Plan.Lines() {
		st := LineStatus{Line: l.ID, Train: -1}
		r, err := c.route(l)
		if err != nil {
			st.Err = err
			status = append(status, st)
			continue
		}
		spec := l.Mode.Vehicle()
		spec.Name = fmt.Sprintf("L%d", len(status)+1)
		half := len(r.Legs) / 2
		// Last leg ends at the first stop; leg half-1 ends at the last stop.
		for _, leg := range []int{len(r.Legs) - 1, half - 1} {
			t, err := w.AddTrain(spec, r, leg, len(r.Legs[leg].Steps)-1)
			if err == nil {
				st.Train, st.Err = t.ID, nil
				break
			}
			st.Err = err
		}
		status = append(status, st)
	}
	return w, status
}
