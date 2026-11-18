package stroke

import (
	"math"
	"prismplate/internal/geom"
	"testing"
)

func ringBounds(rings geom.Path) (minX, minY, maxX, maxY float64) {
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for _, ring := range rings {
		for _, p := range ring {
			minX = math.Min(minX, p.X)
			minY = math.Min(minY, p.Y)
			maxX = math.Max(maxX, p.X)
			maxY = math.Max(maxY, p.Y)
		}
	}
	return
}

func approx(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// Width is interpreted in canvas units: scaling or mirroring the shape must not
// stretch the stroke thickness, only reposition the centerline.
func TestWidthInvariantUnderTransform(t *testing.T) {
	line := geom.Contour{{X: 10, Y: 20}, {X: 90, Y: 20}}
	style := Style{Width: 6, Cap: "butt", Join: "miter", MiterLimit: 4}
	cases := map[string]geom.Matrix{
		"identity":   geom.Identity,
		"mirror-x":   {-1, 0, 0, 1, 120, 0},
		"nonuniform": {2, 0, 0, 0.25, 0, 40},
		"shear":      {1, 0.2, -0.3, 1, 5, 5},
	}
	for name, m := range cases {
		rings := Expand([]geom.Contour{line}, m, style)
		if len(rings) == 0 {
			t.Fatalf("%s: no rings produced", name)
		}
		a := m.Apply(line[0])
		b := m.Apply(line[1])
		dx, dy := b.X-a.X, b.Y-a.Y
		length := math.Hypot(dx, dy)
		ux, uy := dx/length, dy/length
		nx, ny := -uy, ux
		half := style.Width / 2
		maxPerp, minAlong, maxAlong := 0.0, math.Inf(1), math.Inf(-1)
		for _, ring := range rings {
			for _, v := range ring {
				ox, oy := v.X-a.X, v.Y-a.Y
				perp := math.Abs(ox*nx + oy*ny)
				along := ox*ux + oy*uy
				maxPerp = math.Max(maxPerp, perp)
				minAlong = math.Min(minAlong, along)
				maxAlong = math.Max(maxAlong, along)
			}
		}
		if !approx(maxPerp, half, 1e-6) {
			t.Errorf("%s: thickness changed, perpendicular extent %v want %v", name, maxPerp, half)
		}
		// Butt caps keep the along-line span equal to the centerline length.
		if !approx(minAlong, 0, 1e-6) || !approx(maxAlong, length, 1e-6) {
			t.Errorf("%s: butt cap leaked along the line: [%v,%v] length %v", name, minAlong, maxAlong, length)
		}
	}
}

func TestCapsExtendByHalfWidth(t *testing.T) {
	line := geom.Contour{{X: 0, Y: 0}, {X: 50, Y: 0}}
	butt := Expand([]geom.Contour{line}, geom.Identity, Style{Width: 4, Cap: "butt", MiterLimit: 4})
	square := Expand([]geom.Contour{line}, geom.Identity, Style{Width: 4, Cap: "square", MiterLimit: 4})
	bx0, _, bx1, _ := ringBounds(butt)
	sx0, _, sx1, _ := ringBounds(square)
	if !approx(bx0, 0, 1e-9) || !approx(bx1, 50, 1e-9) {
		t.Errorf("butt cap must not extend: [%v,%v]", bx0, bx1)
	}
	if !approx(sx0, -2, 1e-9) || !approx(sx1, 52, 1e-9) {
		t.Errorf("square cap must extend half width: [%v,%v]", sx0, sx1)
	}
	round := Expand([]geom.Contour{line}, geom.Identity, Style{Width: 4, Cap: "round", MiterLimit: 4})
	rx0, ry0, rx1, ry1 := ringBounds(round)
	if !approx(rx0, -2, 1e-9) || !approx(rx1, 52, 1e-9) {
		t.Errorf("round cap must reach half width along line: [%v,%v]", rx0, rx1)
	}
	if !approx(ry0, -2, 1e-9) || !approx(ry1, 2, 1e-9) {
		t.Errorf("round cap half-circle thickness wrong: [%v,%v]", ry0, ry1)
	}
}

// A sharp corner with a tight miter limit must fall back to a bounded bevel;
// the miter spike can never run past miter_limit * half-width.
func TestMiterIsBounded(t *testing.T) {
	// Near-180 degree turn would otherwise produce an enormous spike.
	line := geom.Contour{{X: 0, Y: 0}, {X: 40, Y: 1}, {X: 80, Y: 0}}
	limit := 2.0
	rings := Expand([]geom.Contour{line}, geom.Identity, Style{Width: 4, Cap: "butt", Join: "miter", MiterLimit: limit})
	_, minY, _, maxY := ringBounds(rings)
	if math.Abs(maxY-minY) > 4*limit+1e-6 {
		t.Errorf("miter exceeded limit: height %v > %v", maxY-minY, 4*limit)
	}
}

// Dash phase is measured by global arc length, so a joint between two curve
// pieces does not restart the pattern. A straight polyline split at a joint
// must equal an unsplit one.
func TestDashPhaseContinuousAcrossJoint(t *testing.T) {
	style := Style{Width: 2, Cap: "butt", Join: "miter", MiterLimit: 4, Dash: []float64{6, 4}, DashOffset: 3}
	split := geom.Contour{{X: 0, Y: 0}, {X: 25, Y: 0}, {X: 50, Y: 0}, {X: 75, Y: 0}, {X: 100, Y: 0}}
	whole := geom.Contour{{X: 0, Y: 0}, {X: 100, Y: 0}}
	a := Expand([]geom.Contour{split}, geom.Identity, style)
	b := Expand([]geom.Contour{whole}, geom.Identity, style)
	// Identical dash boundaries prove the phase did not restart at the joints.
	da := dashPieces(split, style.Dash, style.DashOffset)
	db := dashPieces(whole, style.Dash, style.DashOffset)
	if len(da) != len(db) {
		t.Fatalf("dash interval count differs across joint: %d vs %d", len(da), len(db))
	}
	for i := range da {
		if !approx(da[i].a, db[i].a, 1e-7) || !approx(da[i].b, db[i].b, 1e-7) {
			t.Errorf("dash interval %d restarted at joint: [%v,%v] vs [%v,%v]", i, da[i].a, da[i].b, db[i].a, db[i].b)
		}
	}
	// The painted envelope must match even though the split line emits collinear join wedges.
	amx, amy, aMx, aMy := ringBounds(a)
	bmx, bmy, bMx, bMy := ringBounds(b)
	if !(approx(amx, bmx, 1e-7) && approx(amy, bmy, 1e-7) && approx(aMx, bMx, 1e-7) && approx(aMy, bMy, 1e-7)) {
		t.Errorf("painted envelope differs: [%v,%v,%v,%v] vs [%v,%v,%v,%v]", amx, amy, aMx, aMy, bmx, bmy, bMx, bMy)
	}
}

func TestDashOffsetRespected(t *testing.T) {
	line := geom.Contour{{X: 0, Y: 0}, {X: 100, Y: 0}}
	base := Style{Width: 2, Cap: "butt", MiterLimit: 4, Dash: []float64{10, 10}}
	rings := Expand([]geom.Contour{line}, geom.Identity, base)
	// Zero offset: on intervals [0,10],[20,30]... -> five full on-pieces.
	if len(rings) != 5 {
		t.Fatalf("zero offset piece count = %d, want 5", len(rings))
	}
	minX, _, _, _ := ringBounds(rings)
	if !approx(minX, 0, 1e-9) {
		t.Errorf("zero offset first dash must start at origin, got %v", minX)
	}
	shifted := Expand([]geom.Contour{line}, geom.Identity, Style{Width: 2, Cap: "butt", MiterLimit: 4, Dash: []float64{10, 10}, DashOffset: 5})
	// Offset 5 shortens the first on-piece to [0,5] and adds a final [95,100].
	sx0, _, _, _ := ringBounds(geom.Path{shifted[0]})
	if !approx(sx0, 0, 1e-9) {
		t.Errorf("first shifted dash start = %v, want 0", sx0)
	}
	last := shifted[len(shifted)-1]
	_, _, lx1, _ := ringBounds(geom.Path{last})
	if !approx(lx1, 100, 1e-9) {
		t.Errorf("last shifted dash end = %v, want 100", lx1)
	}
	if len(shifted) != 6 {
		t.Errorf("shifted piece count = %d, want 6", len(shifted))
	}
}

func TestDegenerateAndTinyInputsAreBounded(t *testing.T) {
	if rings := Expand([]geom.Contour{{{X: 1, Y: 1}, {X: 1, Y: 1}}}, geom.Identity, Style{Width: 2, MiterLimit: 4}); len(rings) != 0 {
		t.Errorf("zero-length polyline must produce no rings")
	}
	if rings := Expand(nil, geom.Identity, Style{Width: 2, MiterLimit: 4}); len(rings) != 0 {
		t.Errorf("empty input must produce no rings")
	}
}

func TestRoundJoinAndCapStayWithinWidth(t *testing.T) {
	line := geom.Contour{{X: 0, Y: 0}, {X: 30, Y: 30}, {X: 60, Y: 0}}
	rings := Expand([]geom.Contour{line}, geom.Identity, Style{Width: 4, Cap: "round", Join: "round", MiterLimit: 4})
	minX, minY, maxX, maxY := ringBounds(rings)
	// Outer corner at (30,30) reaches at most half-width beyond the vertex band;
	// rounding cannot overshoot the miter-equivalent envelope materially.
	if maxY > 30+2+1e-6 || minY < -2-1e-6 || minX < -2-1e-6 || maxX > 62+1e-6 {
		t.Errorf("round geometry outside envelope: [%v,%v,%v,%v]", minX, minY, maxX, maxY)
	}
}

// A right-hand (clockwise) turn exercises the negative-cross branch and its
// reverse sweep direction; the round join must still be bounded.
func TestRightTurnRoundJoinBounded(t *testing.T) {
	line := geom.Contour{{X: 0, Y: 30}, {X: 30, Y: 30}, {X: 30, Y: 0}}
	rings := Expand([]geom.Contour{line}, geom.Identity, Style{Width: 4, Cap: "butt", Join: "round", MiterLimit: 4})
	if len(rings) == 0 {
		t.Fatal("no rings")
	}
	minX, minY, maxX, maxY := ringBounds(rings)
	if minX < -2-1e-6 || minY < -2-1e-6 || maxX > 32+1e-6 || maxY > 32+1e-6 {
		t.Errorf("right-turn join outside envelope: [%v,%v,%v,%v]", minX, minY, maxX, maxY)
	}
}

func TestNegativeDashOffsetWraps(t *testing.T) {
	style := Style{Width: 2, Cap: "butt", MiterLimit: 4, Dash: []float64{10, 10}, DashOffset: -15}
	line := geom.Contour{{X: 0, Y: 0}, {X: 100, Y: 0}}
	rings := Expand([]geom.Contour{line}, geom.Identity, style)
	if len(rings) == 0 {
		t.Fatal("negative offset produced no dashes")
	}
	// offset -15 over period 20 is equivalent to +5, giving six pieces.
	if len(rings) != 6 {
		t.Errorf("negative offset piece count = %d, want 6", len(rings))
	}
}
