package traffic

import (
	"fmt"
	"sort"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// A Snapshot is the running service, so a save can carry on where it left
// off: every tram and car where it is, the passengers waiting and on
// board, the trams still to enter service, the clock and the statistics.
// Everything else (who occupies which track, junction admissions, wrong-way
// runs) is worked out again from the vehicles every tick, and the signals
// follow the clock.
type Snapshot struct {
	Links      int   `json:"links"` // the network it belongs to has this many links
	Tick       int64 `json:"tick"`
	NextID     int   `json:"nextID"`
	Stats      Stats `json:"stats"`
	Passengers PassengerStats
	Rng        []byte `json:"rng,omitempty"`
	PRng       []byte `json:"prng,omitempty"`
	Vehicles   []VehicleState
	Depots     []DepotState
	Queues     []QueueState
	Lines      []LineState
}

// VehicleState is one tram or car.
type VehicleState struct {
	ID       int
	Kind     Kind
	Length   float64
	Width    float64
	V0, A, B float64
	S0, T    float64
	Path     []street.LinkID
	Idx      int
	S, V     float64
	Admitted int
	DestPos  float64 `json:",omitempty"`
	Variant  uint8   `json:",omitempty"`
	// Trams
	Line      street.LineID         `json:",omitempty"`
	Cars      int                   `json:",omitempty"`
	Call      int                   `json:",omitempty"`
	Platform  street.Platform       `json:",omitempty"`
	PlatIdx   int                   `json:",omitempty"`
	Dwelling  bool                  `json:",omitempty"`
	DwellLeft float64               `json:",omitempty"`
	Reversing bool                  `json:",omitempty"`
	RunFrom   int                   `json:",omitempty"`
	RunTo     int                   `json:",omitempty"`
	Riders    map[street.StopID]int `json:",omitempty"`
	Load      int                   `json:",omitempty"`
	Served    bool                  `json:",omitempty"`
	// Passengers still to get off, and the doors' progress.
	AlightLeft int `json:",omitempty"`
	// The doors (DoorsKnown: saved since doors were modelled).
	Doors       DoorState `json:",omitempty"`
	DoorT       float64   `json:",omitempty"`
	DoorsKnown  bool      `json:",omitempty"`
	AlightTimer float64   `json:",omitempty"`
	BoardTimer  float64   `json:",omitempty"`
	TramStats   TramStats `json:",omitempty"`
	TripFrom    float64   `json:",omitempty"`
	Stuck       float64   `json:",omitempty"`
}

// DepotState is a line's trams not yet in service.
type DepotState struct {
	Line street.LineID
	Left int
	Next float64
}

// QueueState is a stop's waiting passengers, by destination.
type QueueState struct {
	Stop     street.StopID
	Waiting  []street.StopID
	Timer    float64
	Interval float64
	Arrivals int
}

// LineState is a line's statistics.
type LineState struct {
	Line    street.LineID
	Stats   LineStats
	Departs []float64
}

// Snapshot captures the running service.
func (s *Sim) Snapshot() *Snapshot {
	snap := &Snapshot{Links: len(s.Net.Links), Tick: s.Tick, NextID: s.nextID, Stats: s.Stats, Passengers: s.Passengers}
	snap.Rng, _ = s.src.MarshalBinary()
	snap.PRng, _ = s.psrc.MarshalBinary()
	for _, v := range s.Vehicles {
		riders := map[street.StopID]int{}
		for k, n := range v.riders {
			riders[k] = n
		}
		snap.Vehicles = append(snap.Vehicles, VehicleState{
			ID: v.ID, Kind: v.Kind, Length: v.Length, Width: v.Width, V0: v.V0, A: v.A, B: v.B, S0: v.S0, T: v.T,
			Path: append([]street.LinkID(nil), v.Path...), Idx: v.idx, S: v.S, V: v.V, Admitted: v.admitted,
			DestPos: v.DestPos, Variant: v.Variant,
			Line: v.Line, Cars: v.Cars, Call: v.call, Platform: v.platform, PlatIdx: v.platIdx,
			Dwelling: v.Dwelling, DwellLeft: v.DwellLeft, Reversing: v.reversing, RunFrom: v.runFrom, RunTo: v.runTo,
			Riders: riders, Load: v.load, Served: v.served, AlightLeft: v.alightLeft, AlightTimer: v.alightTimer, BoardTimer: v.boardTimer,
			Doors: v.doors, DoorT: v.doorT, DoorsKnown: true, TramStats: v.stats, TripFrom: v.tripFrom, Stuck: v.stuck,
		})
	}
	for _, d := range s.depots {
		snap.Depots = append(snap.Depots, DepotState{d.line, d.left, d.next})
	}
	for _, id := range s.Net.StopIDs() {
		if q := s.queues[id]; q != nil {
			snap.Queues = append(snap.Queues, QueueState{id, append([]street.StopID(nil), q.waiting...), q.timer, q.interval, q.arrivals})
		}
	}
	ids := make([]street.LineID, 0, len(s.lineStats))
	for id := range s.lineStats {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		st := s.lineStats[id]
		snap.Lines = append(snap.Lines, LineState{id, *st, st.departs})
	}
	return snap
}

// Restore sets the service running as it was when the snapshot was taken.
// The network must be the one the snapshot was taken on (same track,
// stops and lines); otherwise it reports an error and changes nothing.
func (s *Sim) Restore(snap *Snapshot) error {
	n := s.Net
	if snap == nil {
		return fmt.Errorf("no snapshot")
	}
	if snap.Links != len(n.Links) {
		return fmt.Errorf("the network changed (%d links, snapshot has %d)", len(n.Links), snap.Links)
	}
	var vs []*Vehicle
	for _, st := range snap.Vehicles {
		if len(st.Path) == 0 || st.Idx < 0 || st.Idx >= len(st.Path) {
			return fmt.Errorf("vehicle %d has no position", st.ID)
		}
		for _, id := range st.Path {
			if id < 0 || int(id) >= len(n.Links) {
				return fmt.Errorf("vehicle %d is on a missing link", st.ID)
			}
		}
		if st.Kind == TramKind && n.Lines[st.Line] == nil {
			return fmt.Errorf("tram %d belongs to a missing line", st.ID)
		}
		v := &Vehicle{ID: st.ID, Kind: st.Kind, Length: st.Length, Width: st.Width, V0: st.V0, A: st.A, B: st.B, S0: st.S0, T: st.T,
			Path: st.Path, idx: st.Idx, S: st.S, V: st.V, admitted: st.Admitted, DestPos: st.DestPos, Variant: st.Variant,
			Line: st.Line, Cars: st.Cars, call: st.Call, platform: st.Platform, platIdx: st.PlatIdx,
			Dwelling: st.Dwelling, DwellLeft: st.DwellLeft, reversing: st.Reversing, runFrom: st.RunFrom, runTo: st.RunTo,
			load: st.Load, served: st.Served, alightLeft: st.AlightLeft, alightTimer: st.AlightTimer, boardTimer: st.BoardTimer, stats: st.TramStats, tripFrom: st.TripFrom, stuck: st.Stuck}
		if len(st.Riders) > 0 {
			v.riders = st.Riders
		}
		v.doors, v.doorT = st.Doors, st.DoorT
		if !st.DoorsKnown && st.Served && st.Dwelling {
			v.doors = DoorsOpen // saved before doors: they were open
		}
		vs = append(vs, v)
	}
	if len(snap.Rng) > 0 {
		if err := s.src.UnmarshalBinary(snap.Rng); err != nil {
			return err
		}
	}
	if len(snap.PRng) > 0 {
		if err := s.psrc.UnmarshalBinary(snap.PRng); err != nil {
			return err
		}
	}
	s.Vehicles, s.Tick, s.nextID, s.Stats, s.Passengers = vs, snap.Tick, snap.NextID, snap.Stats, snap.Passengers
	for _, d := range s.depots {
		for _, ds := range snap.Depots {
			if ds.Line == d.line {
				d.left, d.next = ds.Left, ds.Next
			}
		}
	}
	for _, qs := range snap.Queues {
		if q := s.queues[qs.Stop]; q != nil {
			q.waiting, q.timer, q.interval, q.arrivals = qs.Waiting, qs.Timer, qs.Interval, qs.Arrivals
		}
	}
	s.lineStats = map[street.LineID]*LineStats{}
	for _, ls := range snap.Lines {
		st := ls.Stats
		st.departs = ls.Departs
		if st.On == nil {
			st.On = map[street.StopID]int{}
		}
		if st.Off == nil {
			st.Off = map[street.StopID]int{}
		}
		s.lineStats[ls.Line] = &st
	}
	s.rebuildOccupancy()
	return nil
}
