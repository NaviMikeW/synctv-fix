package embyplayback

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlaybackCheckIns(t *testing.T) {
	t.Helper()

	paths := []string{
		playbackStartedPath,
		playbackProgressPath,
		playbackStoppedPath,
	}
	requestIndex := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestIndex >= len(paths) {
			t.Fatalf("unexpected extra request to %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != paths[requestIndex] {
			t.Errorf("path = %s, want %s", r.URL.Path, paths[requestIndex])
		}
		if got := r.Header.Get("X-Emby-Token"); got != "secret-token" {
			t.Errorf("X-Emby-Token = %q", got)
		}
		authorization := r.Header.Get("X-Emby-Authorization")
		for _, expected := range []string{
			`UserId="user-1"`,
			`Client="SyncTV"`,
			`Device="SyncTV · Living Room"`,
			`DeviceId="synctv-room-1"`,
			`Version="v0.9.15"`,
		} {
			if !strings.Contains(authorization, expected) {
				t.Errorf("authorization header %q does not contain %q", authorization, expected)
			}
		}

		var report Report
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Fatalf("decode report: %v", err)
		}
		if report.ItemID != "item-1" || report.MediaSourceID != "media-1" || report.PlaySessionID != "play-1" {
			t.Errorf("unexpected report identifiers: %+v", report)
		}
		if requestIndex == 1 && report.EventName != "Pause" {
			t.Errorf("progress event = %q, want Pause", report.EventName)
		}
		if requestIndex != 1 && report.EventName != "" {
			t.Errorf("event should be empty for request %d, got %q", requestIndex, report.EventName)
		}

		requestIndex++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	audioIndex := 1
	subtitleIndex := 2
	report := Report{
		QueueableMediaTypes: []string{"Video"},
		CanSeek:             true,
		ItemID:              "item-1",
		MediaSourceID:       "media-1",
		AudioStreamIndex:    &audioIndex,
		SubtitleStreamIndex: &subtitleIndex,
		IsPaused:            false,
		PositionTicks:       123000000,
		PlayMethod:          "Transcode",
		PlaySessionID:       "play-1",
		PlaylistLength:      1,
		PlaybackRate:        1,
	}

	client := NewClient(
		server.URL,
		"secret-token",
		"user-1",
		"synctv-room-1",
		"SyncTV · Living Room",
		"v0.9.15",
		WithHTTPClient(server.Client()),
	)

	if err := client.Started(context.Background(), report); err != nil {
		t.Fatalf("Started: %v", err)
	}
	if err := client.Progress(context.Background(), report, "Pause"); err != nil {
		t.Fatalf("Progress: %v", err)
	}
	if err := client.Stopped(context.Background(), report); err != nil {
		t.Fatalf("Stopped: %v", err)
	}
	if requestIndex != len(paths) {
		t.Fatalf("requests = %d, want %d", requestIndex, len(paths))
	}
}

func TestPlaybackCheckInErrorDoesNotExposeToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "server failure", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "very-secret-token", "user", "device", "room", "v0.9.15", WithHTTPClient(server.Client()))
	err := client.Started(context.Background(), Report{ItemID: "item", PlaySessionID: "play"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "very-secret-token") {
		t.Fatalf("error leaked token: %v", err)
	}
}
