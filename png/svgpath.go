package png

import (
	"fmt"
	"math"
	"strconv"

	"github.com/wellsjo/agentusage"
	"golang.org/x/image/vector"
)

// Provider icons, verbatim from the Web Component's ICON_PATHS. Each path
// sits in a 100x100 viewBox. TestIconPathsMatchWebComponent keeps them in
// sync.
const (
	codexIconPath  = "M83.773 42.809a20.28 20.28 0 0 0-23.418-26.036A20.3 20.3 0 0 0 26.102 24.01a20.28 20.28 0 0 0-10.85 33.323 20.28 20.28 0 0 0 23.419 26.036A20.3 20.3 0 0 0 72.94 76.052a20.28 20.28 0 0 0 10.833-33.243ZM53.7 84.836a14.93 14.93 0 0 1-9.588-3.47l16.4-9.462a2.63 2.63 0 0 0 1.31-2.271V47.177l6.733 3.895a.24.24 0 0 1 .126.174v18.608A15.02 15.02 0 0 1 53.7 84.836ZM21.498 71.084a14.93 14.93 0 0 1-1.782-10.045l16.416 9.477a2.6 2.6 0 0 0 2.602 0L58.21 59.288v7.775a.24.24 0 0 1-.11.205l-16.133 9.304a15.02 15.02 0 0 1-20.47-5.488ZM17.303 36.39a15.02 15.02 0 0 1 7.885-6.576v18.924a2.6 2.6 0 0 0 1.293 2.255l19.381 11.181-6.734 3.895a.24.24 0 0 1-.236 0l-16.101-9.288a15.02 15.02 0 0 1-5.488-20.47v.079Zm55.321 12.853L53.18 37.951l6.718-3.88a.24.24 0 0 1 .237 0l16.1 9.305a15.02 15.02 0 0 1-2.255 27.014V51.466a2.6 2.6 0 0 0-1.356-2.223Zm6.702-10.077-16.385-9.557a2.6 2.6 0 0 0-2.618 0L40.863 40.837v-7.774a.24.24 0 0 1 .095-.205l16.1-9.289a15.02 15.02 0 0 1 22.268 15.596ZM37.189 52.948l-6.734-3.879a.24.24 0 0 1-.126-.19V30.319a15.02 15.02 0 0 1 24.112-11.244l-15.928 9.194a2.63 2.63 0 0 0-1.309 2.27l-.015 22.41Zm3.658-7.885 8.674-4.999 8.689 5v9.997l-8.658 5-8.689-5-.016-9.998Z"
	claudeIconPath = "m25.715 63.215 15.724-8.823.264-.77-.264-.424h-.768l-28.86-1.539-1.78-2.347.181-1.174 1.6-1.072 29.595 2.246h.296l.182-.526-.79-.648-24.427-17.364-.526-3.36 2.186-2.408 2.934.203 19.59 14.53.486-.344.06-.243-12.587-22.868-.344-2.429 2.49-3.38 1.375-.445 3.32.446 1.396 1.214 12.912 28.01h.83v-.486l2.247-23.698 1.254-3.036 2.49-1.64 1.942.931 1.599 2.287-4.25 23.8h.708l15.28-19.226h3.44l2.53 3.764-1.133 3.886-13.317 18.497.243.364.627-.06 23.557-4.007 2.773 1.296.303 1.315-1.093 2.692-25.844 5.97-.142.1.162.203 22.828 1.356 2.631 1.74 1.579 2.126-.263 1.618-4.048 2.065-23.193-5.424h-.607v.364L81.146 74.124l.425 1.922-1.073 1.518-1.133-.162-16.595-13.418h-.425v.567l9.288 13.903.405 3.603-.566 1.174-2.023.708-2.227-.405-13.094-20.116-.465.263-2.247 24.184-1.052 1.235-2.428.93-2.024-1.538-1.073-2.489 4.938-24.872-.04-.142-.466.061-17.79 22.525-1.375.546-2.388-1.235.222-2.206 17.182-21.978-.02-.526h-.182L23.792 71.898l-3.765.485-1.619-1.517.203-2.49 7.104-5.16Z"
)

const iconViewBox = 100.0

var icons = map[string]svgPath{
	agentusage.ProviderIDCodex:  mustParsePath(codexIconPath),
	agentusage.ProviderIDClaude: mustParsePath(claudeIconPath),
}

type point struct{ x, y float64 }

type pathOp uint8

const (
	opMove pathOp = iota
	opLine
	opQuad
	opCube
	opClose
)

// pathSegment is one drawing instruction. opMove and opLine use p[0]; opQuad
// uses p[0] as the control point and p[1] as the end; opCube uses p[0] and
// p[1] as control points and p[2] as the end.
type pathSegment struct {
	op pathOp
	p  [3]point
}

// svgPath is a parsed SVG path: only moves, lines, curves, and closes, with
// every subpath closed, so a rasterizer can fill it directly.
type svgPath []pathSegment

func mustParsePath(d string) svgPath {
	path, err := parsePath(d)
	if err != nil {
		panic(err)
	}
	return path
}

// rasterize feeds the path to z after mapping every point with transform.
func (path svgPath) rasterize(z *vector.Rasterizer, transform func(point) (float32, float32)) {
	for _, segment := range path {
		switch segment.op {
		case opMove:
			z.MoveTo(transform(segment.p[0]))
		case opLine:
			z.LineTo(transform(segment.p[0]))
		case opQuad:
			bx, by := transform(segment.p[0])
			cx, cy := transform(segment.p[1])
			z.QuadTo(bx, by, cx, cy)
		case opCube:
			bx, by := transform(segment.p[0])
			cx, cy := transform(segment.p[1])
			dx, dy := transform(segment.p[2])
			z.CubeTo(bx, by, cx, cy, dx, dy)
		case opClose:
			z.ClosePath()
		}
	}
}

// parsePath parses SVG path data (the d attribute). Arcs become cubic
// Bézier curves.
func parsePath(d string) (svgPath, error) {
	parser := &pathParser{d: d}
	var command byte
	for {
		parser.skipSeparators()
		if parser.i >= len(parser.d) {
			break
		}
		if next := parser.d[parser.i]; isLetter(next) {
			command = next
			parser.i++
		} else {
			switch command {
			case 0:
				return nil, fmt.Errorf("svg path: expected a command at offset %d", parser.i)
			case 'M':
				command = 'L'
			case 'm':
				command = 'l'
			case 'Z', 'z':
				return nil, fmt.Errorf("svg path: unexpected number after Z at offset %d", parser.i)
			}
		}
		if err := parser.command(command); err != nil {
			return nil, err
		}
		parser.last = command
	}
	parser.finish()
	return parser.path, nil
}

type pathParser struct {
	d    string
	i    int
	err  error
	path svgPath
	// cur is the current point, start the current subpath's first point, and
	// ctrl the last control point for S and T reflection.
	cur, start, ctrl point
	open             bool
	last             byte
}

func (p *pathParser) command(command byte) error {
	relative := command >= 'a' && command <= 'z'
	switch command {
	case 'M', 'm':
		p.moveTo(p.point(relative))
	case 'L', 'l':
		p.lineTo(p.point(relative))
	case 'H', 'h':
		x := p.number()
		if relative {
			x += p.cur.x
		}
		p.lineTo(point{x, p.cur.y})
	case 'V', 'v':
		y := p.number()
		if relative {
			y += p.cur.y
		}
		p.lineTo(point{p.cur.x, y})
	case 'C', 'c':
		control1 := p.point(relative)
		control2 := p.point(relative)
		p.cubeTo(control1, control2, p.point(relative))
	case 'S', 's':
		control1 := p.cur
		if p.last == 'C' || p.last == 'c' || p.last == 'S' || p.last == 's' {
			control1 = reflect(p.cur, p.ctrl)
		}
		control2 := p.point(relative)
		p.cubeTo(control1, control2, p.point(relative))
	case 'Q', 'q':
		control := p.point(relative)
		p.quadTo(control, p.point(relative))
	case 'T', 't':
		control := p.cur
		if p.last == 'Q' || p.last == 'q' || p.last == 'T' || p.last == 't' {
			control = reflect(p.cur, p.ctrl)
		}
		p.quadTo(control, p.point(relative))
	case 'A', 'a':
		rx, ry, rotation := p.number(), p.number(), p.number()
		large, sweep := p.flag(), p.flag()
		p.arcTo(rx, ry, rotation, large, sweep, p.point(relative))
	case 'Z', 'z':
		p.closePath()
	default:
		return fmt.Errorf("svg path: unknown command %q at offset %d", command, p.i-1)
	}
	return p.err
}

func (p *pathParser) skipSeparators() {
	for p.i < len(p.d) && (isSpace(p.d[p.i]) || p.d[p.i] == ',') {
		p.i++
	}
}

func (p *pathParser) number() float64 {
	if p.err != nil {
		return 0
	}
	p.skipSeparators()
	start := p.i
	if p.i < len(p.d) && (p.d[p.i] == '+' || p.d[p.i] == '-') {
		p.i++
	}
	digits := p.digits()
	if p.i < len(p.d) && p.d[p.i] == '.' {
		p.i++
		digits += p.digits()
	}
	if digits == 0 {
		p.err = fmt.Errorf("svg path: expected a number at offset %d", start)
		return 0
	}
	if p.i < len(p.d) && (p.d[p.i] == 'e' || p.d[p.i] == 'E') {
		p.i++
		if p.i < len(p.d) && (p.d[p.i] == '+' || p.d[p.i] == '-') {
			p.i++
		}
		if p.digits() == 0 {
			p.err = fmt.Errorf("svg path: bad exponent at offset %d", start)
			return 0
		}
	}
	value, err := strconv.ParseFloat(p.d[start:p.i], 64)
	if err != nil {
		p.err = fmt.Errorf("svg path: bad number %q at offset %d", p.d[start:p.i], start)
		return 0
	}
	return value
}

func (p *pathParser) digits() int {
	count := 0
	for p.i < len(p.d) && p.d[p.i] >= '0' && p.d[p.i] <= '9' {
		p.i++
		count++
	}
	return count
}

// flag reads an arc flag. Flags are single characters and may touch the
// next number, as in "0 01-9.5".
func (p *pathParser) flag() bool {
	if p.err != nil {
		return false
	}
	p.skipSeparators()
	if p.i < len(p.d) {
		switch p.d[p.i] {
		case '0':
			p.i++
			return false
		case '1':
			p.i++
			return true
		}
	}
	p.err = fmt.Errorf("svg path: expected an arc flag at offset %d", p.i)
	return false
}

func (p *pathParser) point(relative bool) point {
	x := p.number()
	y := p.number()
	if relative {
		x += p.cur.x
		y += p.cur.y
	}
	return point{x, y}
}

func (p *pathParser) moveTo(to point) {
	p.finish()
	p.path = append(p.path, pathSegment{op: opMove, p: [3]point{to}})
	p.cur, p.start, p.ctrl = to, to, to
	p.open = true
}

// ensureOpen starts a subpath at the current point for a drawing command
// that follows a close.
func (p *pathParser) ensureOpen() {
	if !p.open {
		p.moveTo(p.cur)
	}
}

func (p *pathParser) lineTo(to point) {
	p.ensureOpen()
	p.path = append(p.path, pathSegment{op: opLine, p: [3]point{to}})
	p.cur, p.ctrl = to, to
}

func (p *pathParser) quadTo(control, to point) {
	p.ensureOpen()
	p.path = append(p.path, pathSegment{op: opQuad, p: [3]point{control, to}})
	p.cur, p.ctrl = to, control
}

func (p *pathParser) cubeTo(control1, control2, to point) {
	p.ensureOpen()
	p.path = append(p.path, pathSegment{op: opCube, p: [3]point{control1, control2, to}})
	p.cur, p.ctrl = to, control2
}

func (p *pathParser) arcTo(rx, ry, rotation float64, large, sweep bool, to point) {
	p.ensureOpen()
	if p.cur == to {
		return
	}
	if rx == 0 || ry == 0 {
		p.lineTo(to)
		return
	}
	for _, cube := range arcCubics(p.cur, rx, ry, rotation, large, sweep, to) {
		p.path = append(p.path, pathSegment{op: opCube, p: cube})
	}
	p.cur, p.ctrl = to, to
}

func (p *pathParser) closePath() {
	p.finish()
	p.cur, p.ctrl = p.start, p.start
}

// finish closes an open subpath, so a fill treats it as closed like SVG does.
func (p *pathParser) finish() {
	if p.open {
		p.path = append(p.path, pathSegment{op: opClose})
		p.open = false
	}
}

func reflect(center, control point) point {
	return point{2*center.x - control.x, 2*center.y - control.y}
}

// arcCubics converts an SVG elliptical arc to cubic Bézier curves. The
// center parameterization follows the SVG implementation notes (F.6.5);
// each curve spans at most a quarter turn.
func arcCubics(from point, rx, ry, rotation float64, large, sweep bool, to point) [][3]point {
	rx, ry = math.Abs(rx), math.Abs(ry)
	phi := rotation * math.Pi / 180
	cosPhi, sinPhi := math.Cos(phi), math.Sin(phi)

	dx, dy := (from.x-to.x)/2, (from.y-to.y)/2
	x1 := cosPhi*dx + sinPhi*dy
	y1 := -sinPhi*dx + cosPhi*dy
	if lambda := x1*x1/(rx*rx) + y1*y1/(ry*ry); lambda > 1 {
		scale := math.Sqrt(lambda)
		rx *= scale
		ry *= scale
	}
	rx2, ry2 := rx*rx, ry*ry
	numerator := rx2*ry2 - rx2*y1*y1 - ry2*x1*x1
	denominator := rx2*y1*y1 + ry2*x1*x1
	coefficient := 0.0
	if denominator != 0 {
		coefficient = math.Sqrt(math.Max(0, numerator/denominator))
	}
	if large == sweep {
		coefficient = -coefficient
	}
	cx1 := coefficient * rx * y1 / ry
	cy1 := -coefficient * ry * x1 / rx
	cx := cosPhi*cx1 - sinPhi*cy1 + (from.x+to.x)/2
	cy := sinPhi*cx1 + cosPhi*cy1 + (from.y+to.y)/2

	ux, uy := (x1-cx1)/rx, (y1-cy1)/ry
	vx, vy := (-x1-cx1)/rx, (-y1-cy1)/ry
	theta := vectorAngle(1, 0, ux, uy)
	delta := vectorAngle(ux, uy, vx, vy)
	if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	} else if sweep && delta < 0 {
		delta += 2 * math.Pi
	}

	count := max(1, int(math.Ceil(math.Abs(delta)/(math.Pi/2)-1e-9)))
	step := delta / float64(count)
	tangent := 4.0 / 3.0 * math.Tan(step/4)
	at := func(angle float64) point {
		cos, sin := math.Cos(angle), math.Sin(angle)
		return point{cx + rx*cosPhi*cos - ry*sinPhi*sin, cy + rx*sinPhi*cos + ry*cosPhi*sin}
	}
	derivative := func(angle float64) point {
		cos, sin := math.Cos(angle), math.Sin(angle)
		return point{-rx*cosPhi*sin - ry*sinPhi*cos, -rx*sinPhi*sin + ry*cosPhi*cos}
	}
	cubes := make([][3]point, 0, count)
	for i := 0; i < count; i++ {
		a, b := theta, theta+step
		pa, pb := at(a), at(b)
		da, db := derivative(a), derivative(b)
		end := pb
		if i == count-1 {
			end = to
		}
		cubes = append(cubes, [3]point{
			{pa.x + tangent*da.x, pa.y + tangent*da.y},
			{pb.x - tangent*db.x, pb.y - tangent*db.y},
			end,
		})
		theta = b
	}
	return cubes
}

// vectorAngle returns the signed angle from vector u to vector v.
func vectorAngle(ux, uy, vx, vy float64) float64 {
	length := math.Hypot(ux, uy) * math.Hypot(vx, vy)
	if length == 0 {
		return 0
	}
	angle := math.Acos(math.Max(-1, math.Min(1, (ux*vx+uy*vy)/length)))
	if ux*vy-uy*vx < 0 {
		return -angle
	}
	return angle
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}
