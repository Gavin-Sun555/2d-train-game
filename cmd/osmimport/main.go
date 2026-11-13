// Command osmimport converts OpenStreetMap XML extracts (for example tiles
// from https://api.openstreetmap.org/api/0.6/map?bbox=...) into the compact
// map format of package mapdata.
//
//	go run ./cmd/osmimport -bbox 47.6045,-122.347,47.6295,-122.329 \
//	    -name "Seattle Downtown & SLU" -out mapdata/seattle.json.gz tiles/*.osm
//
// Water test points (-water lat,lon;...) pick which side of each coastline
// or lake shore is water when closing it along the map edge.
package main

import (
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/Gavin-Sun555/2d-train-game/mapdata"
)

type tag struct {
	K string `xml:"k,attr"`
	V string `xml:"v,attr"`
}

type osmNode struct {
	ID   int64   `xml:"id,attr"`
	Lat  float64 `xml:"lat,attr"`
	Lon  float64 `xml:"lon,attr"`
	Tags []tag   `xml:"tag"`
}

type osmWay struct {
	ID  int64 `xml:"id,attr"`
	Nds []struct {
		Ref int64 `xml:"ref,attr"`
	} `xml:"nd"`
	Tags []tag `xml:"tag"`
}

type osmRel struct {
	ID      int64 `xml:"id,attr"`
	Members []struct {
		Type string `xml:"type,attr"`
		Ref  int64  `xml:"ref,attr"`
		Role string `xml:"role,attr"`
	} `xml:"member"`
	Tags []tag `xml:"tag"`
}

type osmFile struct {
	Nodes []osmNode `xml:"node"`
	Ways  []osmWay  `xml:"way"`
	Rels  []osmRel  `xml:"relation"`
}

func tags(ts []tag) map[string]string {
	m := map[string]string{}
	for _, t := range ts {
		m[t.K] = t.V
	}
	return m
}

type node struct {
	lat, lon float64
	tags     map[string]string
}

type way struct {
	refs []int64
	tags map[string]string
}

var (
	nodes = map[int64]node{}
	ways  = map[int64]way{}
	rels  = map[int64]osmRel{}
)

// projection
var n0, w0, s0, e0, kx, ky float64

func project(lat, lon float64) mapdata.Pt {
	return mapdata.Pt{round((lon - w0) * kx), round((n0 - lat) * ky)}
}

func round(v float64) float64 { return math.Round(v*10) / 10 }

func inside(lat, lon float64) bool { return lat >= s0 && lat <= n0 && lon >= w0 && lon <= e0 }

func main() {
	bbox := flag.String("bbox", "47.6045,-122.347,47.6295,-122.329", "south,west,north,east")
	name := flag.String("name", "Seattle Downtown & SLU", "map name")
	out := flag.String("out", "mapdata/seattle.json.gz", "output file")
	water := flag.String("water", "47.6060,-122.3462;47.6292,-122.3380", "water test points lat,lon;...")
	flag.Parse()
	if _, err := fmt.Sscanf(*bbox, "%f,%f,%f,%f", &s0, &w0, &n0, &e0); err != nil {
		log.Fatalf("-bbox: %v", err)
	}
	mid := (s0 + n0) / 2
	kx, ky = 111320*math.Cos(mid*math.Pi/180), 110574

	for _, f := range flag.Args() {
		data, err := os.ReadFile(f)
		if err != nil {
			log.Fatal(err)
		}
		var of osmFile
		if err := xml.Unmarshal(data, &of); err != nil {
			log.Fatalf("%s: %v", f, err)
		}
		for _, n := range of.Nodes {
			nodes[n.ID] = node{n.Lat, n.Lon, tags(n.Tags)}
		}
		for _, w := range of.Ways {
			refs := make([]int64, len(w.Nds))
			for i, nd := range w.Nds {
				refs[i] = nd.Ref
			}
			ways[w.ID] = way{refs, tags(w.Tags)}
		}
		for _, r := range of.Rels {
			rels[r.ID] = r
		}
	}

	m := &mapdata.Map{Name: *name, Attribution: "© OpenStreetMap contributors (ODbL)"}
	br := project(s0, e0)
	m.W, m.H = br[0], br[1]

	ids := sortedWayIDs()
	importRoads(m, ids)
	importBuildings(m, ids)
	importBuildingRelations(m)
	importAreas(m, ids)
	importWater(m, ids, *water)
	importTrees(m)
	importTram(m, ids)

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	zw, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err := json.NewEncoder(zw).Encode(m); err != nil {
		log.Fatal(err)
	}
	zw.Close()
	f.Close()
	log.Printf("%s: %.0f×%.0f m, %d road nodes, %d roads, %d buildings, %d areas, %d trees, %d tram ways, %d tram stops",
		*out, m.W, m.H, len(m.Nodes), len(m.Roads), len(m.Buildings), len(m.Areas), len(m.Trees), len(m.Tram), len(m.TramStops))
}

func sortedWayIDs() []int64 {
	ids := make([]int64, 0, len(ways))
	for id := range ways {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

var roadClasses = map[string]bool{
	"primary": true, "secondary": true, "tertiary": true, "unclassified": true,
	"residential": true, "living_street": true,
	"primary_link": true, "secondary_link": true, "tertiary_link": true,
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(strings.SplitN(s, ";", 2)[0]))
	return n
}

func importRoads(m *mapdata.Map, ids []int64) {
	index := map[int64]int{}
	nodeIndex := func(id int64) int {
		if i, ok := index[id]; ok {
			return i
		}
		n := nodes[id]
		index[id] = len(m.Nodes)
		m.Nodes = append(m.Nodes, project(n.lat, n.lon))
		return index[id]
	}
	for _, id := range ids {
		w := ways[id]
		hw := w.tags["highway"]
		if !roadClasses[hw] || w.tags["area"] == "yes" || w.tags["tunnel"] == "yes" {
			continue
		}
		refs := append([]int64(nil), w.refs...)
		ow := w.tags["oneway"]
		if ow == "-1" {
			for i, j := 0, len(refs)-1; i < j; i, j = i+1, j-1 {
				refs[i], refs[j] = refs[j], refs[i]
			}
		}
		oneway := ow == "yes" || ow == "1" || ow == "-1" || w.tags["junction"] == "roundabout"
		lanes := atoi(w.tags["lanes"])
		fwd, back := atoi(w.tags["lanes:forward"]), atoi(w.tags["lanes:backward"])
		if ow == "-1" {
			fwd, back = back, fwd
		}
		if oneway {
			if fwd == 0 {
				fwd = lanes
			}
			if fwd == 0 {
				fwd = 1
				if hw == "primary" || hw == "secondary" {
					fwd = 2
				}
			}
			back = 0
		} else {
			if fwd == 0 && back == 0 {
				fwd, back = max(1, lanes/2), max(1, lanes/2)
			}
			fwd, back = max(fwd, 1), max(back, 1)
		}
		class := strings.TrimSuffix(hw, "_link")
		// Split into runs of nodes inside the map.
		var run []int
		flush := func() {
			if len(run) >= 2 {
				m.Roads = append(m.Roads, mapdata.Road{Name: w.tags["name"], Class: class, Fwd: fwd, Back: back,
					Nodes: run, Bridge: w.tags["bridge"] == "yes"})
			}
			run = nil
		}
		for _, r := range refs {
			n, ok := nodes[r]
			if !ok || !inside(n.lat, n.lon) {
				flush()
				continue
			}
			run = append(run, nodeIndex(r))
		}
		flush()
	}
}

func ring(w way) ([]mapdata.Pt, bool) {
	if len(w.refs) < 4 || w.refs[0] != w.refs[len(w.refs)-1] {
		return nil, false
	}
	pts := make([]mapdata.Pt, 0, len(w.refs)-1)
	for _, r := range w.refs[:len(w.refs)-1] {
		n, ok := nodes[r]
		if !ok {
			return nil, false
		}
		pts = append(pts, project(n.lat, n.lon))
	}
	return pts, true
}

func centroidInside(pts []mapdata.Pt) bool {
	var x, y float64
	for _, p := range pts {
		x += p[0]
		y += p[1]
	}
	x, y = x/float64(len(pts)), y/float64(len(pts))
	br := project(s0, e0)
	return x >= 0 && y >= 0 && x <= br[0] && y <= br[1]
}

func parseHeight(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "m"))
	if strings.HasSuffix(s, "'") || strings.HasSuffix(s, "ft") {
		v, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(s, "ft"), "'")), 64)
		return v * 0.3048
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func importBuildings(m *mapdata.Map, ids []int64) {
	for _, id := range ids {
		w := ways[id]
		b, ok := w.tags["building"]
		if !ok || b == "no" {
			continue
		}
		if w.tags["layer"] != "" && atoi(w.tags["layer"]) < 0 {
			continue
		}
		pts, ok := ring(w)
		if !ok || !centroidInside(pts) {
			continue
		}
		h := parseHeight(w.tags["height"])
		if h == 0 {
			if lv := atoi(w.tags["building:levels"]); lv > 0 {
				h = float64(lv) * 3.5
			}
		}
		if h == 0 {
			h = 10
		}
		if b == "roof" || w.tags["building:part"] != "" {
			h = math.Min(h, 6)
		}
		m.Buildings = append(m.Buildings, mapdata.Building{Pts: pts, Height: round(h), Kind: b})
	}
}

// importBuildingRelations adds buildings mapped as multipolygons (outer
// rings only; courtyards are rare and small at this scale).
func importBuildingRelations(m *mapdata.Map) {
	var ids []int64
	for id, r := range rels {
		t := tags(r.Tags)
		if t["type"] == "multipolygon" && t["building"] != "" && t["building"] != "no" {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		r := rels[id]
		t := tags(r.Tags)
		outer := map[int64]bool{}
		for _, mb := range r.Members {
			if _, ok := ways[mb.Ref]; ok && mb.Type == "way" && mb.Role == "outer" {
				outer[mb.Ref] = true
			}
		}
		for _, chain := range chains(outer) {
			if len(chain) < 4 || chain[0] != chain[len(chain)-1] {
				continue // ring not complete in the extract
			}
			pts, ok := ring(way{refs: chain})
			if !ok || !centroidInside(pts) {
				continue
			}
			h := parseHeight(t["height"])
			if h == 0 {
				if lv := atoi(t["building:levels"]); lv > 0 {
					h = float64(lv) * 3.5
				}
			}
			if h == 0 {
				h = 10
			}
			m.Buildings = append(m.Buildings, mapdata.Building{Pts: pts, Height: round(h), Kind: t["building"]})
		}
	}
}

func areaKind(t map[string]string) string {
	switch {
	case t["leisure"] == "park" || t["leisure"] == "garden" || t["leisure"] == "dog_park" || t["leisure"] == "playground":
		return "park"
	case t["leisure"] == "pitch":
		return "pitch"
	case t["landuse"] == "grass" || t["landuse"] == "flowerbed" || t["landuse"] == "village_green" || t["landuse"] == "recreation_ground":
		return "grass"
	case t["natural"] == "wood" || t["natural"] == "scrub" || t["landuse"] == "forest" || t["natural"] == "shrubbery":
		return "wood"
	case t["natural"] == "water" || t["leisure"] == "swimming_pool":
		return "water"
	case t["amenity"] == "parking":
		return "parking"
	case t["place"] == "square" || (t["highway"] == "pedestrian" && t["area"] == "yes"):
		return "plaza"
	}
	return ""
}

func importAreas(m *mapdata.Map, ids []int64) {
	for _, id := range ids {
		w := ways[id]
		k := areaKind(w.tags)
		if k == "" || w.tags["building"] != "" {
			continue
		}
		pts, ok := ring(w)
		if !ok || !centroidInside(pts) {
			continue
		}
		m.Areas = append(m.Areas, mapdata.Area{Kind: k, Pts: pts})
	}
}

// importWater turns coastline and lake shore pieces into closed polygons,
// closing each shore chain along the map edge on its water side.
func importWater(m *mapdata.Map, ids []int64, testPts string) {
	var tests []mapdata.Pt
	for _, s := range strings.Split(testPts, ";") {
		var lat, lon float64
		if _, err := fmt.Sscanf(s, "%f,%f", &lat, &lon); err == nil {
			tests = append(tests, project(lat, lon))
		}
	}
	// Shore pieces: coastline ways plus outer members of water relations.
	pieces := map[int64]bool{}
	for _, id := range ids {
		if ways[id].tags["natural"] == "coastline" {
			pieces[id] = true
		}
	}
	for _, r := range rels {
		t := tags(r.Tags)
		if t["natural"] != "water" {
			continue
		}
		for _, mb := range r.Members {
			if _, ok := ways[mb.Ref]; ok && mb.Type == "way" && mb.Role == "outer" {
				pieces[mb.Ref] = true
			}
		}
	}
	for _, chain := range chains(pieces) {
		if len(chain) < 2 {
			continue
		}
		var pts []mapdata.Pt
		for _, r := range chain {
			if n, ok := nodes[r]; ok {
				pts = append(pts, clampOut(project(n.lat, n.lon), m))
			}
		}
		if len(pts) < 2 {
			continue
		}
		poly := closeAlongEdge(pts, m, tests)
		if poly != nil {
			m.Areas = append([]mapdata.Area{{Kind: "water", Pts: poly}}, m.Areas...)
		}
	}
}

// chains joins ways sharing end nodes into longer node chains.
func chains(set map[int64]bool) [][]int64 {
	var ids []int64
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var cs [][]int64
	for _, id := range ids {
		cs = append(cs, append([]int64(nil), ways[id].refs...))
	}
	for merged := true; merged; {
		merged = false
		for i := 0; i < len(cs) && !merged; i++ {
			for j := 0; j < len(cs) && !merged; j++ {
				if i == j {
					continue
				}
				a, b := cs[i], cs[j]
				var joined []int64
				switch {
				case a[len(a)-1] == b[0]:
					joined = append(a, b[1:]...)
				case a[len(a)-1] == b[len(b)-1]:
					joined = append(a, reversed(b)[1:]...)
				case a[0] == b[0]:
					joined = append(reversed(b), a[1:]...)
				}
				if joined != nil {
					cs[i] = joined
					cs = append(cs[:j], cs[j+1:]...)
					merged = true
				}
			}
		}
	}
	return cs
}

func reversed(a []int64) []int64 {
	out := make([]int64, len(a))
	for i, v := range a {
		out[len(a)-1-i] = v
	}
	return out
}

const edgePad = 60.0

// clampOut pulls far-away shore points onto a box just outside the map.
func clampOut(p mapdata.Pt, m *mapdata.Map) mapdata.Pt {
	return mapdata.Pt{math.Max(-edgePad, math.Min(m.W+edgePad, p[0])), math.Max(-edgePad, math.Min(m.H+edgePad, p[1]))}
}

// perimeter position (clockwise from the north-west corner) of a point on
// the padded box.
func perim(p mapdata.Pt, m *mapdata.Map) float64 {
	x0, y0, x1, y1 := -edgePad, -edgePad, m.W+edgePad, m.H+edgePad
	w, h := x1-x0, y1-y0
	d := []float64{math.Abs(p[1] - y0), math.Abs(p[0] - x1), math.Abs(p[1] - y1), math.Abs(p[0] - x0)}
	best := 0
	for i := range d {
		if d[i] < d[best] {
			best = i
		}
	}
	switch best {
	case 0:
		return p[0] - x0
	case 1:
		return w + (p[1] - y0)
	case 2:
		return w + h + (x1 - p[0])
	}
	return 2*w + h + (y1 - p[1])
}

func closeAlongEdge(pts []mapdata.Pt, m *mapdata.Map, tests []mapdata.Pt) []mapdata.Pt {
	x0, y0, x1, y1 := -edgePad, -edgePad, m.W+edgePad, m.H+edgePad
	w, h := x1-x0, y1-y0
	total := 2 * (w + h)
	corners := []struct {
		t float64
		p mapdata.Pt
	}{{w, mapdata.Pt{x1, y0}}, {w + h, mapdata.Pt{x1, y1}}, {2*w + h, mapdata.Pt{x0, y1}}, {total, mapdata.Pt{x0, y0}}}
	end, start := perim(pts[len(pts)-1], m), perim(pts[0], m)
	build := func(clockwise bool) []mapdata.Pt {
		poly := append([]mapdata.Pt(nil), pts...)
		if clockwise {
			span := math.Mod(start-end+total, total)
			for k := 0; k < 8; k++ {
				c := corners[k%4]
				d := math.Mod(c.t-end+total, total)
				if d > 0 && d < span {
					poly = append(poly, c.p)
				}
			}
			sortByOffset(poly[len(pts):], end, total, func(p mapdata.Pt) float64 { return perim(p, m) }, true)
		} else {
			span := math.Mod(end-start+total, total)
			for k := 0; k < 4; k++ {
				c := corners[k]
				d := math.Mod(end-c.t+total, total)
				if d > 0 && d < span {
					poly = append(poly, c.p)
				}
			}
			sortByOffset(poly[len(pts):], end, total, func(p mapdata.Pt) float64 { return perim(p, m) }, false)
		}
		return dedupe(poly)
	}
	// Of the two ways to close the shore, take the smaller one that holds
	// a known water point: the other side is the land.
	var best []mapdata.Pt
	bestArea := math.Inf(1)
	for _, cw := range []bool{true, false} {
		poly := build(cw)
		for _, t := range tests {
			// A short shore piece whose ends both meet the same map edge
			// would otherwise "close" around the whole map.
			tooBig := math.Abs(polyArea(poly)) > 0.4*m.W*m.H
			if a := math.Abs(polyArea(poly)); contains(poly, t) && a < bestArea && !tooBig {
				best, bestArea = poly, a
			}
		}
	}
	return best
}

func polyArea(p []mapdata.Pt) float64 {
	a := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		a += p[i][0]*p[j][1] - p[j][0]*p[i][1]
	}
	return a / 2
}

func sortByOffset(ps []mapdata.Pt, from, total float64, pos func(mapdata.Pt) float64, forward bool) {
	sort.SliceStable(ps, func(i, j int) bool {
		di, dj := math.Mod(pos(ps[i])-from+total, total), math.Mod(pos(ps[j])-from+total, total)
		if !forward {
			di, dj = math.Mod(from-pos(ps[i])+total, total), math.Mod(from-pos(ps[j])+total, total)
		}
		return di < dj
	})
}

func dedupe(ps []mapdata.Pt) []mapdata.Pt {
	var out []mapdata.Pt
	for _, p := range ps {
		if len(out) == 0 || out[len(out)-1] != p {
			out = append(out, p)
		}
	}
	return out
}

func contains(poly []mapdata.Pt, p mapdata.Pt) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a[1] > p[1]) != (b[1] > p[1]) && p[0] < (b[0]-a[0])*(p[1]-a[1])/(b[1]-a[1])+a[0] {
			in = !in
		}
	}
	return in
}

func importTrees(m *mapdata.Map) {
	var ids []int64
	for id, n := range nodes {
		if n.tags["natural"] == "tree" && inside(n.lat, n.lon) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		n := nodes[id]
		m.Trees = append(m.Trees, project(n.lat, n.lon))
	}
}

func importTram(m *mapdata.Map, ids []int64) {
	for _, id := range ids {
		w := ways[id]
		if w.tags["railway"] != "tram" {
			continue
		}
		var pts []mapdata.Pt
		for _, r := range w.refs {
			if n, ok := nodes[r]; ok {
				pts = append(pts, project(n.lat, n.lon))
			}
		}
		if len(pts) >= 2 {
			m.Tram = append(m.Tram, pts)
		}
	}
	var sids []int64
	for id, n := range nodes {
		if n.tags["railway"] == "tram_stop" && inside(n.lat, n.lon) {
			sids = append(sids, id)
		}
	}
	sort.Slice(sids, func(i, j int) bool { return sids[i] < sids[j] })
	for _, id := range sids {
		n := nodes[id]
		m.TramStops = append(m.TramStops, mapdata.Stop{Name: n.tags["name"], P: project(n.lat, n.lon)})
	}
}
