package agentusage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

type codexRateLimitWindow struct {
	UsedPercent       float64 `json:"usedPercent"`
	WindowDurationMin int64   `json:"windowDurationMins"`
	ResetsAt          int64   `json:"resetsAt"`
}

type codexRateLimit struct {
	LimitID   string                `json:"limitId"`
	LimitName *string               `json:"limitName"`
	Primary   *codexRateLimitWindow `json:"primary"`
	Secondary *codexRateLimitWindow `json:"secondary"`
}

type codexRateLimitResult struct {
	RateLimits          *codexRateLimit            `json:"rateLimits"`
	RateLimitsByLimitID map[string]*codexRateLimit `json:"rateLimitsByLimitId"`
}

type rpcResponse struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (f *Fetcher) fetchCodex(ctx context.Context) ([]Window, error) {
	result, err := f.readCodexRateLimits(ctx)
	if err != nil {
		return nil, err
	}

	rateLimit := result.RateLimits
	if rateLimit == nil {
		rateLimit = result.RateLimitsByLimitID[ProviderIDCodex]
	}
	if rateLimit == nil {
		return nil, fmt.Errorf("Codex returned no rate-limit bucket")
	}
	var windows []Window
	for _, entry := range []struct {
		id     string
		window *codexRateLimitWindow
	}{
		{CodexWindowIDPrimary, rateLimit.Primary},
		{CodexWindowIDSecondary, rateLimit.Secondary},
	} {
		if entry.window == nil || entry.window.WindowDurationMin <= 0 {
			continue
		}
		seconds := entry.window.WindowDurationMin * 60
		windows = append(windows, Window{
			ID:            entry.id,
			Label:         windowLabel(seconds),
			UsedPercent:   clampPercent(entry.window.UsedPercent),
			WindowSeconds: seconds,
			ResetsAt:      unixTime(entry.window.ResetsAt),
		})
	}
	sort.SliceStable(windows, func(i, j int) bool {
		return windows[i].WindowSeconds < windows[j].WindowSeconds
	})
	if len(windows) == 0 {
		return nil, fmt.Errorf("Codex returned no usage windows")
	}
	return windows, nil
}

func (f *Fetcher) readCodexRateLimits(ctx context.Context) (codexRateLimitResult, error) {
	command := f.startCodex(ctx)
	if f.codexHome != "" {
		environment := command.Env
		if environment == nil {
			environment = os.Environ()
		}
		command.Env = append(environment, "CODEX_HOME="+f.codexHome)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		return codexRateLimitResult{}, fmt.Errorf("open Codex app-server input: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return codexRateLimitResult{}, fmt.Errorf("open Codex app-server output: %w", err)
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return codexRateLimitResult{}, fmt.Errorf("start Codex app-server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if command.ProcessState == nil {
			_ = command.Process.Kill()
		}
		_ = command.Wait()
	}()

	encoder := json.NewEncoder(stdin)
	decoder := json.NewDecoder(io.LimitReader(stdout, maxResponse))
	if err := encoder.Encode(map[string]any{
		"method": "initialize",
		"id":     1,
		"params": map[string]any{"clientInfo": map[string]string{
			"name": "agentusage", "title": "agentusage", "version": "0",
		}},
	}); err != nil {
		return codexRateLimitResult{}, fmt.Errorf("initialize Codex app-server: %w", err)
	}
	if _, err := readRPCResponse(decoder, 1); err != nil {
		return codexRateLimitResult{}, fmt.Errorf("initialize Codex app-server: %w", err)
	}
	if err := encoder.Encode(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return codexRateLimitResult{}, fmt.Errorf("acknowledge Codex app-server: %w", err)
	}
	if err := encoder.Encode(map[string]any{"method": "account/rateLimits/read", "id": 2}); err != nil {
		return codexRateLimitResult{}, fmt.Errorf("request Codex rate limits: %w", err)
	}
	raw, err := readRPCResponse(decoder, 2)
	if err != nil {
		return codexRateLimitResult{}, fmt.Errorf("read Codex rate limits: %w", err)
	}
	var result codexRateLimitResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return codexRateLimitResult{}, fmt.Errorf("decode Codex rate limits: %w", err)
	}
	return result, nil
}

func readRPCResponse(decoder *json.Decoder, id int) (json.RawMessage, error) {
	for {
		var response rpcResponse
		if err := decoder.Decode(&response); err != nil {
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("app-server closed before response %d", id)
			}
			return nil, err
		}
		if response.ID == nil || *response.ID != id {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("RPC %d: %s", response.Error.Code, response.Error.Message)
		}
		if len(response.Result) == 0 {
			return nil, fmt.Errorf("app-server response %d has no result", id)
		}
		return response.Result, nil
	}
}

func unixTime(value int64) *time.Time {
	if value <= 0 {
		return nil
	}
	timestamp := time.Unix(value, 0).UTC()
	return &timestamp
}
