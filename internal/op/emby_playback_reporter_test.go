package op

import (
	"testing"
	"time"

	"github.com/synctv-org/synctv/internal/model"
)

func TestParseEmbySourceMetadata(t *testing.T) {
	metadata, err := parseEmbySourceMetadata(
		"https://emby.example/emby/videos/1/original.mp4?MediaSourceId=media-1&PlaySessionId=play-1&AudioStreamIndex=1&SubtitleStreamIndex=0&Static=true",
	)
	if err != nil {
		t.Fatalf("parseEmbySourceMetadata: %v", err)
	}
	if metadata.mediaSourceID != "media-1" || metadata.playSessionID != "play-1" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
	if metadata.audioStreamIndex == nil || *metadata.audioStreamIndex != 1 {
		t.Fatalf("audio stream index = %v", metadata.audioStreamIndex)
	}
	if metadata.subtitleStreamIndex == nil || *metadata.subtitleStreamIndex != 0 {
		t.Fatalf("subtitle stream index = %v", metadata.subtitleStreamIndex)
	}
	if !metadata.static {
		t.Fatal("expected static source")
	}
}

func TestPlayMethodForSource(t *testing.T) {
	tests := []struct {
		name       string
		transcoded bool
		path       string
		static     bool
		want       string
	}{
		{name: "transcode", transcoded: true, path: "/master.m3u8", want: "Transcode"},
		{name: "original", path: "/emby/videos/1/original.mp4", want: "DirectPlay"},
		{name: "static", path: "/emby/Videos/1/stream.mp4", static: true, want: "DirectPlay"},
		{name: "direct stream", path: "/emby/Videos/1/stream.mp4", want: "DirectStream"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := playMethodForSource(test.transcoded, test.path, test.static); got != test.want {
				t.Fatalf("playMethodForSource() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEmbyProgressEvent(t *testing.T) {
	baseTime := time.Unix(100, 0)
	playing := model.Status{IsPlaying: true, CurrentTime: 10, PlaybackRate: 1}

	tests := []struct {
		name     string
		previous model.Status
		current  model.Status
		now      time.Time
		want     string
	}{
		{
			name:     "pause",
			previous: playing,
			current:  model.Status{IsPlaying: false, CurrentTime: 12, PlaybackRate: 1},
			now:      baseTime.Add(2 * time.Second),
			want:     "Pause",
		},
		{
			name:     "unpause",
			previous: model.Status{IsPlaying: false, CurrentTime: 10, PlaybackRate: 1},
			current:  playing,
			now:      baseTime.Add(2 * time.Second),
			want:     "Unpause",
		},
		{
			name:     "rate",
			previous: playing,
			current:  model.Status{IsPlaying: true, CurrentTime: 12, PlaybackRate: 1.5},
			now:      baseTime.Add(2 * time.Second),
			want:     "PlaybackRateChange",
		},
		{
			name:     "seek",
			previous: playing,
			current:  model.Status{IsPlaying: true, CurrentTime: 40, PlaybackRate: 1},
			now:      baseTime.Add(2 * time.Second),
			want:     "TimeUpdate",
		},
		{
			name:     "normal progress",
			previous: playing,
			current:  model.Status{IsPlaying: true, CurrentTime: 12, PlaybackRate: 1},
			now:      baseTime.Add(2 * time.Second),
			want:     "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := embyProgressEvent(test.previous, baseTime, test.current, test.now); got != test.want {
				t.Fatalf("embyProgressEvent() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSecondsToTicks(t *testing.T) {
	if got := secondsToTicks(12.345); got != 123450000 {
		t.Fatalf("secondsToTicks() = %d", got)
	}
	if got := secondsToTicks(-1); got != 0 {
		t.Fatalf("secondsToTicks(-1) = %d", got)
	}
}
