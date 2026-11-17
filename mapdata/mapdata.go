// Package mapdata holds real-world maps imported from OpenStreetMap by
// cmd/osmimport: road centre lines with lane counts, building footprints
// with heights, parks, water, trees and existing tram lines. Coordinates
// are metres, x east and y south of the map's north-west corner.
//
// Map data © OpenStreetMap contributors, available under the Open Database
// Licence (https://www.openstreetmap.org/copyright).
package mapdata

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"io"
	"math"
)

type Pt [2]float64

func (p Pt) X() float64 { return p[0] }
func (p Pt) Y() float64 { return p[1] }

// Road is one OSM way (or the part of it inside the map) carrying traffic.
type Road struct {
	Name   string `json:"n,omitempty"`
	Class  string `json:"c"` // primary, secondary, tertiary, residential, ...
	Fwd    int    `json:"f"` // lanes in the direction of Nodes
	Back   int    `json:"b"` // lanes against it (0 = one-way)
	Nodes  []int  `json:"p"` // indices into Map.Nodes
	Bridge bool   `json:"br,omitempty"`
	// Layer is the vertical level, -2 (deep tunnel) to 2 (very high
	// viaduct); 0 is the ground.
	Layer int `json:"l,omitempty"`
}

type Building struct {
	Pts    []Pt    `json:"p"`
	Height float64 `json:"h"`
	Kind   string  `json:"k,omitempty"`
}

// Area is a coloured ground polygon: park, grass, wood, water, plaza, parking.
type Area struct {
	Kind string `json:"k"`
	Pts  []Pt   `json:"p"`
}

type Stop struct {
	Name string `json:"n"`
	P    Pt     `json:"p"`
}

type Map struct {
	Name        string     `json:"name"`
	Attribution string     `json:"attribution"`
	W           float64    `json:"w"`
	H           float64    `json:"h"`
	Nodes       []Pt       `json:"nodes"`
	Roads       []Road     `json:"roads"`
	Buildings   []Building `json:"buildings"`
	Areas       []Area     `json:"areas"`
	Trees       []Pt       `json:"trees"`
	Tram        [][]Pt     `json:"tram"`
	TramStops   []Stop     `json:"tramStops"`
	// Highways are motorways, expressways and their ramps: drawn, but not
	// part of the street network.
	Highways  []Road     `json:"highways,omitempty"`
	Landmarks []Landmark `json:"landmarks,omitempty"`
}

// Landmark is a well-known place labelled on the map.
type Landmark struct {
	Name string `json:"n"`           // shown name
	OSM  string `json:"o,omitempty"` // name in OpenStreetMap
	P    Pt     `json:"p"`
	Rank int    `json:"r,omitempty"` // 1: major (shown from afar), 2: minor
}

//go:embed seattle.json.gz
var seattleGz []byte

//go:embed seattle-v1.json.gz
var seattleV1Gz []byte

//go:embed seattle-v2.json.gz
var seattleV2Gz []byte

//go:embed seattle-v3.json.gz
var seattleV3Gz []byte

// Seattle is central Seattle from Elliott Bay to Lake Washington: Downtown,
// Belltown, South Lake Union, Queen Anne, Eastlake, Capitol Hill, First
// Hill, Pioneer Square, the International District, the Central District,
// Madison Park and Leschi (bbox 47.592,-122.370,47.645,-122.272), with
// motorways and landmarks.
func Seattle() *Map { return mustDecode(seattleGz) }

// SeattleV1 is the first, smaller Seattle map (Downtown and South Lake
// Union, bbox 47.6045,-122.347,47.6295,-122.329), kept to bring saves made
// on it over to the current map.
func SeattleV1() *Map { return mustDecode(seattleV1Gz) }

// SeattleV2 is the second Seattle map (bbox 47.592,-122.370,47.645,-122.312,
// before it reached Lake Washington and had motorways), kept to bring saves
// made on it over. It shares the current map's origin and projection.
func SeattleV2() *Map { return mustDecode(seattleV2Gz) }

// SeattleV3 is the third Seattle map (to Lake Washington, before road
// levels and ramp junctions), kept to bring saves made on it over.
func SeattleV3() *Map { return mustDecode(seattleV3Gz) }

// SeattleV1Offset is where SeattleV1's origin lies on the current map, in
// metres: the difference of the two north-west corners.
var SeattleV1Offset = Pt{(0.023) * 111320 * math.Cos((47.592+47.645)/2*math.Pi/180), (47.645 - 47.6295) * 110574}

func mustDecode(gz []byte) *Map {
	m, err := Decode(gz)
	if err != nil {
		panic(err)
	}
	return m
}

func Decode(gz []byte) (*Map, error) {
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var m Map
	return &m, json.Unmarshal(data, &m)
}
