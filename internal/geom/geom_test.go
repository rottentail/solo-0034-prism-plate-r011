package geom

import "testing"

func TestFlattenOpenSeparatesPolylines(t *testing.T) {
	ops := []Op{
		{Kind: "M", Points: []Point{{X: 0, Y: 0}}},
		{Kind: "L", Points: []Point{{X: 10, Y: 0}}},
		{Kind: "L", Points: []Point{{X: 10, Y: 10}}},
		{Kind: "Z"},
		{Kind: "M", Points: []Point{{X: 20, Y: 20}}},
		{Kind: "L", Points: []Point{{X: 40, Y: 20}}},
	}
	closed, open := FlattenOpen(ops)
	if len(closed) != 1 || len(open) != 1 {
		t.Fatalf("expected 1 ring and 1 polyline, got %d/%d", len(closed), len(open))
	}
	if open[0][0] != (Point{X: 20, Y: 20}) || open[0][len(open[0])-1] != (Point{X: 40, Y: 20}) {
		t.Errorf("open polyline endpoints wrong: %v", open[0])
	}
	// Legacy Flatten keeps only closed contours.
	if rings := Flatten(ops); len(rings) != 1 {
		t.Errorf("legacy Flatten must ignore open lines, got %d", len(rings))
	}
}

func TestFlattenCurveSubdivisionCounts(t *testing.T) {
	ops := []Op{
		{Kind: "M", Points: []Point{{X: 0, Y: 0}}},
		{Kind: "Q", Points: []Point{{X: 5, Y: 5}, {X: 10, Y: 0}}},
		{Kind: "L", Points: []Point{{X: 20, Y: 0}}},
	}
	_, open := FlattenOpen(ops)
	if len(open) != 1 {
		t.Fatalf("expected one open polyline, got %d", len(open))
	}
	// M point + 16 quadratic subdivisions + 1 line point.
	if len(open[0]) != 1+16+1 {
		t.Errorf("quadratic subdivision count = %d, want 18", len(open[0]))
	}
}

func TestContourContains(t *testing.T) {
	ring := Contour{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	if !ring.Contains(Point{X: 5, Y: 5}) {
		t.Error("interior point reported outside")
	}
	if ring.Contains(Point{X: 50, Y: 50}) {
		t.Error("exterior point reported inside")
	}
}
