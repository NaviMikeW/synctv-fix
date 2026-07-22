package op

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/synctv-org/synctv/internal/embyplayback"
	"github.com/synctv-org/synctv/internal/model"
	"github.com/synctv-org/synctv/internal/version"
	"github.com/synctv-org/synctv/utils"
)

const (
	embyPlaybackScanInterval     = 2 * time.Second
	embyPlaybackProgressInterval = 10 * time.Second
	embyPlaybackRequestTimeout   = 8 * time.Second
	embyPlaybackSeekThreshold    = 4 * time.Second
	embyPlaybackErrorLogInterval = 30 * time.Second
)

var embyPlaybackReporterOnce sync.Once

func startEmbyPlaybackReporter() {
	embyPlaybackReporterOnce.Do(func() {
		reporter := &embyPlaybackReporter{
			sessions:     make(map[string]*trackedEmbyPlayback),
			lastErrorLog: make(map[string]time.Time),
		}
		go reporter.run()
	})
}

type embyPlaybackReporter struct {
	sessions     map[string]*trackedEmbyPlayback
	lastErrorLog map[string]time.Time
}

type trackedEmbyPlayback struct {
	client       *embyplayback.Client
	movieKey     string
	report       embyplayback.Report
	lastStatus   model.Status
	lastObserved time.Time
	lastReported time.Time
}

func (r *embyPlaybackReporter) run() {
	ticker := time.NewTicker(embyPlaybackScanInterval)
	defer ticker.Stop()

	for range ticker.C {
		r.scan()
	}
}

func (r *embyPlaybackReporter) scan() {
	rooms := make(map[string]*Room)
	RangeRoomCache(func(roomID string, entry *RoomEntry) bool {
		if entry != nil && entry.Value() != nil {
			rooms[roomID] = entry.Value()
		}
		return true
	})

	for roomID, room := range rooms {
		r.syncRoom(roomID, room)
	}

	for roomID := range r.sessions {
		if _, exists := rooms[roomID]; !exists {
			r.stop(roomID, "room left cache")
		}
	}
}

func (r *embyPlaybackReporter) syncRoom(roomID string, room *Room) {
	tracked := r.sessions[roomID]
	if room.ViewerCount() == 0 {
		if tracked != nil {
			r.stop(roomID, "no viewers")
		}
		return
	}

	current := room.Current()
	if current.Movie.ID == "" {
		if tracked != nil {
			r.stop(roomID, "no current movie")
		}
		return
	}

	movie, err := room.LoadCurrentMovie()
	if err != nil {
		if tracked != nil {
			r.stop(roomID, "current movie unavailable")
		}
		r.logError(room, err)
		return
	}
	if movie.Live || movie.VendorInfo.Vendor != model.VendorEmby || movie.VendorInfo.Emby == nil {
		if tracked != nil {
			r.stop(roomID, "current movie is not an Emby video")
		}
		return
	}

	itemID, err := currentEmbyItemID(movie)
	if err != nil {
		r.logError(room, err)
		return
	}
	movieKey := movie.ID + ":" + itemID
	if tracked != nil && tracked.movieKey != movieKey {
		r.stop(roomID, "current Emby item changed")
		tracked = nil
	}

	if tracked == nil {
		if !current.Status.IsPlaying {
			return
		}

		tracked, err = r.start(room, movie, itemID, current.Status)
		if err != nil {
			r.logError(room, err)
			return
		}
		r.sessions[roomID] = tracked
		delete(r.lastErrorLog, roomID)
		return
	}

	r.progress(room, tracked, current.Status)
}

func (r *embyPlaybackReporter) start(
	room *Room,
	movie *Movie,
	itemID string,
	status model.Status,
) (*trackedEmbyPlayback, error) {
	ctx, cancel := context.WithTimeout(context.Background(), embyPlaybackRequestTimeout)
	defer cancel()

	creatorEntry, err := LoadOrInitUserByID(movie.CreatorID)
	if err != nil {
		return nil, fmt.Errorf("load Emby movie creator: %w", err)
	}
	creator := creatorEntry.Value()

	serverID, _, err := movie.VendorInfo.Emby.ServerIDAndFilePath()
	if err != nil {
		return nil, fmt.Errorf("parse Emby movie path: %w", err)
	}
	authorization, err := creator.EmbyCache().LoadOrStore(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("load Emby authorization: %w", err)
	}

	movieCache, err := movie.EmbyCache().Get(ctx, creator.EmbyCache())
	if err != nil {
		return nil, fmt.Errorf("load Emby playback source: %w", err)
	}
	if movieCache == nil || len(movieCache.Sources) == 0 {
		return nil, errors.New("Emby playback source is empty")
	}

	source := movieCache.Sources[0]
	metadata, err := parseEmbySourceMetadata(source.URL)
	if err != nil {
		return nil, err
	}

	playSessionID := movieCache.TranscodeSessionID
	if playSessionID == "" {
		playSessionID = metadata.playSessionID
	}
	if playSessionID == "" {
		playSessionID = utils.SortUUID()
	}

	deviceID := "synctv-room-" + room.ID
	client := embyplayback.NewClient(
		authorization.Host,
		authorization.APIKey,
		authorization.UserID,
		deviceID,
		"SyncTV · "+room.Name,
		version.Version,
	)

	report := newEmbyPlaybackReport(
		itemID,
		metadata.mediaSourceID,
		playSessionID,
		metadata.audioStreamIndex,
		metadata.subtitleStreamIndex,
		playMethodForSource(source.IsTranscode, metadata.path, metadata.static),
		status,
	)
	if err := client.Started(ctx, report); err != nil {
		return nil, fmt.Errorf("report Emby playback start: %w", err)
	}

	now := time.Now()
	log.WithFields(log.Fields{
		"rid": room.ID,
		"rn":  room.Name,
		"mid": movie.ID,
	}).Info("Emby playback session started")

	return &trackedEmbyPlayback{
		client:       client,
		movieKey:     movie.ID + ":" + itemID,
		report:       report,
		lastStatus:   status,
		lastObserved: now,
		lastReported: now,
	}, nil
}

func (r *embyPlaybackReporter) progress(room *Room, tracked *trackedEmbyPlayback, status model.Status) {
	now := time.Now()
	eventName := embyProgressEvent(tracked.lastStatus, tracked.lastObserved, status, now)
	if eventName == "" && now.Sub(tracked.lastReported) >= embyPlaybackProgressInterval {
		eventName = "TimeUpdate"
	}

	tracked.report.IsPaused = !status.IsPlaying
	tracked.report.PositionTicks = secondsToTicks(status.CurrentTime)
	tracked.report.PlaybackRate = normalizedPlaybackRate(status.PlaybackRate)
	tracked.lastStatus = status
	tracked.lastObserved = now

	if eventName == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), embyPlaybackRequestTimeout)
	defer cancel()
	if err := tracked.client.Progress(ctx, tracked.report, eventName); err != nil {
		r.logError(room, fmt.Errorf("report Emby playback progress: %w", err))
		return
	}

	tracked.lastReported = now
	delete(r.lastErrorLog, room.ID)
}

func (r *embyPlaybackReporter) stop(roomID, reason string) {
	tracked := r.sessions[roomID]
	if tracked == nil {
		return
	}
	delete(r.sessions, roomID)
	delete(r.lastErrorLog, roomID)

	ctx, cancel := context.WithTimeout(context.Background(), embyPlaybackRequestTimeout)
	defer cancel()
	if err := tracked.client.Stopped(ctx, tracked.report); err != nil {
		log.WithField("rid", roomID).Warnf("report Emby playback stop: %v", err)
		return
	}

	log.WithFields(log.Fields{
		"rid":    roomID,
		"reason": reason,
	}).Info("Emby playback session stopped")
}

func (r *embyPlaybackReporter) logError(room *Room, err error) {
	now := time.Now()
	if last, exists := r.lastErrorLog[room.ID]; exists && now.Sub(last) < embyPlaybackErrorLogInterval {
		return
	}
	r.lastErrorLog[room.ID] = now
	log.WithFields(log.Fields{
		"rid": room.ID,
		"rn":  room.Name,
	}).Warnf("Emby playback session reporting failed: %v", err)
}

type embySourceMetadata struct {
	path                string
	mediaSourceID       string
	playSessionID       string
	audioStreamIndex    *int
	subtitleStreamIndex *int
	static              bool
}

func parseEmbySourceMetadata(rawURL string) (embySourceMetadata, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return embySourceMetadata{}, fmt.Errorf("parse Emby playback source URL: %w", err)
	}
	query := parsed.Query()
	return embySourceMetadata{
		path:                parsed.Path,
		mediaSourceID:       query.Get("MediaSourceId"),
		playSessionID:       query.Get("PlaySessionId"),
		audioStreamIndex:    optionalQueryInt(query, "AudioStreamIndex"),
		subtitleStreamIndex: optionalQueryInt(query, "SubtitleStreamIndex"),
		static:              strings.EqualFold(query.Get("Static"), "true"),
	}, nil
}

func optionalQueryInt(query url.Values, key string) *int {
	value, exists := query[key]
	if !exists || len(value) == 0 || value[0] == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value[0])
	if err != nil {
		return nil
	}
	return &parsed
}

func currentEmbyItemID(movie *Movie) (string, error) {
	_, itemID, err := movie.VendorInfo.Emby.ServerIDAndFilePath()
	if err != nil {
		return "", fmt.Errorf("parse Emby item ID: %w", err)
	}
	if movie.IsFolder && movie.SubPath() != "" {
		itemID = movie.SubPath()
	}
	if itemID == "" {
		return "", errors.New("Emby item ID is empty")
	}
	return itemID, nil
}

func playMethodForSource(transcoded bool, path string, static bool) string {
	if transcoded {
		return "Transcode"
	}
	lowerPath := strings.ToLower(path)
	if static || strings.Contains(lowerPath, "/original.") {
		return "DirectPlay"
	}
	return "DirectStream"
}

func newEmbyPlaybackReport(
	itemID,
	mediaSourceID,
	playSessionID string,
	audioStreamIndex,
	subtitleStreamIndex *int,
	playMethod string,
	status model.Status,
) embyplayback.Report {
	return embyplayback.Report{
		QueueableMediaTypes: []string{"Video"},
		CanSeek:             true,
		ItemID:              itemID,
		MediaSourceID:       mediaSourceID,
		AudioStreamIndex:    audioStreamIndex,
		SubtitleStreamIndex: subtitleStreamIndex,
		IsPaused:            !status.IsPlaying,
		IsMuted:             false,
		PositionTicks:       secondsToTicks(status.CurrentTime),
		PlayMethod:          playMethod,
		PlaySessionID:       playSessionID,
		PlaylistIndex:       0,
		PlaylistLength:      1,
		PlaybackRate:        normalizedPlaybackRate(status.PlaybackRate),
	}
}

func embyProgressEvent(previous model.Status, previousAt time.Time, current model.Status, currentAt time.Time) string {
	if previous.IsPlaying != current.IsPlaying {
		if current.IsPlaying {
			return "Unpause"
		}
		return "Pause"
	}
	if math.Abs(normalizedPlaybackRate(previous.PlaybackRate)-normalizedPlaybackRate(current.PlaybackRate)) > 0.001 {
		return "PlaybackRateChange"
	}

	expectedPosition := previous.CurrentTime
	if previous.IsPlaying {
		expectedPosition += currentAt.Sub(previousAt).Seconds() * normalizedPlaybackRate(previous.PlaybackRate)
	}
	if math.Abs(current.CurrentTime-expectedPosition) > embyPlaybackSeekThreshold.Seconds() {
		return "TimeUpdate"
	}
	return ""
}

func normalizedPlaybackRate(rate float64) float64 {
	if rate <= 0 {
		return 1
	}
	return rate
}

func secondsToTicks(seconds float64) int64 {
	if seconds <= 0 {
		return 0
	}
	return int64(math.Round(seconds * 10_000_000))
}
