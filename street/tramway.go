package street

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/Gavin-Sun555/2d-train-game/internal/locale"
)

const (
	minTramwayLen = 20.0 // m: shortest flat off-road / elevated / underground piece
	minRampLen    = 36.0 // m: shortest transition ramp / portal between levels
)

// RoadLevels returns the vertical levels at ends A and B of road r.
func (n *Network) RoadLevels(r RoadID) (int, int) {
	rd := n.Roads[r]
	la, lb := n.Junctions[rd.A].Level, n.Junctions[rd.B].Level
	if !rd.Tramway && la == 0 && lb == 0 && rd.Level != 0 {
		return rd.Level, rd.Level
	}
	return la, lb
}

// IsRamp reports whether road r is a portal or incline transitioning between
// two different vertical levels.
func (n *Network) IsRamp(r RoadID) bool {
	rd := n.Roads[r]
	return n.Junctions[rd.A].Level != n.Junctions[rd.B].Level
}

// StopLevel returns the vertical level of stop s.
func (n *Network) StopLevel(s *Stop) int {
	la, _ := n.RoadLevels(s.Road)
	return la
}

// LinkElevation returns the continuous vertical level (-1..+1) at distance s
// along link id.
func (n *Network) LinkElevation(id LinkID, s float64) float64 {
	l := &n.Links[id]
	if l.Kind == ConnectorLink {
		return float64(n.Junctions[l.Junction].Level)
	}
	la, lb := n.RoadLevels(l.Road)
	if la == lb {
		return float64(la)
	}
	p, _ := l.PointAt(s)
	a, b := n.Ends(l.Road)
	t, _ := project(p, a, b)
	return float64(la) + (float64(lb)-float64(la))*t
}

// splitPreservingTrack splits flat road/tramway r at q, keeping any existing
// track, stops and crossovers on the two resulting pieces.
func (n *Network) splitPreservingTrack(r RoadID, q Vec) (JunctionID, error) {
	if n.IsRamp(r) {
		return -1, errors.New(locale.T("不能在出入坡道中间接入其他轨道或道路", "Can't join midway along a portal ramp"))
	}
	rd := n.Roads[r]
	a, b := n.Ends(r)
	total := dist(a, b)
	u := dist(a, q) / math.Max(total, 1e-9)
	if u <= 0.02 || u >= 0.98 {
		if u < 0.5 {
			return rd.A, nil
		}
		return rd.B, nil
	}
	posM := u * total
	margin := rd.HalfWidth() + JunctionApproach + 2
	for _, st := range n.Stops {
		if st.Road == r && math.Abs(st.T*total-posM) < PlatformLength(st.Capacity())/2+margin {
			return -1, fmt.Errorf(locale.T("离 %s 站台太近，无法在此交叉或分支", "Too close to the %s platform to cross or branch here"), st.Name)
		}
	}
	for _, c := range rd.Crossovers {
		if math.Abs(c*total-posM) < CrossoverLength/2+margin {
			return -1, errors.New(locale.T("离交叉渡线太近，无法在此交叉或分支", "Too close to a crossover to cross or branch here"))
		}
	}

	lv := n.Junctions[rd.A].Level
	j := JunctionID(len(n.Junctions))
	n.Junctions = append(n.Junctions, Junction{ID: j, Pos: q, Level: lv})

	tailID := RoadID(len(n.Roads))
	tail := rd
	tail.ID, tail.A = tailID, j
	var cHead, cTail []float64
	for _, c := range rd.Crossovers {
		if c < u {
			cHead = append(cHead, c/u)
		} else if c > u {
			cTail = append(cTail, (c-u)/(1-u))
		}
	}
	n.Roads[r].B = j
	n.Roads[r].Crossovers = cHead
	tail.Crossovers = cTail
	n.Roads = append(n.Roads, tail)

	for _, st := range n.Stops {
		if st.Road != r {
			continue
		}
		if st.T < u {
			st.T = st.T / u
		} else {
			st.Road = tailID
			st.T = (st.T - u) / (1 - u)
		}
	}
	return j, nil
}

// tramEndpoint resolves (or creates) a junction at level lv near p.
func (n *Network) tramEndpoint(p Vec, lv int, snap float64) (JunctionID, error) {
	if j, ok := n.NearestJunctionAt(p, lv, snap); ok {
		return j, nil
	}
	// Snap onto a flat road or tramway at the same level if close enough.
	best, bestT, bestD := RoadID(-1), 0.0, snap
	for _, r := range n.Roads {
		la, lb := n.RoadLevels(r.ID)
		if la != lv || lb != lv {
			continue
		}
		a, b := n.Ends(r.ID)
		if t, d := project(p, a, b); d <= bestD {
			best, bestT, bestD = r.ID, t, d
		}
	}
	if best >= 0 {
		a, b := n.Ends(best)
		q := a.Lerp(b, bestT)
		if dist(q, a) < junctionSpace {
			return n.Roads[best].A, nil
		}
		if dist(q, b) < junctionSpace {
			return n.Roads[best].B, nil
		}
		return n.splitPreservingTrack(best, q)
	}
	// Open ground / elevated air / underground rock: create a free junction.
	j := JunctionID(len(n.Junctions))
	n.Junctions = append(n.Junctions, Junction{ID: j, Pos: p, Level: lv})
	return j, nil
}

// AddTramway builds an off-road ground tramway (lvA=0, lvB=0), an elevated
// viaduct (1, 1), an underground tunnel (-1, -1), or a transition ramp/portal
// between ground (0) and elevated (1) or underground (-1).
func (n *Network) AddTramway(a, b Vec, lvA, lvB int, single bool, snap float64) error {
	return n.addTramway(a, b, lvA, lvB, single, snap, false)
}

func (n *Network) addTramway(a, b Vec, lvA, lvB int, single bool, snap float64, replay bool) error {
	if lvA < -1 || lvA > 1 || lvB < -1 || lvB > 1 {
		return errors.New(locale.T("当前仅支持地面层、高架层与地下层建设", "Only ground, elevated and underground levels can be built on"))
	}
	diff := lvA - lvB
	if diff < -1 || diff > 1 {
		return errors.New(locale.T("高架与地下之间请先回到地面层过渡", "Transition through the ground level between elevated and underground"))
	}
	e := &RoadEdit{Op: "tramway", LevelA: lvA, LevelB: lvB, Single: single}
	return n.edit(e, func() error {
		ja, err := n.tramEndpoint(a, lvA, snap)
		if err != nil {
			return err
		}
		jb, err := n.tramEndpoint(b, lvB, snap)
		if err != nil {
			return err
		}
		if ja == jb {
			return errors.New(locale.T("起点和终点相同", "Start and end are the same point"))
		}
		pa, pb := n.Junctions[ja].Pos, n.Junctions[jb].Pos
		e.A, e.B = pa, pb
		total := dist(pa, pb)
		minLen := minTramwayLen
		if lvA != lvB {
			minLen = minRampLen
		}
		if total < minLen {
			return fmt.Errorf(locale.T("轨道段太短（%.0f m），至少需要 %.0f m", "Segment is too short (%.0f m), need at least %.0f m"), total, minLen)
		}
		if !replay && n.CheckCorridor != nil {
			if err := n.CheckCorridor(pa, pb, lvA, lvB); err != nil {
				return err
			}
		}

		// Transition ramp / portal (0 <-> +-1): cannot cross or run inside streets.
		if lvA != lvB {
			for _, rd := range n.Roads {
				if rd.A == ja || rd.A == jb || rd.B == ja || rd.B == jb {
					// Even if sharing the ground junction, a ramp cannot run right
					// along the middle of a street carriageway.
					if !rd.Tramway {
						ra, rb := n.Ends(rd.ID)
						mid := pa.Lerp(pb, 0.5)
						if _, d := project(mid, ra, rb); d < rd.HalfWidth()+1.5 {
							return errors.New(locale.T("出入坡道不能建在现有道路车行道内，请在道路外侧空地起坡或入地", "A portal ramp can't sit inside a street carriageway: place the ramp on open ground beside or beyond the street"))
						}
					}
					continue
				}
				la, lb := n.RoadLevels(rd.ID)
				samePlane := la == 0 || lb == 0 || la == lvA || lb == lvA || la == lvB || lb == lvB
				if !samePlane {
					continue
				}
				ra, rb := n.Ends(rd.ID)
				if _, _, ok := segHit(pa, pb, ra, rb); ok {
					if !rd.Tramway {
						return errors.New(locale.T("高架/地下出入坡道不能直接横穿地面道路，请在道路前完成起坡或入地", "A portal ramp can't cut across a street: finish rising or diving clear of the road"))
					}
					return errors.New(locale.T("出入坡道不能在途中与其他轨道交叉", "A portal ramp can't cross another track midway"))
				}
				// Also prevent placing a ramp right inside a street carriageway.
				if !rd.Tramway {
					for _, f := range []float64{0.25, 0.5, 0.75} {
						if _, d := project(pa.Lerp(pb, f), ra, rb); d < rd.HalfWidth()+2.0 {
							return errors.New(locale.T("出入坡道与地面道路冲突，请保持安全间距", "The portal ramp conflicts with a ground street"))
						}
					}
				}
			}
			f, bk := 1, 1
			if single {
				f, bk = 0, 0
			}
			lvl := lvA
			if lvl == 0 {
				lvl = lvB
			}
			n.Roads = append(n.Roads, Road{
				ID: RoadID(len(n.Roads)), A: ja, B: jb,
				Class: Street, Fwd: f, Back: bk,
				Tram: Dedicated, Single: single, Tramway: true, Level: lvl,
			})
			return nil
		}

		// Flat tramway at level lv (lvA == lvB): crosses same-level streets/tracks
		// at shared junctions, and passes cleanly over/under other levels.
		lv := lvA
		type hit struct {
			s float64
			r RoadID
			q Vec
		}
		var hits []hit
		for _, rd := range n.Roads {
			if rd.A == ja || rd.A == jb || rd.B == ja || rd.B == jb {
				continue
			}
			la, lb := n.RoadLevels(rd.ID)
			ra, rb := n.Ends(rd.ID)
			if la != lb && (la == lv || lb == lv) {
				if _, _, ok := segHit(pa, pb, ra, rb); ok {
					return errors.New(locale.T("不能横穿高架/地下出入坡道", "Can't cross a portal ramp midway"))
				}
				continue
			}
			if la != lv || lb != lv {
				continue // grade-separated: passes cleanly above or below!
			}
			if s, u, ok := segHit(pa, pb, ra, rb); ok {
				hits = append(hits, hit{s, rd.ID, ra.Lerp(rb, u)})
			}
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].s < hits[j].s })
		chain := []JunctionID{ja}
		for _, h := range hits {
			ra, rb := n.Ends(h.r)
			if dist(h.q, ra) < junctionSpace || dist(h.q, rb) < junctionSpace {
				return fmt.Errorf(locale.T("离 %s 的路口太近，请直接连到该路口", "Passes too close to a junction of %s: connect to that junction instead"), n.roadName(h.r))
			}
			j, err := n.splitPreservingTrack(h.r, h.q)
			if err != nil {
				return err
			}
			chain = append(chain, j)
		}
		chain = append(chain, jb)

		for i := 0; i+1 < len(chain); i++ {
			p, q := chain[i], chain[i+1]
			if d := dist(n.Junctions[p].Pos, n.Junctions[q].Pos); d < minTramwayLen && len(chain) == 2 {
				return fmt.Errorf(locale.T("轨道段太短（%.0f m），至少 %.0f m", "Segment too short (%.0f m), at least %.0f m"), d, minTramwayLen)
			}
			existing := RoadID(-1)
			for _, rd := range n.Roads {
				if (rd.A == p && rd.B == q) || (rd.A == q && rd.B == p) {
					existing = rd.ID
					break
				}
			}
			if existing >= 0 {
				rd := &n.Roads[existing]
				rd.Tram = Dedicated
				rd.Single = single && !rd.OneWay()
				if rd.Tramway {
					if single {
						rd.Fwd, rd.Back = 0, 0
					} else {
						rd.Fwd, rd.Back = 1, 1
					}
				}
				if rd.Single || rd.OneWay() {
					rd.Crossovers = nil
				}
				continue
			}
			f, bk := 1, 1
			if single {
				f, bk = 0, 0
			}
			n.Roads = append(n.Roads, Road{
				ID: RoadID(len(n.Roads)), A: p, B: q,
				Class: Street, Fwd: f, Back: bk,
				Tram: Dedicated, Single: single, Tramway: true, Level: lv,
			})
		}
		return nil
	})
}

// RemoveTramway demolishes a standalone tramway segment (off-road, elevated,
// underground, or ramp), dropping any stops on it.
func (n *Network) RemoveTramway(r RoadID) error {
	if r < 0 || int(r) >= len(n.Roads) || !n.Roads[r].Tramway {
		return errors.New(locale.T("这不是独立建设的轨道段", "Not a standalone tramway segment"))
	}
	a, b := n.Ends(r)
	la, lb := n.RoadLevels(r)
	return n.edit(&RoadEdit{Op: "rmtramway", A: a, B: b, LevelA: la, LevelB: lb}, func() error {
		for _, id := range n.StopIDs() {
			if n.Stops[id].Road == r {
				n.deleteStop(id)
			}
		}
		kept := make([]Road, 0, len(n.Roads)-1)
		for _, rd := range n.Roads {
			if rd.ID != r {
				kept = append(kept, rd)
			}
		}
		n.Roads = kept
		return nil
	})
}

// tramwayBetween finds a standalone tramway joining (a, lvA) and (b, lvB).
func (n *Network) tramwayBetween(a, b Vec, lvA, lvB int) (RoadID, bool) {
	ja, ok1 := n.NearestJunctionAt(a, lvA, replaySnap)
	jb, ok2 := n.NearestJunctionAt(b, lvB, replaySnap)
	if !ok1 || !ok2 {
		return -1, false
	}
	for _, rd := range n.Roads {
		if !rd.Tramway {
			continue
		}
		if (rd.A == ja && rd.B == jb) || (rd.A == jb && rd.B == ja) {
			return rd.ID, true
		}
	}
	return -1, false
}
