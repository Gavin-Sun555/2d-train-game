// Package build is the player's construction model: track segments drawn on
// the city, stops, and lines. It validates construction against the city
// map and compiles the result into a rail/sim network. Engine-free.
package build

import (
	"github.com/Gavin-Sun555/2d-train-game/city"
	"github.com/Gavin-Sun555/2d-train-game/rail/sim"
)

// LevelRules describes what can be built on one vertical level.
type LevelRules struct {
	Level     sim.Level
	Buildable bool // false = reserved for a later version
	allowed   map[city.Kind]bool
}

func (r LevelRules) Allows(k city.Kind) bool { return r.allowed[k] }

func kinds(ks ...city.Kind) map[city.Kind]bool {
	m := map[city.Kind]bool{}
	for _, k := range ks {
		m[k] = true
	}
	return m
}

// Rules per level. Ground track (trams) runs on streets, bridges and open
// ground. Elevated viaducts will be allowed over streets and water but not
// through buildings; tunnels go anywhere. Only Ground is enabled for now.
var Rules = map[sim.Level]LevelRules{
	sim.Ground: {
		Level: sim.Ground, Buildable: true,
		allowed: kinds(city.Road, city.Bridge, city.Grass, city.Park),
	},
	sim.Elevated: {
		Level: sim.Elevated, Buildable: false,
		allowed: kinds(city.Road, city.Bridge, city.Grass, city.Park, city.Water),
	},
	sim.Underground: {
		Level: sim.Underground, Buildable: false,
		allowed: kinds(city.Road, city.Bridge, city.Grass, city.Park, city.Water, city.Building),
	},
	// The rare very high viaduct (high-speed rail) and the deep bore
	// (express metro) pass over or under everything.
	sim.HighElevated: {
		Level: sim.HighElevated, Buildable: false,
		allowed: kinds(city.Road, city.Bridge, city.Grass, city.Park, city.Water),
	},
	sim.DeepUnderground: {
		Level: sim.DeepUnderground, Buildable: false,
		allowed: kinds(city.Road, city.Bridge, city.Grass, city.Park, city.Water, city.Building),
	},
}

// Mode is the kind of service a line runs. Each mode has its home level and
// rolling stock; Tram is the only one wired up today.
type Mode int

const (
	Tram Mode = iota
	LightRail
	Metro
)

func (m Mode) String() string { return [...]string{"tram", "light rail", "metro"}[m] }

func (m Mode) Level() sim.Level {
	return [...]sim.Level{sim.Ground, sim.Elevated, sim.Underground}[m]
}

func (m Mode) Vehicle() sim.TrainSpec {
	switch m {
	case LightRail:
		return sim.TrainSpec{Length: 60, MaxSpeed: 80 / 3.6, Accel: 1.0, Decel: 1.1}
	case Metro:
		return sim.TrainSpec{Length: 120, MaxSpeed: 90 / 3.6, Accel: 1.0, Decel: 1.0}
	}
	return sim.TrainSpec{Length: 30, MaxSpeed: 50 / 3.6, Accel: 1.2, Decel: 1.3}
}

const (
	MinSegment = 10.0 // m
	snapEps    = 0.5  // m: points closer than this are the same point
	sampleStep = 1.0  // m between terrain checks along a segment
	StopDwell  = 20.0 // s
)
