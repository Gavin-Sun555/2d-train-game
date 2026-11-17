package traffic

import "github.com/Gavin-Sun555/2d-train-game/street"

// Operating statistics, as in the bus game's line management: passengers
// on and off at each stop, trips run, distance, speed, headways and how
// full the trams are. They count from the moment the service (re)starts.

// LineStats is what a line's trams have done since the service started.
type LineStats struct {
	Boarded, Delivered int
	Trips              int     // one-way runs completed, terminus to terminus
	TripTime           float64 // s, summed over those runs
	Distance           float64 // m run by the line's trams
	TramTime           float64 // tram-seconds in service
	PassDist, SeatDist float64 // passenger-metres carried, and places-metres offered
	On, Off            map[street.StopID]int
	departs            []float64 // recent departures from the first stop
}

const headwaySamples = 12

// AvgTrip is the mean time of a completed one-way run (0 before the first).
func (st *LineStats) AvgTrip() float64 {
	if st.Trips == 0 {
		return 0
	}
	return st.TripTime / float64(st.Trips)
}

// Speed is the average speed in service, stops included, in m/s.
func (st *LineStats) Speed() float64 {
	if st.TramTime < 1 {
		return 0
	}
	return st.Distance / st.TramTime
}

// Headway is the mean interval between recent departures from the first
// stop (0 until two have left).
func (st *LineStats) Headway() float64 {
	n := len(st.departs)
	if n < 2 {
		return 0
	}
	return (st.departs[n-1] - st.departs[0]) / float64(n-1)
}

// LoadFactor is the share of the places offered that carried a passenger,
// weighted by distance.
func (st *LineStats) LoadFactor() float64 {
	if st.SeatDist <= 0 {
		return 0
	}
	return st.PassDist / st.SeatDist
}

// TramStats is what one tram has done since it entered service.
type TramStats struct {
	Trips    int
	Distance float64 // m
	Boarded  int
	Since    float64 // s, when it entered service
}

// Speed is the tram's average speed in service, in m/s.
func (t TramStats) Speed(now float64) float64 {
	if now-t.Since < 1 {
		return 0
	}
	return t.Distance / (now - t.Since)
}

// LineStats returns the line's statistics (empty if it has none yet).
func (s *Sim) LineStats(id street.LineID) *LineStats {
	if st := s.lineStats[id]; st != nil {
		return st
	}
	return &LineStats{}
}

func (s *Sim) statsFor(id street.LineID) *LineStats {
	if s.lineStats == nil {
		s.lineStats = map[street.LineID]*LineStats{}
	}
	st := s.lineStats[id]
	if st == nil {
		st = &LineStats{On: map[street.StopID]int{}, Off: map[street.StopID]int{}}
		s.lineStats[id] = st
	}
	return st
}

// Stats returns the tram's statistics.
func (v *Vehicle) Stats() TramStats { return v.stats }

// countRun adds a moving tram's distance this tick.
func (s *Sim) countRun(v *Vehicle, step float64) {
	st := s.statsFor(v.Line)
	st.Distance += step
	st.PassDist += float64(v.load) * step
	st.SeatDist += float64(s.Capacity(v)) * step
	v.stats.Distance += step
}

// countTime adds a tick of service for every tram.
func (s *Sim) countTime() {
	for _, v := range s.Vehicles {
		if v.Kind == TramKind {
			s.statsFor(v.Line).TramTime += DT
		}
	}
}

// countStop records passengers at a stop and, at a terminus, the end of a
// one-way run.
func (s *Sim) countStop(v *Vehicle, stop street.StopID, on, off int, terminus bool) {
	st := s.statsFor(v.Line)
	st.Boarded += on
	st.Delivered += off
	if on > 0 {
		st.On[stop] += on
	}
	if off > 0 {
		st.Off[stop] += off
	}
	v.stats.Boarded += on
	if terminus && v.tripFrom >= 0 {
		st.Trips++
		st.TripTime += s.Seconds() - v.tripFrom
		v.stats.Trips++
		v.tripFrom = -1
	}
}

// countDepart notes a departure from call i: where the tram turns back a
// one-way run starts; from the first call it also times the headway.
func (s *Sim) countDepart(v *Vehicle, l *street.Line, i int) {
	now := s.Seconds()
	if l.TurnsAt(i) {
		v.tripFrom = now
	}
	if i == 0 {
		st := s.statsFor(v.Line)
		st.departs = append(st.departs, now)
		if len(st.departs) > headwaySamples {
			st.departs = st.departs[len(st.departs)-headwaySamples:]
		}
	}
}

// LineLength is the length of the line's route one way, in metres: half
// the distance a tram runs over a whole round trip.
func LineLength(n *street.Network, l *street.Line) float64 { return n.CycleLength(l) / 2 }
