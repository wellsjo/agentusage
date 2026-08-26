package agentusage

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	maxResponse = 1 << 20
	// maxRetryAfter caps a server Retry-After value. The cap protects against
	// an absurd or overflowed backoff that would block a provider forever.
	maxRetryAfter = 24 * time.Hour
)

func (f *Fetcher) fetchJSON(request *http.Request, destination any) error {
	response, err := f.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &httpStatusError{
			Code:       response.StatusCode,
			RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), f.now()),
		}
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponse)).Decode(destination); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

type httpStatusError struct {
	Code       int
	RetryAfter time.Duration
}

func (err *httpStatusError) Error() string {
	if err.RetryAfter > 0 {
		return fmt.Sprintf("HTTP %d (retry in %s)", err.Code, err.RetryAfter.Round(time.Second))
	}
	return fmt.Sprintf("HTTP %d", err.Code)
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && seconds > 0 {
		if seconds > int64(maxRetryAfter/time.Second) {
			return maxRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return min(at.Sub(now), maxRetryAfter)
	}
	return 0
}

func retryBlocked(until, now time.Time) error {
	return fmt.Errorf("provider rate limited (retry in %s)", until.Sub(now).Round(time.Second))
}

func windowLabel(seconds int64) string {
	if seconds == weekSeconds {
		return "1w window"
	}
	if seconds%(24*60*60) == 0 {
		return fmt.Sprintf("%dd window", seconds/(24*60*60))
	}
	if seconds%(60*60) == 0 {
		return fmt.Sprintf("%dh window", seconds/(60*60))
	}
	return fmt.Sprintf("%dm window", seconds/60)
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func slug(value string) string {
	var builder strings.Builder
	dash := false
	for _, char := range strings.ToLower(value) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			builder.WriteRune(char)
			dash = false
			continue
		}
		if builder.Len() > 0 && !dash {
			builder.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}
