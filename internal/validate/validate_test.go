package validate

import (
	"prismplate/internal/geom"
	"prismplate/internal/scene"
	"strings"
	"testing"
)

func solidShape(open bool, path []geom.Op, stroke *scene.Stroke) scene.Shape {
	return scene.Shape{
		ID:        "s",
		Open:      open,
		Path:      path,
		FillRule:  "nonzero",
		Brush:     scene.Brush{Kind: "solid", Color: scene.Color{R: 1, G: 1, B: 1, A: 1}},
		Transform: geom.Identity,
		Stroke:    stroke,
	}
}

func docWith(shapes ...scene.Shape) scene.Document {
	return scene.Document{
		Width:      128,
		Height:     128,
		Background: scene.Color{A: 1},
		Layers:     []scene.Layer{{ID: "L", Opacity: 1, Blend: "normal", Shapes: shapes}},
	}
}

func linePath() []geom.Op {
	return []geom.Op{
		{Kind: "M", Points: []geom.Point{{X: 10, Y: 40}}},
		{Kind: "L", Points: []geom.Point{{X: 110, Y: 40}}},
	}
}

func TestAdmitsOpenStrokeAndExpandsRings(t *testing.T) {
	d := docWith(solidShape(true, linePath(), &scene.Stroke{Width: 8, Cap: "round", Join: "bevel"}))
	c, err := Compile(d)
	if err != nil {
		t.Fatalf("open stroke rejected: %v", err)
	}
	var rings int
	for _, l := range c.Layers {
		for _, s := range l.Shapes {
			if len(s.Open) != 1 {
				t.Errorf("expected one open polyline, got %d", len(s.Open))
			}
			rings += len(s.Strokes)
		}
	}
	if rings == 0 {
		t.Fatal("stroke produced no fill rings")
	}
}

func TestStrokeAdmissionFailures(t *testing.T) {
	cases := map[string]*scene.Stroke{
		"zero-width":    {Width: 0},
		"huge-width":    {Width: 200},
		"bad-cap":       {Width: 4, Cap: "pointy"},
		"bad-join":      {Width: 4, Join: "glue"},
		"low-miter":     {Width: 4, MiterLimit: 0.1},
		"odd-dash":      {Width: 4, Dash: []float64{5}},
		"zero-dash":     {Width: 4, Dash: []float64{0, 0}},
		"negative-dash": {Width: 4, Dash: []float64{-1, 2}},
	}
	for name, st := range cases {
		if _, err := Compile(docWith(solidShape(true, linePath(), st))); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

func TestOpenContourRequiresStroke(t *testing.T) {
	_, err := Compile(docWith(solidShape(true, linePath(), nil)))
	if err == nil || !strings.Contains(err.Error(), "stroke") {
		t.Fatalf("expected stroke-required error, got %v", err)
	}
}

func TestClosedPathRemainsFillOnly(t *testing.T) {
	closed := []geom.Op{
		{Kind: "M", Points: []geom.Point{{X: 10, Y: 10}}},
		{Kind: "L", Points: []geom.Point{{X: 60, Y: 10}}},
		{Kind: "L", Points: []geom.Point{{X: 60, Y: 60}}},
		{Kind: "Z"},
	}
	c, err := Compile(docWith(solidShape(false, closed, nil)))
	if err != nil {
		t.Fatalf("legacy fill rejected: %v", err)
	}
	for _, l := range c.Layers {
		for _, s := range l.Shapes {
			if len(s.Path) != 1 || len(s.Strokes) != 0 {
				t.Errorf("closed fill changed semantics: rings=%d strokes=%d", len(s.Path), len(s.Strokes))
			}
		}
	}
}

func TestMixedOpenAndClosedContours(t *testing.T) {
	ops := []geom.Op{
		{Kind: "M", Points: []geom.Point{{X: 10, Y: 10}}},
		{Kind: "L", Points: []geom.Point{{X: 60, Y: 10}}},
		{Kind: "L", Points: []geom.Point{{X: 60, Y: 60}}},
		{Kind: "Z"},
		{Kind: "M", Points: []geom.Point{{X: 20, Y: 80}}},
		{Kind: "L", Points: []geom.Point{{X: 100, Y: 80}}},
	}
	d := docWith(solidShape(true, ops, &scene.Stroke{Width: 4}))
	c, err := Compile(d)
	if err != nil {
		t.Fatalf("mixed contours rejected: %v", err)
	}
	s := c.Layers[0].Shapes[0]
	if len(s.Path) != 1 || len(s.Open) != 1 {
		t.Errorf("expected 1 fill ring and 1 open line, got %d/%d", len(s.Path), len(s.Open))
	}
	if len(s.Strokes) == 0 {
		t.Error("open contour not stroked")
	}
}

func TestGradientStrokeAdmitted(t *testing.T) {
	s := solidShape(true, linePath(), &scene.Stroke{Width: 6, Dash: []float64{8, 4}})
	s.Brush = scene.Brush{
		Kind: "linear",
		From: geom.Point{X: 0, Y: 0},
		To:   geom.Point{X: 128, Y: 0},
		Stops: []scene.Stop{
			{At: 0, Color: scene.Color{R: 1, A: 1}},
			{At: 1, Color: scene.Color{B: 1, A: 1}},
		},
	}
	if _, err := Compile(docWith(s)); err != nil {
		t.Fatalf("gradient stroke rejected: %v", err)
	}
}
