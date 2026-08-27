package png

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	stdpng "image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wellsjo/agentusage"
)

var update = flag.Bool("update", false, "rewrite the golden images in testdata")

var testNow = time.Date(2026, 8, 5, 18, 0, 0, 0, time.UTC)

func at(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return &parsed
}

func fullSnapshot() agentusage.Snapshot {
	fetched := testNow.Add(-time.Minute)
	return agentusage.Snapshot{
		Providers: []agentusage.Provider{
			{ID: agentusage.ProviderIDCodex, Name: "Codex", FetchedAt: &fetched, Windows: []agentusage.Window{
				{ID: agentusage.CodexWindowIDPrimary, Label: "5h window", UsedPercent: 12.5, WindowSeconds: 5 * 60 * 60, ResetsAt: at("2026-08-05T19:40:00Z")},
				{ID: agentusage.CodexWindowIDSecondary, Label: "1w window", UsedPercent: 65, WindowSeconds: 7 * 24 * 60 * 60, ResetsAt: at("2026-08-10T20:00:00Z")},
			}},
			{ID: agentusage.ProviderIDClaude, Name: "Claude Code", FetchedAt: &fetched, Windows: []agentusage.Window{
				{ID: "five-hour", Label: "5h window", UsedPercent: 0, WindowSeconds: 5 * 60 * 60},
				{ID: "seven-day", Label: "1w window", Scope: "all models", UsedPercent: 45, WindowSeconds: 7 * 24 * 60 * 60, ResetsAt: at("2026-08-06T13:50:00Z")},
				{ID: "weekly-fable", Label: "1w window", Scope: "Fable", UsedPercent: 88, WindowSeconds: 7 * 24 * 60 * 60, ResetsAt: at("2026-08-06T13:50:00Z")},
			}},
		},
		UpdatedAt: testNow,
	}
}

func staleSnapshot() agentusage.Snapshot {
	snapshot := fullSnapshot()
	snapshot.Providers[0].Stale = true
	snapshot.Providers[0].Error = "HTTP 429 (retry in 10m)"
	snapshot.Providers[1].Windows = []agentusage.Window{}
	snapshot.Providers[1].FetchedAt = nil
	snapshot.Providers[1].Error = "claude credentials: the credential file is missing and the macOS Keychain has no Claude Code item; sign in with claude login"
	return snapshot
}

func narrowSnapshot() agentusage.Snapshot {
	snapshot := fullSnapshot()
	snapshot.Providers[0].Windows[1].Scope = "Claude Sonnet 4.5 with extended thinking and a 1M context window"
	snapshot.Providers = append(snapshot.Providers, agentusage.Provider{
		ID: "gemini", Name: "Gemini", Windows: []agentusage.Window{{ID: "daily", UsedPercent: 33.333}},
	})
	return snapshot
}

type goldenCase struct {
	name     string
	snapshot agentusage.Snapshot
	options  Options
}

func goldenCases() []goldenCase {
	return []goldenCase{
		{name: "dark", snapshot: fullSnapshot(), options: Options{Now: testNow}},
		{name: "stale", snapshot: staleSnapshot(), options: Options{Now: testNow, Scale: 1}},
		{name: "empty", snapshot: agentusage.Snapshot{UpdatedAt: testNow}, options: Options{Now: testNow, Scale: 1}},
		{name: "narrow", snapshot: narrowSnapshot(), options: Options{Now: testNow, Width: 480}},
		{name: "light", snapshot: fullSnapshot(), options: Options{Now: testNow, Scale: 1, Width: 640, Theme: Light}},
	}
}

// TestGolden compares each rendering with the image in testdata. Run
// go test ./png -update to accept new images. The comparison allows a small
// per-channel difference on a small share of pixels, because the rasterizer
// uses different float code paths on different CPUs.
func TestGolden(t *testing.T) {
	for _, tc := range goldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.snapshot, tc.options)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", tc.name+".png")
			if *update {
				writePNG(t, path, got)
				return
			}
			want := readPNG(t, path)
			if diff := compareImages(want, got); diff != "" {
				actual := filepath.Join("testdata", tc.name+".actual.png")
				writePNG(t, actual, got)
				t.Fatalf("%s: %s\nwrote %s; run go test ./png -update to accept it", path, diff, actual)
			}
		})
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	var buffer bytes.Buffer
	if err := stdpng.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readPNG(t *testing.T, path string) image.Image {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./png -update to create it)", err)
	}
	img, err := stdpng.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

const (
	goldenMaxDelta   = 8     // per channel, 0-255
	goldenMaxChanged = 0.005 // share of pixels that may differ at all
)

func compareImages(want, got image.Image) string {
	if want.Bounds() != got.Bounds() {
		return "size " + got.Bounds().Size().String() + ", want " + want.Bounds().Size().String()
	}
	bounds := want.Bounds()
	changed, worst := 0, 0
	var worstAt image.Point
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			w := color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
			g := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
			delta := max(absDiff(w.R, g.R), absDiff(w.G, g.G), absDiff(w.B, g.B), absDiff(w.A, g.A))
			if delta > 0 {
				changed++
			}
			if delta > worst {
				worst, worstAt = delta, image.Point{x, y}
			}
		}
	}
	total := bounds.Dx() * bounds.Dy()
	if worst > goldenMaxDelta || float64(changed) > goldenMaxChanged*float64(total) {
		return "pixels differ: " + itoa(changed) + " of " + itoa(total) + " changed, largest channel delta " + itoa(worst) + " at " + worstAt.String()
	}
	return ""
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func TestRenderDefaults(t *testing.T) {
	img, err := Render(fullSnapshot(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dx(); got != int(defaultWidth*defaultScale) {
		t.Errorf("width = %d, want %d", got, int(defaultWidth*defaultScale))
	}
	if img.Bounds().Dy() <= 2*defaultPadding*defaultScale {
		t.Errorf("height = %d, want room for content", img.Bounds().Dy())
	}
	if got := img.RGBAAt(0, 0); got != Dark.Background {
		t.Errorf("corner = %v, want the dark background %v", got, Dark.Background)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	first, err := Render(fullSnapshot(), Options{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(fullSnapshot(), Options{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Pix, second.Pix) {
		t.Fatal("two renderings of the same snapshot differ")
	}
}

func TestEncodeWritesPNG(t *testing.T) {
	var buffer bytes.Buffer
	if err := Encode(&buffer, fullSnapshot(), Options{Width: 300, Scale: 1, Now: testNow}); err != nil {
		t.Fatal(err)
	}
	img, format, err := image.Decode(bytes.NewReader(buffer.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" {
		t.Errorf("format = %q", format)
	}
	if got := img.Bounds().Dx(); got != 300 {
		t.Errorf("width = %d, want 300", got)
	}
}

func TestRenderRejectsBadOptions(t *testing.T) {
	cases := map[string]Options{
		"negative width":   {Width: -1},
		"negative scale":   {Scale: -1},
		"nan scale":        {Scale: math.NaN()},
		"negative columns": {Columns: -1},
		"huge image":       {Width: 20000, Scale: 1},
		"no room":          {Width: 32, Padding: 16},
		"bad font":         {Font: []byte("not a font")},
		"bad heading font": {HeadingFont: []byte("not a font")},
	}
	for name, options := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Render(fullSnapshot(), options); err == nil {
				t.Fatal("Render accepted the options")
			}
		})
	}
}

func TestRenderAcceptsCustomFont(t *testing.T) {
	img, err := Render(fullSnapshot(), Options{Now: testNow, Scale: 1, Font: goRegular.data, HeadingFont: goRegular.data})
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Empty() {
		t.Fatal("empty image")
	}
}

func TestRenderNoPadding(t *testing.T) {
	img, err := Render(fullSnapshot(), Options{Now: testNow, Scale: 1, Padding: -1})
	if err != nil {
		t.Fatal(err)
	}
	// With no padding, the Codex icon paints in the top-left corner.
	if got := img.RGBAAt(6, 7); got == Dark.Background {
		t.Errorf("pixel (6,7) = %v, want the icon over the background", got)
	}
}

func TestGridColumns(t *testing.T) {
	cases := []struct {
		width   float64
		columns int
		want    int
		gap     float64
	}{
		{688, 2, 2, gridGap},
		{541, 2, 2, gridGap},
		{540, 2, 1, narrowGap},
		{300, 3, 1, narrowGap},
		{900, 3, 3, gridGap},
	}
	for _, tc := range cases {
		columns, gap := gridColumns(tc.width, tc.columns)
		if columns != tc.want || gap != tc.gap {
			t.Errorf("gridColumns(%v, %d) = (%d, %v), want (%d, %v)", tc.width, tc.columns, columns, gap, tc.want, tc.gap)
		}
	}
}

func testRenderer(t *testing.T) *renderer {
	t.Helper()
	options, err := Options{Scale: 1, Now: testNow}.normalize()
	if err != nil {
		t.Fatal(err)
	}
	r, err := newRenderer(options)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestTruncate(t *testing.T) {
	r := testRenderer(t)
	text := "1w window · all models · resets in 19h 50m"
	full := r.measure(r.body, text, 0)
	if got := r.truncate(r.body, text, full); got != text {
		t.Errorf("truncate at full width = %q, want the text unchanged", got)
	}
	got := r.truncate(r.body, text, full/2)
	if !strings.HasSuffix(got, "…") || !strings.HasPrefix(text, strings.TrimSuffix(got, "…")) {
		t.Errorf("truncate = %q, want a prefix of the text with an ellipsis", got)
	}
	if width := r.measure(r.body, got, 0); width > full/2 {
		t.Errorf("truncated width %v exceeds %v", width, full/2)
	}
	if got := r.truncate(r.body, text, 0); got != "" {
		t.Errorf("truncate with no room = %q, want empty", got)
	}
}

func TestWrap(t *testing.T) {
	r := testRenderer(t)
	text := "Stale data · provider rate limited (retry in 9m30s) and more words"
	width := r.measure(r.body, text, 0) / 3
	lines := r.wrap(r.body, text, width)
	if len(lines) < 3 {
		t.Fatalf("wrap produced %d lines: %q", len(lines), lines)
	}
	if got := strings.Join(lines, " "); got != text {
		t.Errorf("wrapped text = %q, want every word kept in order", got)
	}
	for _, line := range lines {
		if strings.Contains(line, " ") && r.measure(r.body, line, 0) > width {
			t.Errorf("line %q is wider than %v", line, width)
		}
	}
	if got := r.wrap(r.body, "   ", width); len(got) != 1 || got[0] != "" {
		t.Errorf("wrap of blank text = %q, want one empty line", got)
	}
}

func TestIconPathsMatchWebComponent(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "web", "agent-usage.js"))
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"codex": codexIconPath, "claude": claudeIconPath} {
		if !bytes.Contains(script, []byte(name+`: "`+path+`"`)) {
			t.Errorf("the %s icon path differs from the Web Component", name)
		}
	}
}
