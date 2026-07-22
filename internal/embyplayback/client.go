package embyplayback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	playbackStartedPath  = "/emby/Sessions/Playing"
	playbackProgressPath = "/emby/Sessions/Playing/Progress"
	playbackStoppedPath  = "/emby/Sessions/Playing/Stopped"
)

type Client struct {
	host       string
	token      string
	userID     string
	deviceID   string
	deviceName string
	version    string
	httpClient *http.Client
}

type Option func(*Client)

func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

func NewClient(host, token, userID, deviceID, deviceName, version string, options ...Option) *Client {
	client := &Client{
		host:       strings.TrimRight(host, "/"),
		token:      token,
		userID:     userID,
		deviceID:   deviceID,
		deviceName: deviceName,
		version:    version,
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
	for _, option := range options {
		option(client)
	}
	return client
}

type Report struct {
	QueueableMediaTypes []string `json:"QueueableMediaTypes"`
	CanSeek             bool     `json:"CanSeek"`
	ItemID              string   `json:"ItemId"`
	MediaSourceID       string   `json:"MediaSourceId,omitempty"`
	AudioStreamIndex    *int     `json:"AudioStreamIndex,omitempty"`
	SubtitleStreamIndex *int     `json:"SubtitleStreamIndex,omitempty"`
	IsPaused            bool     `json:"IsPaused"`
	IsMuted             bool     `json:"IsMuted"`
	PositionTicks       int64    `json:"PositionTicks,omitempty"`
	PlayMethod          string   `json:"PlayMethod,omitempty"`
	PlaySessionID       string   `json:"PlaySessionId"`
	PlaylistIndex       int      `json:"PlaylistIndex"`
	PlaylistLength      int      `json:"PlaylistLength"`
	PlaybackRate        float64  `json:"PlaybackRate,omitempty"`
	EventName           string   `json:"EventName,omitempty"`
}

func (c *Client) Started(ctx context.Context, report Report) error {
	report.EventName = ""
	return c.post(ctx, playbackStartedPath, report)
}

func (c *Client) Progress(ctx context.Context, report Report, eventName string) error {
	report.EventName = eventName
	return c.post(ctx, playbackProgressPath, report)
}

func (c *Client) Stopped(ctx context.Context, report Report) error {
	report.EventName = ""
	return c.post(ctx, playbackStoppedPath, report)
}

func (c *Client) post(ctx context.Context, path string, report Report) error {
	endpoint, err := url.JoinPath(c.host, path)
	if err != nil {
		return fmt.Errorf("build Emby playback endpoint: %w", err)
	}

	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encode Emby playback report: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Emby playback request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Emby-Token", c.token)
	}
	req.Header.Set("X-Emby-Authorization", c.authorizationHeader())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send Emby playback request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	limited := io.LimitReader(resp.Body, 4096)
	responseBody, _ := io.ReadAll(limited)
	return fmt.Errorf("Emby playback request returned %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
}

func (c *Client) authorizationHeader() string {
	return fmt.Sprintf(
		`Emby UserId="%s", Client="SyncTV", Device="%s", DeviceId="%s", Version="%s"`,
		escapeHeaderValue(c.userID),
		escapeHeaderValue(c.deviceName),
		escapeHeaderValue(c.deviceID),
		escapeHeaderValue(c.version),
	)
}

func escapeHeaderValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}
