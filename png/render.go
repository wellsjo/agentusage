package png

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"

	"github.com/wellsjo/agentusage"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Layout constants in CSS pixels, from the Web Component's stylesheet.
const (
	bodySize       = 12.0           // .meta, .status, .empty font-size
	metaLine       = 16.0           // .meta line-height
	statusLine     = bodySize * 1.4 // .status and .empty inherit line-height 1.4
	metaMargin     = 5.0
	headingSize    = 11.0
	headingLine    = 16.0
	headingSpacing = 0.14 * headingSize // letter-spacing: .14em
	headingMargin  = 10.0
	iconSize       = 14.0
	iconGap        = 7.0
	usedMargin     = 8.0
	trackHeight    = 6.0
	trackRadius    = 6.0
	markerWidth    = 1.0
	markerHeight   = 10.0
	markerTop      = -2.0
	markerShift    = -1.0 // transform: translateX(-1px)
	windowGap      = 12.0
	statusMargin   = 8.0
	gridGap        = 30.0
	narrowGap      = 22.0
	narrowWidth    = 540.0 // @container (max-width: 540px)

	emptyText = "No usage providers available."
)

// cachedFont parses an embedded font once. The parsed font is safe for
// concurrent use; each renderer gets its own faces.
type cachedFont struct {
	data []byte
	once sync.Once
	font *opentype.Font
	err  error
}

func (c *cachedFont) get() (*opentype.Font, error) {
	c.once.Do(func() { c.font, c.err = opentype.Parse(c.data) })
	return c.font, c.err
}

var (
	goRegular = &cachedFont{data: goregular.TTF}
	goBold    = &cachedFont{data: gobold.TTF}
)

func loadFont(data []byte, fallback *cachedFont) (*opentype.Font, error) {
	if len(data) == 0 {
		return fallback.get()
	}
	parsed, err := opentype.Parse(bytes.Clone(data))
	if err != nil {
		return nil, fmt.Errorf("agentusage/png: parse font: %w", err)
	}
	return parsed, nil
}

func newFace(f *opentype.Font, size float64) (font.Face, error) {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, fmt.Errorf("agentusage/png: font face: %w", err)
	}
	return face, nil
}

// renderer draws one image. Layout runs in CSS pixels; drawing multiplies by
// the scale and snaps box edges to device pixels like a browser does.
type renderer struct {
	options Options
	scale   float64
	img     *image.RGBA
	rast    *vector.Rasterizer
	body    font.Face
	heading font.Face
}

func newRenderer(options Options) (*renderer, error) {
	bodyFont, err := loadFont(options.Font, goRegular)
	if err != nil {
		return nil, err
	}
	headingFont, err := loadFont(options.HeadingFont, goBold)
	if err != nil {
		return nil, err
	}
	body, err := newFace(bodyFont, bodySize*options.Scale)
	if err != nil {
		return nil, err
	}
	heading, err := newFace(headingFont, headingSize*options.Scale)
	if err != nil {
		return nil, err
	}
	return &renderer{
		options: options,
		scale:   options.Scale,
		rast:    &vector.Rasterizer{},
		body:    body,
		heading: heading,
	}, nil
}

type providerLayout struct {
	view   providerView
	status []string
	height float64
}

func (r *renderer) layoutProvider(view providerView, width float64) providerLayout {
	layout := providerLayout{view: view}
	height := headingLine + headingMargin
	for i := range view.windows {
		if i > 0 {
			height += windowGap
		}
		height += metaLine + metaMargin + trackHeight
	}
	if view.status != "" {
		layout.status = r.wrap(r.body, view.status, width)
		height += statusMargin + statusLine*float64(len(layout.status))
	}
	layout.height = height
	return layout
}

// gridColumns applies the component's container query: one column with a
// smaller gap when the component is 540 CSS pixels wide or narrower.
func gridColumns(contentWidth float64, columns int) (int, float64) {
	if contentWidth <= narrowWidth {
		return 1, narrowGap
	}
	return columns, gridGap
}

func render(snapshot agentusage.Snapshot, options Options) (*image.RGBA, error) {
	r, err := newRenderer(options)
	if err != nil {
		return nil, err
	}
	padding := float64(options.Padding)
	contentWidth := float64(options.Width) - 2*padding
	columns, gap := gridColumns(contentWidth, options.Columns)
	columnWidth := (contentWidth - gap*float64(columns-1)) / float64(columns)
	if columnWidth < 1 {
		return nil, fmt.Errorf("agentusage/png: width %d leaves no room for %d columns with padding %d",
			options.Width, columns, options.Padding)
	}

	views := providerViews(snapshot, options.Now)
	layouts := make([]providerLayout, len(views))
	for i, view := range views {
		layouts[i] = r.layoutProvider(view, columnWidth)
	}

	// Grid rows are as tall as their tallest provider.
	contentHeight := 0.0
	if len(layouts) == 0 {
		contentHeight = statusLine
	}
	rowTops := make([]float64, 0, (len(layouts)+columns-1)/columns)
	for i := 0; i < len(layouts); i += columns {
		if i > 0 {
			contentHeight += gap
		}
		rowTops = append(rowTops, contentHeight)
		rowHeight := 0.0
		for _, layout := range layouts[i:min(i+columns, len(layouts))] {
			rowHeight = math.Max(rowHeight, layout.height)
		}
		contentHeight += rowHeight
	}

	width := int(math.Round(float64(options.Width) * r.scale))
	height := int(math.Ceil((contentHeight + 2*padding) * r.scale))
	if height > maxDimension {
		return nil, fmt.Errorf("agentusage/png: image height %d exceeds %d pixels", height, maxDimension)
	}
	r.img = image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(r.img, r.img.Bounds(), image.NewUniform(options.Theme.Background), image.Point{}, draw.Src)

	if len(layouts) == 0 {
		r.drawLine(r.body, emptyText, padding, padding, statusLine, options.Theme.Muted, 0)
		return r.img, nil
	}
	for i, layout := range layouts {
		x := padding + float64(i%columns)*(columnWidth+gap)
		y := padding + rowTops[i/columns]
		r.drawProvider(layout, x, y, columnWidth)
	}
	return r.img, nil
}

func (r *renderer) drawProvider(layout providerLayout, x, y, width float64) {
	theme := r.options.Theme
	titleX := x
	if icon, ok := icons[layout.view.id]; ok {
		r.drawIcon(icon, x, y+(headingLine-iconSize)/2, iconSize, r.iconColor(layout.view.id))
		titleX += iconSize + iconGap
	}
	r.drawLine(r.heading, layout.view.title, titleX, y, headingLine, theme.Muted, headingSpacing)
	y += headingLine + headingMargin

	for i, window := range layout.view.windows {
		if i > 0 {
			y += windowGap
		}
		// .meta: the description takes what the "used" column leaves, and
		// truncates with an ellipsis.
		usedWidth := r.measure(r.body, window.used, 0)
		description := r.truncate(r.body, window.description, width-usedWidth-usedMargin)
		r.drawLine(r.body, description, x, y, metaLine, theme.Muted, 0)
		r.drawLine(r.body, window.used, x+width-usedWidth, y, metaLine, theme.Text, 0)
		y += metaLine + metaMargin

		r.fillPill(x, y, width, trackHeight, trackRadius, theme.Track)
		r.fillPill(x, y, width*window.usedPercent/100, trackHeight, trackRadius, theme.Fill)
		r.fillRect(x+width*window.elapsed/100+markerShift, y+markerTop, markerWidth, markerHeight, theme.Text)
		y += trackHeight
	}

	if len(layout.status) > 0 {
		y += statusMargin
		statusColor := theme.Muted
		if layout.view.isError {
			statusColor = theme.Danger
		}
		for _, line := range layout.status {
			r.drawLine(r.body, line, x, y, statusLine, statusColor, 0)
			y += statusLine
		}
	}
}

func (r *renderer) iconColor(id string) color.RGBA {
	if id == agentusage.ProviderIDClaude {
		return r.options.Theme.IconClaude
	}
	return r.options.Theme.IconCodex
}

// snap rounds a device coordinate to a whole pixel, like a browser snaps box
// edges before it paints them.
func snap(v float64) int {
	return int(math.Round(v))
}

// fillRect paints an axis-aligned box. A thin box keeps at least one device
// pixel, so the marker stays visible at any scale.
func (r *renderer) fillRect(x, y, w, h float64, c color.RGBA) {
	x0, x1 := snap(x*r.scale), snap((x+w)*r.scale)
	y0, y1 := snap(y*r.scale), snap((y+h)*r.scale)
	x1, y1 = max(x1, x0+1), max(y1, y0+1)
	draw.Draw(r.img, image.Rect(x0, y0, x1, y1), image.NewUniform(c), image.Point{}, draw.Over)
}

// fillPill paints a rounded box. The radius clamps to half the box like CSS
// border-radius, so a narrow fill stays a circle segment. A box with no
// width paints nothing.
func (r *renderer) fillPill(x, y, w, h, radius float64, c color.RGBA) {
	x0, x1 := snap(x*r.scale), snap((x+w)*r.scale)
	y0, y1 := snap(y*r.scale), snap((y+h)*r.scale)
	if x1 <= x0 || y1 <= y0 {
		return
	}
	width, height := float32(x1-x0), float32(y1-y0)
	rounding := float32(math.Min(radius*r.scale, math.Min(float64(x1-x0), float64(y1-y0))/2))
	r.fillShape(image.Rect(x0, y0, x1, y1), c, func(z *vector.Rasterizer) {
		roundedRect(z, width, height, rounding)
	})
}

func roundedRect(z *vector.Rasterizer, w, h, r float32) {
	const kappa = 0.5522847498 // cubic Bézier approximation of a quarter circle
	k := r * (1 - kappa)
	z.MoveTo(r, 0)
	z.LineTo(w-r, 0)
	z.CubeTo(w-k, 0, w, k, w, r)
	z.LineTo(w, h-r)
	z.CubeTo(w, h-k, w-k, h, w-r, h)
	z.LineTo(r, h)
	z.CubeTo(k, h, 0, h-k, 0, h-r)
	z.LineTo(0, r)
	z.CubeTo(0, k, k, 0, r, 0)
	z.ClosePath()
}

func (r *renderer) drawIcon(icon svgPath, x, y, size float64, c color.RGBA) {
	left, top := x*r.scale, y*r.scale
	unit := size * r.scale / iconViewBox
	bounds := image.Rect(
		int(math.Floor(left)), int(math.Floor(top)),
		int(math.Ceil(left+size*r.scale)), int(math.Ceil(top+size*r.scale)),
	)
	originX, originY := float64(bounds.Min.X), float64(bounds.Min.Y)
	r.fillShape(bounds, c, func(z *vector.Rasterizer) {
		icon.rasterize(z, func(p point) (float32, float32) {
			return float32(left + p.x*unit - originX), float32(top + p.y*unit - originY)
		})
	})
}

// fillShape rasterizes a path in the coordinate space of bounds, with (0, 0)
// at bounds.Min, and composites it over the image.
func (r *renderer) fillShape(bounds image.Rectangle, c color.RGBA, build func(*vector.Rasterizer)) {
	if !bounds.In(r.img.Bounds()) {
		return
	}
	r.rast.Reset(bounds.Dx(), bounds.Dy())
	build(r.rast)
	r.rast.Draw(r.img, bounds, image.NewUniform(c), image.Point{})
}

// drawLine draws text in a CSS line box: the baseline sits so the font's
// ascent and descent center in the line height, like half-leading.
func (r *renderer) drawLine(face font.Face, text string, x, top, lineHeight float64, c color.RGBA, spacing float64) {
	metrics := face.Metrics()
	ascent := float64(metrics.Ascent) / 64 / r.scale
	descent := float64(metrics.Descent) / 64 / r.scale
	baseline := top + (lineHeight-(ascent+descent))/2 + ascent
	r.drawText(face, text, x, baseline, c, spacing)
}

// drawText draws text with kerning and CSS letter-spacing. The baseline
// snaps to a device pixel; glyphs keep sub-pixel horizontal positions.
func (r *renderer) drawText(face font.Face, text string, x, baseline float64, c color.RGBA, spacing float64) {
	src := image.NewUniform(c)
	dot := fixed.Point26_6{X: toFixed(x * r.scale), Y: fixed.I(snap(baseline * r.scale))}
	space := toFixed(spacing * r.scale)
	previous := rune(-1)
	for _, ch := range text {
		if previous >= 0 {
			dot.X += face.Kern(previous, ch)
		}
		bounds, mask, maskPoint, advance, ok := face.Glyph(dot, ch)
		if !ok {
			previous = -1
			continue
		}
		draw.DrawMask(r.img, bounds, src, image.Point{}, mask, maskPoint, draw.Over)
		dot.X += advance + space
		previous = ch
	}
}

// measure returns the advance width of text in CSS pixels.
func (r *renderer) measure(face font.Face, text string, spacing float64) float64 {
	var width fixed.Int26_6
	space := toFixed(spacing * r.scale)
	previous := rune(-1)
	for _, ch := range text {
		if previous >= 0 {
			width += face.Kern(previous, ch)
		}
		advance, _ := face.GlyphAdvance(ch)
		width += advance + space
		previous = ch
	}
	return float64(width) / 64 / r.scale
}

// truncate applies text-overflow: ellipsis. It drops characters from the
// end until the text and an ellipsis fit in width.
func (r *renderer) truncate(face font.Face, text string, width float64) string {
	if r.measure(face, text, 0) <= width {
		return text
	}
	runes := []rune(text)
	for n := len(runes) - 1; n >= 0; n-- {
		candidate := string(runes[:n]) + "…"
		if r.measure(face, candidate, 0) <= width {
			return candidate
		}
	}
	return ""
}

// wrap breaks text at spaces into lines no wider than width. A single word
// wider than width takes its own line.
func (r *renderer) wrap(face font.Face, text string, width float64) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{words[0]}
	for _, word := range words[1:] {
		candidate := lines[len(lines)-1] + " " + word
		if r.measure(face, candidate, 0) <= width {
			lines[len(lines)-1] = candidate
			continue
		}
		lines = append(lines, word)
	}
	return lines
}

func toFixed(v float64) fixed.Int26_6 {
	return fixed.Int26_6(math.Round(v * 64))
}
