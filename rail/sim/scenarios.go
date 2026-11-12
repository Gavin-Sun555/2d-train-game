package sim

// Default rolling stock: a 2-car light-rail unit.
var LightRail = TrainSpec{
	Length:   60,
	MaxSpeed: 80 / 3.6,
	Accel:    1.0,
	Decel:    1.1,
}

func named(spec TrainSpec, name string) TrainSpec {
	spec.Name = name
	return spec
}

// Shuttle: two terminals on a single track split into three blocks, one
// train running back and forth.
func Shuttle() *World {
	net := &Network{}
	a := net.AddNode("West", 0, 0)
	m1 := net.AddNode("", 500, 0)
	m2 := net.AddNode("", 1000, 150)
	b := net.AddNode("East", 1500, 150)
	net.AddEdge(a, m1)
	net.AddEdge(m1, m2)
	net.AddEdge(m2, b)

	route := Route{Legs: []Leg{
		{Steps: net.MustPath(a, m1, m2, b), Station: "East", Dwell: 30},
		{Steps: net.MustPath(b, m2, m1, a), Station: "West", Dwell: 30},
	}}
	w := NewWorld(net)
	if _, err := w.AddTrain(named(LightRail, "T1"), route, 1, 2); err != nil {
		panic(err)
	}
	return w
}

// Loop: a one-way ring of six blocks with two stations and three trains.
// T2 starts directly behind T3, so it must wait at a red signal.
func Loop() *World {
	net := &Network{}
	n := []NodeID{
		net.AddNode("North", 0, 0),
		net.AddNode("", 600, 0),
		net.AddNode("", 1200, 0),
		net.AddNode("South", 1200, 600),
		net.AddNode("", 600, 600),
		net.AddNode("", 0, 600),
	}
	for i := range n {
		net.AddEdge(n[i], n[(i+1)%len(n)])
	}
	route := Route{Legs: []Leg{
		{Steps: net.MustPath(n[0], n[1], n[2], n[3]), Station: "South", Dwell: 25},
		{Steps: net.MustPath(n[3], n[4], n[5], n[0]), Station: "North", Dwell: 25},
	}}
	w := NewWorld(net)
	for _, p := range []struct {
		name      string
		leg, step int
	}{{"T1", 0, 0}, {"T2", 1, 0}, {"T3", 1, 1}} {
		if _, err := w.AddTrain(named(LightRail, p.name), route, p.leg, p.step); err != nil {
			panic(err)
		}
	}
	return w
}

// Scenarios lists the built-in worlds in menu order.
var Scenarios = []struct {
	Name string
	New  func() *World
}{
	{"Shuttle (1 train, single track)", Shuttle},
	{"Loop (3 trains, block signals)", Loop},
}
