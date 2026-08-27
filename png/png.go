// Package png renders an agentusage.Snapshot to a PNG image without HTML or a
// browser. The output matches the dark theme of the agent-usage Web
// Component: the same layout, colors, and text.
//
// The package is optional. Applications that only need the JSON contract or
// the Web Component do not import it, and so do not link its font and
// rasterizer dependency.
package png

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	stdpng "image/png"
	"io"
	"math"
	"time"

	"github.com/wellsjo/agentusage"
)

const (
	defaultWidth   = 720
	defaultScale   = 2.0
	defaultPadding = 16
	defaultColumns = 2
	// maxDimension caps the pixel size of one image side. The cap keeps a
	// mistaken Width or Scale from an enormous allocation.
	maxDimension = 16384
)

// Theme holds the colors of a rendered image. The fields mirror the CSS
// custom properties of the Web Component; Background is the page color
// behind the component.
type Theme struct {
	Background color.RGBA
	Fill       color.RGBA
	Track      color.RGBA
	Text       color.RGBA
	Muted      color.RGBA
	Danger     color.RGBA
	IconCodex  color.RGBA
	IconClaude color.RGBA
}

// Dark matches the Web Component under prefers-color-scheme: dark.
var Dark = Theme{
	Background: color.RGBA{0x0d, 0x11, 0x17, 0xff},
	Fill:       color.RGBA{0x3f, 0xb9, 0x50, 0xff},
	Track:      color.RGBA{0x30, 0x36, 0x3d, 0xff},
	Text:       color.RGBA{0xf0, 0xf6, 0xfc, 0xff},
	Muted:      color.RGBA{0x8b, 0x94, 0x9e, 0xff},
	Danger:     color.RGBA{0xf8, 0x51, 0x49, 0xff},
	IconCodex:  color.RGBA{0x8b, 0x94, 0x9e, 0xff},
	IconClaude: color.RGBA{0xd9, 0x77, 0x57, 0xff},
}

// Light matches the Web Component's default palette.
var Light = Theme{
	Background: color.RGBA{0xff, 0xff, 0xff, 0xff},
	Fill:       color.RGBA{0x1f, 0x88, 0x3d, 0xff},
	Track:      color.RGBA{0xd8, 0xde, 0xe4, 0xff},
	Text:       color.RGBA{0x1f, 0x23, 0x28, 0xff},
	Muted:      color.RGBA{0x59, 0x63, 0x6e, 0xff},
	Danger:     color.RGBA{0xcf, 0x22, 0x2e, 0xff},
	IconCodex:  color.RGBA{0x59, 0x63, 0x6e, 0xff},
	IconClaude: color.RGBA{0xd9, 0x77, 0x57, 0xff},
}

// Options controls the rendered image. The zero value renders the dark theme
// at 720 CSS pixels wide with a device pixel ratio of 2.
type Options struct {
	// Width is the image width in CSS pixels, Padding included. Zero uses 720.
	Width int
	// Scale is the device pixel ratio. The image is Width*Scale pixels wide.
	// Zero uses 2.
	Scale float64
	// Padding is the space between the image edge and the component, in CSS
	// pixels. Zero uses 16; a negative value removes the padding.
	Padding int
	// Columns is the number of provider columns. Zero uses 2. A component
	// narrower than 540 CSS pixels uses one column, like the Web Component's
	// container query.
	Columns int
	// Now is the reference time for the "resets in" text and the elapsed-time
	// marker. Zero uses time.Now().
	Now time.Time
	// Theme sets the colors. The zero value uses Dark.
	Theme Theme
	// Font and HeadingFont hold TrueType or OpenType data for body text and
	// for the provider heading. Nil uses the Go fonts (Go Regular and Go
	// Bold), which ship with the rasterizer dependency.
	Font        []byte
	HeadingFont []byte
}

// Render draws the snapshot and returns the image.
func Render(snapshot agentusage.Snapshot, options Options) (*image.RGBA, error) {
	options, err := options.normalize()
	if err != nil {
		return nil, err
	}
	return render(snapshot, options)
}

// Encode draws the snapshot and writes it to w as PNG.
func Encode(w io.Writer, snapshot agentusage.Snapshot, options Options) error {
	img, err := Render(snapshot, options)
	if err != nil {
		return err
	}
	return stdpng.Encode(w, img)
}

func (o Options) normalize() (Options, error) {
	if o.Width == 0 {
		o.Width = defaultWidth
	}
	if o.Width < 0 {
		return o, fmt.Errorf("agentusage/png: width %d is negative", o.Width)
	}
	if o.Scale == 0 {
		o.Scale = defaultScale
	}
	if o.Scale < 0 || math.IsNaN(o.Scale) || math.IsInf(o.Scale, 0) {
		return o, fmt.Errorf("agentusage/png: scale %v is not a positive number", o.Scale)
	}
	if o.Padding == 0 {
		o.Padding = defaultPadding
	}
	if o.Padding < 0 {
		o.Padding = 0
	}
	if o.Columns == 0 {
		o.Columns = defaultColumns
	}
	if o.Columns < 0 {
		return o, fmt.Errorf("agentusage/png: columns %d is negative", o.Columns)
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.Theme == (Theme{}) {
		o.Theme = Dark
	}
	if float64(o.Width)*o.Scale > maxDimension {
		return o, errors.New("agentusage/png: width times scale exceeds 16384 pixels")
	}
	return o, nil
}
