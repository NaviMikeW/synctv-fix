package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/synctv-org/synctv/internal/model"
	"github.com/synctv-org/synctv/internal/op"
)

type errorReader struct {
	err error
}

func (r errorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func TestReadWebSocketPayloadAcceptsLimit(t *testing.T) {
	got, err := readWebSocketPayload(strings.NewReader("1234"), 4)
	if err != nil {
		t.Fatalf("readWebSocketPayload() error = %v", err)
	}
	if string(got) != "1234" {
		t.Fatalf("readWebSocketPayload() = %q, want %q", got, "1234")
	}
}

func TestReadWebSocketPayloadRejectsLimitPlusOne(t *testing.T) {
	_, err := readWebSocketPayload(strings.NewReader("12345"), 4)
	if !errors.Is(err, ErrWebSocketMessageTooLarge) {
		t.Fatalf(
			"readWebSocketPayload() error = %v, want ErrWebSocketMessageTooLarge",
			err,
		)
	}
}

func TestReadWebSocketPayloadNormalizesConnectionLimitError(t *testing.T) {
	_, err := readWebSocketPayload(errorReader{err: websocket.ErrReadLimit}, 4)
	if !errors.Is(err, ErrWebSocketMessageTooLarge) {
		t.Fatalf(
			"readWebSocketPayload() error = %v, want ErrWebSocketMessageTooLarge",
			err,
		)
	}
}

func TestReadWebSocketPayloadPreservesReadError(t *testing.T) {
	want := errors.New("read failed")
	_, err := readWebSocketPayload(errorReader{err: want}, 4)
	if !errors.Is(err, want) {
		t.Fatalf("readWebSocketPayload() error = %v, want %v", err, want)
	}
}

func TestPrepareChatMessageLimitsRawTextBeforeEscaping(t *testing.T) {
	raw := strings.Repeat("<", MaxChatMessageLength)
	got, err := prepareChatMessage(raw)
	if err != nil {
		t.Fatalf("prepareChatMessage() error = %v", err)
	}
	if got != strings.Repeat("&lt;", MaxChatMessageLength) {
		t.Fatal("prepareChatMessage() did not preserve server-side HTML escaping")
	}

	if _, err = prepareChatMessage(raw + "<"); err == nil ||
		err.Error() != "message too long" {
		t.Fatalf("oversized prepareChatMessage() error = %v", err)
	}
}

func TestOversizedWebSocketMessageDisconnectsAndUnregistersClient(t *testing.T) {
	user := &op.User{User: model.User{
		ID:       "test-user",
		Username: "test-user",
		Role:     model.RoleUser,
	}}
	room := &op.Room{Room: model.Room{ID: "test-room"}}
	handlerDone := make(chan error, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		}).Upgrade(w, r, nil)
		if err != nil {
			handlerDone <- err
			return
		}
		defer conn.Close()

		handlerDone <- NewWSMessageHandler(
			user,
			room,
			log.New().WithField("test", "oversized-websocket"),
		)(conn)
	}))
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"),
		nil,
	)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer client.Close()

	_ = client.WriteMessage(
		websocket.BinaryMessage,
		make([]byte, MaxWebSocketMessageSize+1),
	)

	select {
	case <-handlerDone:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket handler did not return after an oversized message")
	}

	if got := room.ViewerCount(); got != 0 {
		t.Fatalf("viewer count after oversized message = %d, want 0", got)
	}
}
