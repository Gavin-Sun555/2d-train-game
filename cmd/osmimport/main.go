// Command osmimport converts OpenStreetMap XML extracts (for example tiles
// from https://api.openstreetmap.org/api/0.6/map?bbox=...) into the compact
// map format of package mapdata.
//
//	go run ./cmd/osmimport -bbox 47.592,-122.370,47.645,-122.312 \
//	    -name "Seattle" -out mapdata/seattle.json.gz seattle.osm
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
	bbox := flag.String("bbox", "47.592,-122.370,47.645,-122.312", "south,west,north,east")
	name := flag.String("name", "Seattle", "map name")
	out := flag.String("out", "mapdata/seattle.json.gz", "output file")
	water := flag.String("water", "47.6050,-122.3560;47.6350,-122.3340", "water test points lat,lon;...")
	landmarks := flag.String("landmarks", "", "JSON list of named landmarks to place ([{name, osm, lat, lon, rank}])")
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
	importAreaRelations(m)
	importWater(m, ids, *water)
	importTrees(m)
	importTram(m, ids)
	importLandmarks(m, *landmarks)

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
	log.Printf("%d highway pieces, %d landmarks", len(m.Highways), len(m.Landmarks))
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

var nodeIdx = map[int64]int{}

func nodeIndex(m *mapdata.Map, id int64) int {
	if i, ok := nodeIdx[id]; ok {
		return i
	}
	n := nodes[id]
	nodeIdx[id] = len(m.Nodes)
	m.Nodes = append(m.Nodes, project(n.lat, n.lon))
	return nodeIdx[id]
}

// Motorways, expressways and their ramps: drawn on the map, but not part of
// the street network (no trams, no junctions with the streets they cross).
var highwayClasses = map[string]bool{
	"motorway": true, "motorway_link": true, "trunk": true, "trunk_link": true,
}

func importRoads(m *mapdata.Map, ids []int64) {
	importWays(m, ids, roadClasses, &m.Roads)
	importWays(m, ids, highwayClasses, &m.Highways)
}

func importWays(m *mapdata.Map, ids []int64, classes map[string]bool, into *[]mapdata.Road) {
	for _, id := range ids {
		w := ways[id]
		hw := w.tags["highway"]
		tunnel := w.tags["tunnel"] != "" && w.tags["tunnel"] != "no" && w.tags["tunnel"] != "building_passage"
		// Streets in tunnels are left out; motorway tunnels are kept on
		// their underground level.
		if !classes[hw] || w.tags["area"] == "yes" || (tunnel && !classes["motorway"]) {
			continue
		}
		switch w.tags["access"] {
		case "private", "no", "customers", "delivery", "permissive", "destination":
			if classes["motorway"] {
				continue // ferry holding lanes and the like are not motorways
			}
		}
		layer := atoi(w.tags["layer"])
		bridgeTag := w.tags["bridge"] != "" && w.tags["bridge"] != "no"
		switch {
		case tunnel:
			layer = min(layer, -1)
		case bridgeTag:
			layer = max(layer, 1)
		case layer < 0:
			layer = 0 // covered by a building, or a cutting: still the ground
		}
		layer = min(max(layer, -2), 2)
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
				switch hw {
				case "primary", "secondary", "trunk":
					fwd = 2
				case "motorway":
					fwd = 3
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
				bridge := bridgeTag
				if classes["motorway"] {
					class = hw // ramps keep their own class
					bridge = bridge || layer > 0
				}
				*into = append(*into, mapdata.Road{Name: w.tags["name"], Class: class, Fwd: fwd, Back: back,
					Nodes: run, Bridge: bridge, Layer: layer})
			}
			run = nil
		}
		// Motorways are cut exactly at the map edge, so they run off it
		// rather than stopping short.
		clip := classes["motorway"]
		var prev int64
		havePrev := false
		for _, r := range refs {
			n, ok := nodes[r]
			if !ok {
				flush()
				havePrev = false
				continue
			}
			in := inside(n.lat, n.lon)
			if clip && havePrev {
				p := nodes[prev]
				if pin := inside(p.lat, p.lon); pin != in {
					run = append(run, edgeNode(m, p, n))
				}
			}
			prev, havePrev = r, true
			if !in {
				flush()
				continue
			}
			run = append(run, nodeIndex(m, r))
		}
		flush()
	}
}

// edgeNode adds the point where the segment a–b crosses the map edge.
func edgeNode(m *mapdata.Map, a, b node) int {
	lo, hi := 0.0, 1.0
	ain := inside(a.lat, a.lon)
	for i := 0; i < 40; i++ {
		t := (lo + hi) / 2
		if inside(a.lat+(b.lat-a.lat)*t, a.lon+(b.lon-a.lon)*t) == ain {
			lo = t
		} else {
			hi = t
		}
	}
	t := lo
	if !ain {
		t = hi
	}
	m.Nodes = append(m.Nodes, project(a.lat+(b.lat-a.lat)*t, a.lon+(b.lon-a.lon)*t))
	return len(m.Nodes) - 1
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

// importAreaRelations adds parks, woods and the like mapped as
// multipolygons (outer rings).
func importAreaRelations(m *mapdata.Map) {
	var ids []int64
	for id, r := range rels {
		t := tags(r.Tags)
		if t["type"] == "multipolygon" && t["natural"] != "water" && t["building"] == "" && areaKind(t) != "" {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		r := rels[id]
		k := areaKind(tags(r.Tags))
		outer := map[int64]bool{}
		for _, mb := range r.Members {
			if _, ok := ways[mb.Ref]; ok && mb.Type == "way" && mb.Role == "outer" {
				outer[mb.Ref] = true
			}
		}
		for _, chain := range chains(outer) {
			if len(chain) < 4 || chain[0] != chain[len(chain)-1] {
				continue
			}
			pts, ok := ring(way{refs: chain})
			if !ok || !overlaps(pts) {
				continue
			}
			m.Areas = append(m.Areas, mapdata.Area{Kind: k, Pts: pts})
		}
	}
}

// overlaps reports whether a polygon's bounding box meets the map.
func overlaps(pts []mapdata.Pt) bool {
	br := project(s0, e0)
	x0, y0, x1, y1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		x0, y0, x1, y1 = math.Min(x0, p[0]), math.Min(y0, p[1]), math.Max(x1, p[0]), math.Max(y1, p[1])
	}
	return x1 >= 0 && y1 >= 0 && x0 <= br[0] && y0 <= br[1]
}

// importLandmarks places named landmarks from a JSON list
// ([{name, osm, lat, lon, rank}]) on the map.
func importLandmarks(m *mapdata.Map, path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var list []struct {
		Name, OSM string
		Lat, Lon  float64
		Rank      int
	}
	if err := json.Unmarshal(data, &list); err != nil {
		log.Fatalf("%s: %v", path, err)
	}
	for _, l := range list {
		if inside(l.Lat, l.Lon) {
			m.Landmarks = append(m.Landmarks, mapdata.Landmark{Name: l.Name, OSM: l.OSM, P: project(l.Lat, l.Lon), Rank: l.Rank})
		}
	}
}

func areaKind(t map[string]string) string {
	switch {
	case t["leisure"] == "park" || t["leisure"] == "garden" || t["leisure"] == "dog_park" || t["leisure"] == "playground":
		return "park"
	case t["leisure"] == "pitch" || t["leisure"] == "golf_course":
		return "pitch"
	case t["landuse"] == "grass" || t["landuse"] == "flowerbed" || t["landuse"] == "village_green" || t["landuse"] == "recreation_ground" ||
		t["landuse"] == "cemetery" || t["leisure"] == "nature_reserve":
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
