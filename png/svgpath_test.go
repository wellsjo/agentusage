package png

import (
	"fmt"
	"image"
	"math"
	"testing"

	"golang.org/x/image/vector"
)

func ops(path svgPath) []pathOp {
	result := make([]pathOp, len(path))
	for i, segment := range path {
		result[i] = segment.op
	}
	return result
}

func equalOps(a, b []pathOp) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestParsePathCommands(t *testing.T) {
	path, err := parsePath("M0 0 L10 0 l0 10 H0 V0 Q5 5 0 0 T1 1 C1 1 2 2 3 3 S4 4 5 5 Z")
	if err != nil {
		t.Fatal(err)
	}
	want := []pathOp{opMove, opLine, opLine, opLine, opLine, opQuad, opQuad, opCube, opCube, opClose}
	if got := ops(path); !equalOps(got, want) {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	if path[2].p[0] != (point{10, 10}) {
		t.Errorf("relative line end = %v, want (10,10)", path[2].p[0])
	}
	if path[3].p[0] != (point{0, 10}) || path[4].p[0] != (point{0, 0}) {
		t.Errorf("H and V ends = %v, %v", path[3].p[0], path[4].p[0])
	}
	// T reflects the previous quadratic control point (5,5) around (0,0).
	if path[6].p[0] != (point{-5, -5}) {
		t.Errorf("T control = %v, want (-5,-5)", path[6].p[0])
	}
	// S reflects the previous cubic control point (2,2) around (3,3).
	if path[8].p[0] != (point{4, 4}) {
		t.Errorf("S control = %v, want (4,4)", path[8].p[0])
	}
}

func TestParsePathImplicitCommandsAndCompactNumbers(t *testing.T) {
	path, err := parsePath("m1 2 3 4 5 6M.5-.5.25.25a1 1 0 011 1z")
	if err != nil {
		t.Fatal(err)
	}
	want := []pathOp{opMove, opLine, opLine, opClose, opMove, opLine, opCube, opClose}
	if got := ops(path); !equalOps(got, want) {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	if path[1].p[0] != (point{4, 6}) || path[2].p[0] != (point{9, 12}) {
		t.Errorf("implicit relative lines end at %v and %v", path[1].p[0], path[2].p[0])
	}
	if path[4].p[0] != (point{0.5, -0.5}) || path[5].p[0] != (point{0.25, 0.25}) {
		t.Errorf("compact numbers parsed as %v and %v", path[4].p[0], path[5].p[0])
	}
	if end := path[6].p[2]; end != (point{1.25, 1.25}) {
		t.Errorf("arc end = %v, want (1.25,1.25)", end)
	}
}

func TestParsePathClosesOpenSubpaths(t *testing.T) {
	path, err := parsePath("M0 0 L1 0 L1 1 M5 5 L6 6")
	if err != nil {
		t.Fatal(err)
	}
	want := []pathOp{opMove, opLine, opLine, opClose, opMove, opLine, opClose}
	if got := ops(path); !equalOps(got, want) {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	// A drawing command after Z starts a new subpath at the closed subpath's
	// first point.
	path, err = parsePath("M2 2 L3 3 Z l1 0")
	if err != nil {
		t.Fatal(err)
	}
	want = []pathOp{opMove, opLine, opClose, opMove, opLine, opClose}
	if got := ops(path); !equalOps(got, want) {
		t.Fatalf("ops = %v, want %v", got, want)
	}
	if path[3].p[0] != (point{2, 2}) || path[4].p[0] != (point{3, 2}) {
		t.Errorf("subpath after Z runs from %v to %v", path[3].p[0], path[4].p[0])
	}
}

func TestParsePathErrors(t *testing.T) {
	for _, d := range []string{"10 10", "M0 0 Z 5", "M0 0 A1 1 0 2 0 5 5", "M0 0 L1", "M0 0 X1 1", "M1e 2", "M- 1", "M. 1"} {
		if _, err := parsePath(d); err == nil {
			t.Errorf("parsePath(%q) accepted bad data", d)
		}
	}
	if path, err := parsePath(""); err != nil || len(path) != 0 {
		t.Errorf("parsePath(\"\") = (%v, %v), want an empty path", path, err)
	}
}

// coverage rasterizes the path into a size×size alpha mask and returns the
// covered area in pixels.
func coverage(path svgPath, size int) float64 {
	z := vector.NewRasterizer(size, size)
	path.rasterize(z, func(p point) (float32, float32) { return float32(p.x), float32(p.y) })
	mask := image.NewAlpha(image.Rect(0, 0, size, size))
	z.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	total := 0.0
	for _, alpha := range mask.Pix {
		total += float64(alpha) / 255
	}
	return total
}

// rotatedEllipse is an ellipse with radii 40 and 30, rotated 45 degrees
// around (50,50), drawn from one end of its major axis to the other and
// back, so both arcs share one center.
var rotatedEllipse = fmt.Sprintf("M%.6f %.6f A40 30 45 1 0 %.6f %.6f A40 30 45 1 0 %.6f %.6f Z",
	50-40*math.Cos(math.Pi/4), 50-40*math.Sin(math.Pi/4),
	50+40*math.Cos(math.Pi/4), 50+40*math.Sin(math.Pi/4),
	50-40*math.Cos(math.Pi/4), 50-40*math.Sin(math.Pi/4))

func TestArcsApproximateAnEllipse(t *testing.T) {
	cases := map[string]struct {
		d    string
		area float64
	}{
		"circle":         {"M50 0 A50 50 0 1 1 50 100 A50 50 0 1 1 50 0 Z", math.Pi * 50 * 50},
		"ellipse":        {"M50 10 A40 30 0 1 0 50 70 A40 30 0 1 0 50 10 Z", math.Pi * 40 * 30},
		"rotated":        {"M50 10 A40 30 90 1 0 50 90 A40 30 90 1 0 50 10 Z", math.Pi * 40 * 30},
		"rotated-45":     {rotatedEllipse, math.Pi * 40 * 30},
		"small-arc-flag": {"M10 50 A40 40 0 0 1 90 50 L90 100 L10 100 Z", math.Pi*40*40/2 + 80*50},
		"scaled-radii":   {"M0 50 A1 1 0 0 1 100 50 A1 1 0 0 1 0 50 Z", math.Pi * 50 * 50},
	}
	for name, tc := range cases {
		path, err := parsePath(tc.d)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := coverage(path, 100)
		if math.Abs(got-tc.area)/tc.area > 0.01 {
			t.Errorf("%s: covered %.1f px, want %.1f", name, got, tc.area)
		}
	}
}

func TestIconsRasterize(t *testing.T) {
	for id, icon := range icons {
		if len(icon) == 0 {
			t.Errorf("%s icon is empty", id)
		}
		got := coverage(icon, 100)
		if got < 500 || got > 5000 {
			t.Errorf("%s icon covers %.0f of 10000 px, which does not look like a glyph", id, got)
		}
	}
}
