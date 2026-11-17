package street

import (
	"testing"

	"github.com/Gavin-Sun555/2d-train-game/mapdata"
)

func TestSeattleNetwork(t *testing.T) {
	n := FromMap(mapdata.Seattle())
	if len(n.Roads) < 500 || len(n.signals) < 100 {
		t.Fatalf("network too small: %d roads, %d signals", len(n.Roads), len(n.signals))
	}
	oneway := 0
	for _, r := range n.Roads {
		if r.OneWay() {
			oneway++
		}
	}
	if oneway == 0 {
		t.Error("downtown Seattle should have one-way streets")
	}
	for _, l := range n.Links {
		// One-way streets onto a motorway ramp carry on along the ramp.
		if l.Kind == LaneLink && l.Modes&Car != 0 && len(l.Next) == 0 && n.Roads[l.Road].OneWay() &&
			!n.anchors[n.Junctions[n.Roads[l.Road].B].Pos] {
			t.Errorf("one-way lane %d on %q leads nowhere", l.ID, n.Roads[l.Road].Name)
		}
		if l.Kind == LaneLink && l.Length < 5 {
			t.Errorf("lane %d on %q only %.1f m long", l.ID, n.Roads[l.Road].Name, l.Length)
		}
	}
}
