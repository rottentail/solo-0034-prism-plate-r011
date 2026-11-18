// Package validate owns structural admission and the shared compilation budget.
package validate

import (
	"fmt"
	"math"
	"prismplate/internal/geom"
	"prismplate/internal/scene"
	"prismplate/internal/stroke"
	"regexp"
)

var identifier = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)

func ID(s string) bool         { return identifier.MatchString(s) }
func finite(x float64) bool    { return !math.IsNaN(x) && !math.IsInf(x, 0) && math.Abs(x) <= 1e6 }
func Unit(x float64) bool      { return finite(x) && x >= 0 && x <= 1 }
func Color(c scene.Color) bool { return Unit(c.R) && Unit(c.G) && Unit(c.B) && Unit(c.A) }
func path(ops []geom.Op) (geom.Path, error) {
	closed, open, err := paths(ops, false)
	if err != nil {
		return nil, err
	}
	if len(open) != 0 {
		return nil, fmt.Errorf("contour must terminate with Z")
	}
	return closed, nil
}

// paths admits closed fill contours and, when openOK, open stroke polylines.
// Multiple M-separated contours may mix the two within one shape path.
func paths(ops []geom.Op, openOK bool) (geom.Path, geom.Path, error) {
	if len(ops) == 0 || len(ops) > 256 {
		return nil, nil, fmt.Errorf("contour operation count must be 1..256")
	}
	open := false
	vertices := 0
	for _, o := range ops {
		n, ok := map[string]int{"M": 1, "L": 1, "Q": 2, "C": 3, "Z": 0}[o.Kind]
		if !ok || len(o.Points) != n {
			return nil, nil, fmt.Errorf("invalid contour verb or arity")
		}
		for _, p := range o.Points {
			if !finite(p.X) || !finite(p.Y) {
				return nil, nil, fmt.Errorf("non-finite contour")
			}
		}
		if o.Kind == "M" {
			if open {
				if !openOK {
					return nil, nil, fmt.Errorf("close each contour before another M")
				}
				if vertices < 2 {
					return nil, nil, fmt.Errorf("open contour requires two vertices")
				}
			}
			open = true
			vertices = 1
		} else {
			if !open {
				return nil, nil, fmt.Errorf("contour must begin with M")
			}
			if o.Kind == "Z" {
				if vertices < 3 {
					return nil, nil, fmt.Errorf("contour requires three vertices")
				}
				open = false
			} else {
				vertices++
			}
		}
	}
	if open && !openOK {
		return nil, nil, fmt.Errorf("contour must terminate with Z")
	}
	if open && vertices < 2 {
		return nil, nil, fmt.Errorf("open contour requires two vertices")
	}
	closed, polylines := geom.FlattenOpen(ops)
	return closed, polylines, nil
}

func strokeSpec(s *scene.Stroke) (stroke.Style, error) {
	if s == nil {
		return stroke.Style{}, nil
	}
	if !finite(s.Width) || s.Width <= 0 || s.Width > 128 {
		return stroke.Style{}, fmt.Errorf("stroke width must be within 0..128 canvas units")
	}
	cap := s.Cap
	if cap == "" {
		cap = "butt"
	}
	join := s.Join
	if join == "" {
		join = "miter"
	}
	if cap != "butt" && cap != "round" && cap != "square" {
		return stroke.Style{}, fmt.Errorf("invalid stroke cap")
	}
	if join != "miter" && join != "bevel" && join != "round" {
		return stroke.Style{}, fmt.Errorf("invalid stroke join")
	}
	miter := s.MiterLimit
	if miter == 0 {
		miter = 4
	}
	if !finite(miter) || miter < 1 || miter > 32 {
		return stroke.Style{}, fmt.Errorf("miter limit must be within 1..32")
	}
	if len(s.Dash) > 16 || len(s.Dash)%2 == 1 {
		return stroke.Style{}, fmt.Errorf("dash pattern needs an even count of at most 16")
	}
	period := 0.0
	for _, v := range s.Dash {
		if !finite(v) || v < 0 || v > 4096 {
			return stroke.Style{}, fmt.Errorf("invalid dash length")
		}
		period += v
	}
	if len(s.Dash) > 0 && period <= 0 {
		return stroke.Style{}, fmt.Errorf("dash pattern must contain a positive on length")
	}
	if len(s.Dash) > 0 && period < 0.5 {
		return stroke.Style{}, fmt.Errorf("dash pattern period too small")
	}
	if !finite(s.DashOffset) || math.Abs(s.DashOffset) > 1e6 {
		return stroke.Style{}, fmt.Errorf("invalid dash offset")
	}
	return stroke.Style{Width: s.Width, Cap: cap, Join: join, MiterLimit: miter, Dash: append([]float64(nil), s.Dash...), DashOffset: s.DashOffset}, nil
}
func brush(b scene.Brush) error {
	if b.Kind == "solid" {
		if !Color(b.Color) {
			return fmt.Errorf("invalid solid color")
		}
		return nil
	}
	if b.Kind != "linear" {
		return fmt.Errorf("unknown brush")
	}
	for _, v := range []float64{b.From.X, b.From.Y, b.To.X, b.To.Y} {
		if !finite(v) {
			return fmt.Errorf("invalid gradient axis")
		}
	}
	if math.Hypot(b.To.X-b.From.X, b.To.Y-b.From.Y) < 1e-6 {
		return fmt.Errorf("degenerate gradient axis")
	}
	if len(b.Stops) < 2 || len(b.Stops) > 16 {
		return fmt.Errorf("gradient requires 2..16 stops")
	}
	prev := -1.0
	for _, s := range b.Stops {
		if !Unit(s.At) || s.At <= prev || !Color(s.Color) {
			return fmt.Errorf("invalid gradient stops")
		}
		prev = s.At
	}
	if b.Stops[0].At != 0 || prev != 1 {
		return fmt.Errorf("gradient endpoints must be 0 and 1")
	}
	return nil
}
func Compile(d scene.Document) (scene.Compiled, error) {
	c := scene.Compiled{Document: d}
	if d.Width < 1 || d.Height < 1 || d.Width > 512 || d.Height > 512 || !Color(d.Background) {
		return c, fmt.Errorf("invalid canvas (maximum 512 square)")
	}
	if len(d.Layers) < 1 || len(d.Layers) > 16 {
		return c, fmt.Errorf("requires 1..16 layers")
	}
	ids := map[string]bool{}
	add := func(id string) error {
		if !ID(id) || ids[id] {
			return fmt.Errorf("invalid or duplicate node id %q", id)
		}
		ids[id] = true
		return nil
	}
	count := func(p geom.Path) {
		for _, r := range p {
			c.Segments += len(r)
		}
	}
	for _, l := range d.Layers {
		if err := add(l.ID); err != nil {
			return c, err
		}
		if !Unit(l.Opacity) || (l.Blend != "normal" && l.Blend != "multiply" && l.Blend != "screen") {
			return c, fmt.Errorf("invalid layer compositing")
		}
		if len(l.Shapes) < 1 || len(l.Shapes) > 32 {
			return c, fmt.Errorf("requires 1..32 shapes per layer")
		}
		cl := scene.CompiledLayer{Layer: l}
		if len(l.Clip) > 0 {
			p, e := path(l.Clip)
			if e != nil {
				return c, e
			}
			cl.Clip = p
			count(p)
		}
		if len(l.Mask) > 0 {
			p, e := path(l.Mask)
			if e != nil {
				return c, e
			}
			cl.Mask = p
			count(p)
		}
		for _, s := range l.Shapes {
			if err := add(s.ID); err != nil {
				return c, err
			}
			if s.FillRule != "nonzero" && s.FillRule != "evenodd" {
				return c, fmt.Errorf("invalid fill rule")
			}
			for _, v := range s.Transform {
				if !finite(v) {
					return c, fmt.Errorf("invalid transform")
				}
			}
			inv, ok := s.Transform.Inverse()
			if !ok {
				return c, fmt.Errorf("singular transform")
			}
			for _, v := range inv {
				if !finite(v) {
					return c, fmt.Errorf("ill-conditioned transform")
				}
			}
			style, e := strokeSpec(s.Stroke)
			if e != nil {
				return c, e
			}
			closed, polylines, e := paths(s.Path, s.Open)
			if e != nil {
				return c, e
			}
			if !s.Open && len(polylines) != 0 {
				return c, fmt.Errorf("contour must terminate with Z")
			}
			if s.Open && len(polylines) == 0 && len(closed) == 0 {
				return c, fmt.Errorf("open shape requires a drawable contour")
			}
			if s.Stroke == nil && len(polylines) != 0 {
				return c, fmt.Errorf("open contour requires a stroke")
			}
			if e = brush(s.Brush); e != nil {
				return c, e
			}
			count(closed)
			for _, line := range polylines {
				length := 0.0
				for i := 1; i < len(line); i++ {
					length += math.Hypot(line[i].X-line[i-1].X, line[i].Y-line[i-1].Y)
				}
				if length <= 0 {
					return c, fmt.Errorf("open contour has no drawable length")
				}
				// Stroke expansion uses the same tessellation contract as the
				// stroke compiler so execution cannot exceed the admission estimate.
				c.StrokeVertices += stroke.VertexEstimate(len(line), length, style)
			}
			compiled := scene.CompiledShape{Shape: s, Path: closed, Open: polylines, Inverse: inv}
			if s.Stroke != nil {
				compiled.Strokes = stroke.Expand(polylines, s.Transform, style)
			}
			cl.Shapes = append(cl.Shapes, compiled)
		}
		c.Layers = append(c.Layers, cl)
	}
	// Four samples per pixel; every fill edge and every expanded stroke ring is
	// bounded even under adversarial input. Fills use the historical edge-traversal
	// budget; stroke rings are single point-in-ring tests and get a separate,
	// calibrated vertex budget so dashing cannot grow without limit.
	samples := int64(d.Width) * int64(d.Height) * 4
	if int64(c.Segments)*samples > 160_000_000 {
		return c, fmt.Errorf("scene exceeds bounded fill work")
	}
	if int64(c.StrokeVertices)*samples > 640_000_000 {
		return c, fmt.Errorf("scene exceeds bounded stroke work")
	}
	return c, nil
}
