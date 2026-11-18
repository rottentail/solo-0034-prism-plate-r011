// Package scene defines the portable prism-plate-v1 document contract.
package scene

import "prismplate/internal/geom"

type Color struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}
type Stop struct {
	At    float64 `json:"at"`
	Color Color   `json:"color"`
}
type Brush struct {
	Kind  string     `json:"kind"`
	Color Color      `json:"color"`
	From  geom.Point `json:"from"`
	To    geom.Point `json:"to"`
	Stops []Stop     `json:"stops,omitempty"`
}

// Stroke describes an open-path pen. Width is interpreted in canvas units
// regardless of the shape transform, so mirroring and non-uniform scaling never
// distort the agreed stroke width.
type Stroke struct {
	Width      float64   `json:"width"`
	Cap        string    `json:"cap,omitempty"`
	Join       string    `json:"join,omitempty"`
	MiterLimit float64   `json:"miter_limit,omitempty"`
	Dash       []float64 `json:"dash,omitempty"`
	DashOffset float64   `json:"dash_offset,omitempty"`
}
type Shape struct {
	ID        string      `json:"id"`
	Open      bool        `json:"open,omitempty"`
	Path      []geom.Op   `json:"path"`
	FillRule  string      `json:"fill_rule"`
	Brush     Brush       `json:"brush"`
	Transform geom.Matrix `json:"transform"`
	Stroke    *Stroke     `json:"stroke,omitempty"`
}
type Layer struct {
	ID      string    `json:"id"`
	Opacity float64   `json:"opacity"`
	Blend   string    `json:"blend"`
	Clip    []geom.Op `json:"clip,omitempty"`
	Mask    []geom.Op `json:"mask,omitempty"`
	Shapes  []Shape   `json:"shapes"`
}
type Document struct {
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Background Color   `json:"background"`
	Layers     []Layer `json:"layers"`
}
type CompiledShape struct {
	Shape   Shape
	Path    geom.Path
	Open    geom.Path
	Strokes geom.Path
	Inverse geom.Matrix
}
type CompiledLayer struct {
	Layer  Layer
	Clip   geom.Path
	Mask   geom.Path
	Shapes []CompiledShape
}
type Compiled struct {
	Document       Document
	Layers         []CompiledLayer
	Segments       int
	StrokeVertices int
}
