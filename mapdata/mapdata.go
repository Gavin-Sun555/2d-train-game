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
}

//go:embed seattle.json.gz
var seattleGz []byte

// Seattle is Downtown Seattle and South Lake Union.
func Seattle() *Map {
	m, err := Decode(seattleGz)
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
