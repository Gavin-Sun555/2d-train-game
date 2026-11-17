package game

import (
	"math"

	"github.com/Gavin-Sun555/2d-train-game/street"
)

// Street trees come from OpenStreetMap at their real positions, but the
// game's streets are drawn straightened and at standard lane widths, so
// many trees would stand in the carriageway. Each tree is kept where it is
// if it is clear of the roads; a tree on a street is moved out to the
// pavement on its side; a tree in a junction, on a motorway or with no room
// on the pavement is left out.

const (
	treeKerbGap  = 1.6 // m from the kerb to a street tree's trunk
	treeClear    = 1.0 // m a tree keeps from any carriageway
	treeGridSize = 60.0
)

// placeTrees works out where the map's trees stand on the network's
// streets (again after the player edits the roads).
func (s *scene) placeTrees(n *street.Network) {
	type cell struct{ x, y int }
	grid := map[cell][]street.RoadID{}
	for _, rd := range n.Roads {
		la, lb := n.RoadLevels(rd.ID)
		if la < 0 && lb < 0 {
			continue // underground tunnels do not disturb surface trees
		}
		a, b := n.Ends(rd.ID)
		pad := rd.HalfWidth() + sidewalkWidth
		for x := int(math.Floor((math.Min(a.X, b.X) - pad) / treeGridSize)); x <= int(math.Floor((math.Max(a.X, b.X)+pad)/treeGridSize)); x++ {
			for y := int(math.Floor((math.Min(a.Y, b.Y) - pad) / treeGridSize)); y <= int(math.Floor((math.Max(a.Y, b.Y)+pad)/treeGridSize)); y++ {
				grid[cell{x, y}] = append(grid[cell{x, y}], rd.ID)
			}
		}
	}
	jgrid := map[cell][]street.JunctionID{}
	for _, j := range n.Junctions {
		if j.Level < 0 {
			continue
		}
		c := cell{int(math.Floor(j.Pos.X / treeGridSize)), int(math.Floor(j.Pos.Y / treeGridSize))}
		jgrid[c] = append(jgrid[c], j.ID)
	}
	near := func(p street.Vec) cell {
		return cell{int(math.Floor(p.X / treeGridSize)), int(math.Floor(p.Y / treeGridSize))}
	}
	// onRoad reports the road whose carriageway (plus clearance) covers p.
	onRoad := func(p street.Vec, skip street.RoadID) (street.RoadID, bool) {
		for _, r := range grid[near(p)] {
			if r != skip && onCarriageway(n, r, p, treeClear) {
				return r, true
			}
		}
		return -1, false
	}
	inJunction := func(p street.Vec) bool {
		c := near(p)
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for _, j := range jgrid[cell{c.x + dx, c.y + dy}] {
					if dist(p, n.Junctions[j].Pos) < n.Radius(j)+sidewalkWidth {
						return true
					}
				}
			}
		}
		return false
	}
	onHighway := func(p street.Vec) bool {
		for i := range s.highways {
			h := &s.highways[i]
			if h.level < 0 || p.X < h.box[0]-h.width || p.X > h.box[2]+h.width || p.Y < h.box[1]-h.width || p.Y > h.box[3]+h.width {
				continue
			}
			for k := 0; k+1 < len(h.pts); k++ {
				if segDist(p, h.pts[k], h.pts[k+1]) < h.width/2+treeClear {
					return true
				}
			}
		}
		return false
	}
	s.trees = s.trees[:0]
	for _, p := range s.mapTrees {
		if r, ok := onRoad(p, -1); ok {
			if n.Roads[r].Tramway {
				continue // clear trees from off-road tramways, portals and viaducts
			}
			// Out to the pavement on the tree's side of the street.
			a, b := n.Ends(r)
			d := unit(sub(b, a))
			right := street.Vec{X: -d.Y, Y: d.X}
			t := math.Max(0, math.Min(1, ((p.X-a.X)*(b.X-a.X)+(p.Y-a.Y)*(b.Y-a.Y))/math.Max(dist(a, b)*dist(a, b), 1e-9)))
			foot := a.Lerp(b, t)
			l, rt := n.Roads[r].Sides()
			side := sub(p, foot)
			if side.X*right.X+side.Y*right.Y >= 0 {
				p = add(foot, scale(right, rt+treeKerbGap))
			} else {
				p = sub(foot, scale(right, l+treeKerbGap))
			}
			if _, still := onRoad(p, r); still {
				continue // no room between two streets
			}
		}
		if inJunction(p) || onHighway(p) {
			continue
		}
		s.trees = append(s.trees, p)
	}
}

// onCarriageway reports whether p lies on the road's carriageway, or within
// clear metres of it, each side measured against its own width. A planted
// median is not carriageway: trees there stay.
func onCarriageway(n *street.Network, r street.RoadID, p street.Vec, clear float64) bool {
	a, b := n.Ends(r)
	l2 := dist(a, b) * dist(a, b)
	if l2 == 0 {
		return false
	}
	t := ((p.X-a.X)*(b.X-a.X) + (p.Y-a.Y)*(b.Y-a.Y)) / l2
	if t < 0 || t > 1 {
		return false // beyond the road's ends: the junction's business
	}
	d := unit(sub(b, a))
	off := sub(p, a.Lerp(b, t))
	side := off.X*(-d.Y) + off.Y*d.X // + to the right
	rd := n.Roads[r]
	left, right := rd.Sides()
	if m := rd.Class.Median(); m > 0 && !rd.OneWay() && rd.Tram == street.NoTram && math.Abs(side) < m/2-0.5 {
		return false
	}
	if side >= 0 {
		return side < right+clear
	}
	return -side < left+clear
}
