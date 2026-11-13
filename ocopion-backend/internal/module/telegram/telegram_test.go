package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"ocopion/internal/pkg/response"
	client "ocopion/internal/pkg/telegram"
)

type fakeRepository struct {
	calls int
	text  string
	ctx   context.Context
	err   error
}

func (r *fakeRepository) Send(ctx context.Context, text string) error {
	r.calls++
	r.ctx, r.text = ctx, text
	return r.err
}

func TestControllerSend(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		fail       bool
	}{
		{"success", `{"text":" \n第一行\n第二行\t "}`, false},
		{"upstream failure", `{"text":"hello"}`, true},
		{"missing", `{}`, false},
		{"blank", `{"text":" \n\t "}`, false},
		{"number", `{"text":123}`, false},
		{"array", `{"text":[]}`, false},
		{"object", `{"text":{}}`, false},
		{"boolean", `{"text":true}`, false},
		{"null text", `{"text":null}`, false},
		{"null body", `null`, false},
		{"array body", `[]`, false},
		{"invalid JSON", `{"text":`, false},
		{"empty body", ``, false},
		{"trailing JSON", `{"text":"hello"} {}`, false},
		{"trailing garbage", `{"text":"hello"} x`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{}
			if tc.fail {
				repo.err = errors.New("private token and upstream error")
			}
			core, logs := observer.New(zap.ErrorLevel)
			restore := zap.ReplaceGlobals(zap.New(core))
			t.Cleanup(restore)
			r := gin.New()
			r.POST("/telegram", NewController(NewService(repo)).Send)
			req := httptest.NewRequest("POST", "/telegram", strings.NewReader(tc.body))
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			var result response.Response
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			wantCode, wantMessage, wantCalls := -1, ErrInvalidInput.Error(), 0
			if tc.name == "success" {
				wantCode, wantMessage, wantCalls = 200, "操作成功", 1
			}
			if tc.fail {
				wantMessage, wantCalls = ErrSendFailed.Error(), 1
			}
			if w.Code != 200 || result.Code != wantCode || result.Message != wantMessage || repo.calls != wantCalls || result.Data != nil {
				t.Fatalf("response=%s calls=%d", w.Body.String(), repo.calls)
			}
			if wantCalls > 0 && repo.ctx != ctx {
				t.Fatal("request context lost")
			}
			if tc.name == "success" && repo.text != "第一行\n第二行" {
				t.Fatalf("text=%q", repo.text)
			}
			if tc.fail && (logs.Len() != 1 || logs.All()[0].ContextMap()["error"] != repo.err.Error()) {
				t.Fatal("send failure not logged")
			}
		})
	}
}

func TestServiceRejectsBlank(t *testing.T) {
	repo := &fakeRepository{}
	for _, text := range []string{"", " \t\r\n", "　"} {
		if err := NewService(repo).Send(context.Background(), SendRequest{Text: text}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal(err)
		}
	}
	if repo.calls != 0 {
		t.Fatal("invalid text sent")
	}
}

func TestClientRepository(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.ChatID != "fixed-chat" || body.Text != "hello\nworld" {
			t.Errorf("body=%+v", body)
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	repo := NewClientRepository(client.NewClient("token", "fixed-chat", server.URL))
	if err := repo.Send(context.Background(), "hello\nworld"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := repo.Send(ctx, "hello"); err == nil {
		t.Fatal("canceled context ignored")
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	for _, cfg := range [][2]string{{"", "chat"}, {"token", ""}} {
		s := NewService(NewClientRepository(client.NewClient(cfg[0], cfg[1], server.URL)))
		if err := s.Send(context.Background(), SendRequest{Text: "hello"}); !errors.Is(err, ErrSendFailed) {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("missing configuration triggered request")
	}
}
