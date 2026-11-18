// Package stroke expands open polylines into bounded fill contours with explicit
// end caps, corner joins and arc-length dashing. All geometry is produced in
// canvas coordinates so the agreed stroke width survives mirroring and
// non-uniform scaling of the shape transform.
package stroke

import (
	"math"
	"prismplate/internal/geom"
)

// Round arcs are tessellated once with a fixed contract shared by PNG and SVG.
const (
	roundSegments = 12
	epsilon       = 1e-9
)

// Style is the validated pen description. Width is measured in canvas units.
type Style struct {
	Width      float64
	Cap        string // butt, round, square
	Join       string // miter, bevel, round
	MiterLimit float64
	Dash       []float64 // alternating on/off canvas-unit lengths
	DashOffset float64
}

type builder struct {
	style Style
	half  float64
	rings []geom.Contour
}

// Expand turns every open polyline into zero or more closed fill rings. A
// polyline with zero drawable length produces nothing instead of a degenerate
// ring, keeping every output contour bounded and rasterizable.
func Expand(polylines []geom.Contour, m geom.Matrix, s Style) geom.Path {
	b := &builder{style: s, half: s.Width / 2}
	for _, line := range polylines {
		pts := make(geom.Contour, 0, len(line))
		for _, p := range line {
			pts = append(pts, m.Apply(p))
		}
		pts = dropRepeats(pts)
		if len(pts) < 2 {
			continue
		}
		if len(s.Dash) == 0 {
			b.polyline(pts, s.Cap, s.Cap)
			continue
		}
		for _, piece := range dashPieces(pts, s.Dash, s.DashOffset) {
			// Internal dash boundaries are always butt; the requested cap only
			// applies to the true path endpoints.
			startCap, endCap := "butt", "butt"
			if piece.a == 0 {
				startCap = s.Cap
			}
			if piece.b >= totalLength(pts)-epsilon {
				endCap = s.Cap
			}
			b.polyline(piece.pts, startCap, endCap)
		}
	}
	return b.rings
}

// VertexEstimate bounds the expansion cost before it is computed, sharing the
// same tessellation contract so admission and execution cannot disagree. Each
// dash piece emits one quad per traversed segment plus bounded join/cap
// vertices; the formula mirrors what Expand actually allocates.
func VertexEstimate(polylineVertices int, polylineLength float64, s Style) int {
	if polylineVertices < 2 {
		return 0
	}
	segments := polylineVertices - 1
	joinCost := 3
	if s.Join == "round" {
		joinCost = roundSegments + 2
	}
	capCost := 0
	if s.Cap == "round" {
		capCost = roundSegments + 2
	} else if s.Cap == "square" {
		capCost = 4
	}
	pieces := 1
	if len(s.Dash) > 0 {
		period := 0.0
		for _, v := range s.Dash {
			period += v
		}
		// Number of on-pieces over the span; +2 covers the two boundary pieces.
		pieces = int(polylineLength/period+0.5) + 2
		if pieces < 1 {
			pieces = 1
		}
	}
	// Every piece contributes four quad vertices per segment it crosses; joins
	// and caps are bounded per piece. This stays at or above the true allocation.
	return pieces*(4+capCost) + segments*4 + segments*joinCost
}

func dropRepeats(pts geom.Contour) geom.Contour {
	out := geom.Contour{pts[0]}
	for _, p := range pts[1:] {
		last := out[len(out)-1]
		if math.Abs(p.X-last.X) > epsilon || math.Abs(p.Y-last.Y) > epsilon {
			out = append(out, p)
		}
	}
	return out
}

func totalLength(pts geom.Contour) float64 {
	total := 0.0
	for i := 1; i < len(pts); i++ {
		total += dist(pts[i-1], pts[i])
	}
	return total
}

func dist(a, b geom.Point) float64 {
	return math.Hypot(b.X-a.X, b.Y-a.Y)
}

func unit(a, b geom.Point) (float64, float64) {
	d := dist(a, b)
	return (b.X - a.X) / d, (b.Y - a.Y) / d
}

// polyline emits one stroked body. capStart/capEnd name the cap at each end;
// dash cuts pass "butt" for boundaries that are not true path endpoints.
func (b *builder) polyline(pts geom.Contour, capStart, capEnd string) {
	if len(pts) < 2 {
		return
	}
	for i := 1; i < len(pts); i++ {
		tx, ty := unit(pts[i-1], pts[i])
		nx, ny := -ty, tx
		h := b.half
		a, c := pts[i-1], pts[i]
		b.emit(geom.Contour{
			{X: a.X + nx*h, Y: a.Y + ny*h},
			{X: c.X + nx*h, Y: c.Y + ny*h},
			{X: c.X - nx*h, Y: c.Y - ny*h},
			{X: a.X - nx*h, Y: a.Y - ny*h},
		})
	}
	for i := 1; i < len(pts)-1; i++ {
		b.join(pts[i-1], pts[i], pts[i+1])
	}
	sx, sy := negUnit(unit(pts[0], pts[1]))
	b.cap(pts[0], sx, sy, capStart)
	last := len(pts) - 1
	tx, ty := unit(pts[last-1], pts[last])
	b.cap(pts[last], tx, ty, capEnd)
}

func negUnit(x, y float64) (float64, float64) { return -x, -y }

// join fills the outer wedge at p. The inner wedge is already covered by the
// adjacent segment quads, so only the bounded outer side is constructed.
func (b *builder) join(a, p, c geom.Point) {
	t1x, t1y := unit(a, p)
	t2x, t2y := unit(p, c)
	cross := t1x*t2y - t1y*t2x
	if math.Abs(cross) <= epsilon {
		return
	}
	n1x, n1y := -t1y, t1x
	n2x, n2y := -t2y, t2x
	h := b.half
	var outerPrev, outerNext geom.Point
	if cross > 0 {
		// Left turn: the right-hand offset is the outer side.
		outerPrev = geom.Point{X: p.X - n1x*h, Y: p.Y - n1y*h}
		outerNext = geom.Point{X: p.X - n2x*h, Y: p.Y - n2y*h}
	} else {
		outerPrev = geom.Point{X: p.X + n1x*h, Y: p.Y + n1y*h}
		outerNext = geom.Point{X: p.X + n2x*h, Y: p.Y + n2y*h}
	}
	switch b.style.Join {
	case "bevel":
		b.emit(geom.Contour{p, outerPrev, outerNext})
	case "round":
		ring := geom.Contour{p, outerPrev}
		a1 := math.Atan2(outerPrev.Y-p.Y, outerPrev.X-p.X)
		a2 := math.Atan2(outerNext.Y-p.Y, outerNext.X-p.X)
		for a, step := a1, sweep(a1, a2, cross); ; a += step {
			ring = append(ring, geom.Point{X: p.X + h*math.Cos(a), Y: p.Y + h*math.Sin(a)})
			if len(ring) >= roundSegments+2 {
				break
			}
		}
		ring = append(ring, outerNext)
		b.emit(ring)
	default: // miter, with a hard fallback so a spike can never run away
		miter, ok := intersect(
			outerPrev, geom.Point{X: outerPrev.X + t1x, Y: outerPrev.Y + t1y},
			outerNext, geom.Point{X: outerNext.X + t2x, Y: outerNext.Y + t2y},
		)
		if !ok || dist(p, miter)/h > b.style.MiterLimit {
			b.emit(geom.Contour{p, outerPrev, outerNext})
			return
		}
		b.emit(geom.Contour{p, outerPrev, miter, outerNext})
	}
}

// sweep returns the per-step angle change along the outer side of the turn.
func sweep(a1, a2 float64, cross float64) float64 {
	d := a2 - a1
	if cross > 0 {
		for d <= 0 {
			d += 2 * math.Pi
		}
	} else {
		for d >= 0 {
			d -= 2 * math.Pi
		}
	}
	return d / roundSegments
}

// cap constructs the ending at p, where tx,ty is the outward travel direction.
// butt needs no geometry because the segment quad already terminates at p.
func (b *builder) cap(p geom.Point, tx, ty float64, kind string) {
	nx, ny := -ty, tx
	h := b.half
	l := geom.Point{X: p.X + nx*h, Y: p.Y + ny*h}
	r := geom.Point{X: p.X - nx*h, Y: p.Y - ny*h}
	switch kind {
	case "square":
		e := geom.Point{X: p.X + tx*h, Y: p.Y + ty*h}
		b.emit(geom.Contour{
			p, l,
			{X: e.X + nx*h, Y: e.Y + ny*h},
			{X: e.X - nx*h, Y: e.Y - ny*h},
			r,
		})
	case "round":
		ring := geom.Contour{p, l}
		a1 := math.Atan2(l.Y-p.Y, l.X-p.X)
		a2 := math.Atan2(r.Y-p.Y, r.X-p.X)
		// Sweep the outward half-circle from the left offset to the right offset.
		d := a1 - a2
		for d <= 0 {
			d += 2 * math.Pi
		}
		step := d / roundSegments
		for a := a1 - step; len(ring) < roundSegments+1; a -= step {
			ring = append(ring, geom.Point{X: p.X + h*math.Cos(a), Y: p.Y + h*math.Sin(a)})
		}
		ring = append(ring, r)
		b.emit(ring)
	}
}

// intersect returns the crossing of lines p1->p2 and p3->p4.
func intersect(p1, p2, p3, p4 geom.Point) (geom.Point, bool) {
	dx1, dy1 := p2.X-p1.X, p2.Y-p1.Y
	dx2, dy2 := p4.X-p3.X, p4.Y-p3.Y
	den := dx1*dy2 - dy1*dx2
	if math.Abs(den) < epsilon {
		return geom.Point{}, false
	}
	t := ((p3.X-p1.X)*dy2 - (p3.Y-p1.Y)*dx2) / den
	return geom.Point{X: p1.X + t*dx1, Y: p1.Y + t*dy1}, true
}

// emit stores a ring, normalizing to CCW so overlapping pieces union under the
// nonzero fill rule instead of cancelling each other's winding.
func (b *builder) emit(ring geom.Contour) {
	if len(ring) < 3 {
		return
	}
	area := 0.0
	for i, v := range ring {
		w := ring[(i+1)%len(ring)]
		area += v.X*w.Y - w.X*v.Y
	}
	if area < 0 {
		for i, j := 0, len(ring)-1; i < j; i, j = i+1, j-1 {
			ring[i], ring[j] = ring[j], ring[i]
		}
	}
	b.rings = append(b.rings, ring)
}

type dashPiece struct {
	a, b float64
	pts  geom.Contour
}

// dashPieces cuts one polyline by a global arc-length pattern. Phase is carried
// continuously across every curve joint, so a dash never restarts where two
// path verbs meet.
func dashPieces(pts geom.Contour, dash []float64, offset float64) []dashPiece {
	length := totalLength(pts)
	period := 0.0
	for _, v := range dash {
		period += v
	}
	type interval struct{ a, b float64 }
	var intervals []interval
	idx := 0
	pos := mod(offset, period)
	for pos >= dash[idx] {
		pos -= dash[idx]
		idx = (idx + 1) % len(dash)
	}
	on := idx%2 == 0
	s := 0.0
	for s < length-epsilon {
		remaining := dash[idx] - pos
		take := math.Min(remaining, length-s)
		if on && take > epsilon {
			intervals = append(intervals, interval{s, s + take})
		}
		s += take
		if take >= remaining-epsilon {
			idx = (idx + 1) % len(dash)
			pos = 0
			on = !on
		} else {
			pos += take
		}
	}
	pieces := make([]dashPiece, 0, len(intervals))
	for _, in := range intervals {
		pieces = append(pieces, dashPiece{a: in.a, b: in.b, pts: slicePolyline(pts, in.a, in.b)})
	}
	return pieces
}

// slicePolyline returns the sub-polyline spanning arc-length distances [a,b].
func slicePolyline(pts geom.Contour, a, b float64) geom.Contour {
	cum := make([]float64, len(pts))
	for i := 1; i < len(pts); i++ {
		cum[i] = cum[i-1] + dist(pts[i-1], pts[i])
	}
	pointAt := func(s float64) geom.Point {
		if s <= 0 {
			return pts[0]
		}
		if s >= cum[len(cum)-1] {
			return pts[len(pts)-1]
		}
		i := 1
		for cum[i] < s {
			i++
		}
		prev := cum[i-1]
		u := (s - prev) / (cum[i] - prev)
		return geom.Point{
			X: pts[i-1].X + u*(pts[i].X-pts[i-1].X),
			Y: pts[i-1].Y + u*(pts[i].Y-pts[i-1].Y),
		}
	}
	out := geom.Contour{pointAt(a)}
	for i := 1; i < len(pts); i++ {
		if cum[i] > a+epsilon && cum[i] < b-epsilon {
			out = append(out, pts[i])
		}
	}
	out = append(out, pointAt(b))
	return dropRepeats(out)
}

func mod(x, m float64) float64 {
	r := math.Mod(x, m)
	if r < 0 {
		r += m
	}
	return r
}
