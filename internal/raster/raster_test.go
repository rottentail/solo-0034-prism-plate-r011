package raster

import (
	"bytes"
	"context"
	"image/png"
	"strings"
	"testing"

	"prismplate/internal/geom"
	"prismplate/internal/scene"
	"prismplate/internal/validate"
	"prismplate/internal/vector"
)

func strokeDoc() scene.Document {
	return scene.Document{
		Width:      120,
		Height:     80,
		Background: scene.Color{},
		Layers: []scene.Layer{{
			ID: "L", Opacity: 1, Blend: "normal",
			Shapes: []scene.Shape{{
				ID: "s", Open: true, FillRule: "nonzero",
				Path: []geom.Op{
					{Kind: "M", Points: []geom.Point{{X: 10, Y: 40}}},
					{Kind: "L", Points: []geom.Point{{X: 110, Y: 40}}},
				},
				Transform: geom.Identity,
				Brush:     scene.Brush{Kind: "solid", Color: scene.Color{R: 1, G: 1, B: 1, A: 1}},
				Stroke:    &scene.Stroke{Width: 10, Cap: "butt"},
			}},
		}},
	}
}

// Raster and SVG consume the same compiled stroke rings, and both stay within
// the fill-only output vocabulary.
func TestStrokeRendersInBothFormats(t *testing.T) {
	c, err := validate.Compile(strokeDoc())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	pngBytes, err := Render(context.Background(), c)
	if err != nil {
		t.Fatalf("raster: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	painted := 0
	for y := 0; y < 80; y++ {
		for x := 0; x < 120; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				painted++
			}
		}
	}
	// A 100-long, 10-wide butt-capped stroke covers exactly 1000 pixels.
	if painted != 1000 {
		t.Errorf("coverage = %d, want 1000", painted)
	}

	svg := string(vector.Export(c))
	if !strings.Contains(svg, "<path") {
		t.Error("SVG has no path output")
	}
	if strings.Contains(svg, "stroke=") || strings.Contains(svg, "<script") {
		t.Error("SVG must express strokes as fills, never raw stroke markup")
	}
}

func TestStrokeUnderMirrorKeepsCanvasWidth(t *testing.T) {
	mirror := geom.Matrix{-1, 0, 0, 1, 120, 0}
	d := strokeDoc()
	d.Layers[0].Shapes[0].Transform = mirror
	c, err := validate.Compile(d)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	out, err := Render(context.Background(), c)
	if err != nil {
		t.Fatalf("raster: %v", err)
	}
	img, _ := png.Decode(bytes.NewReader(out))
	heights := []int{}
	for y := 0; y < 80; y++ {
		if _, _, _, a := img.At(60, y).RGBA(); a > 0 {
			heights = append(heights, y)
		}
	}
	if got := heights[len(heights)-1] - heights[0] + 1; got != 10 {
		t.Errorf("mirrored stroke thickness = %d, want 10", got)
	}
}
