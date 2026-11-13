package mail

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"ocopion/internal/pkg/postal"
	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type fakeRepository struct {
	calls int
	input SendRequest
	err   error
}

func (r *fakeRepository) Send(to, subject, body string) error {
	r.calls++
	r.input = SendRequest{To: to, Subject: subject, Body: body}
	return r.err
}

func TestServiceSend(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo)
	input := SendRequest{To: " user@example.com ", Subject: "  邮件主题  ", Body: " \n<p>正文</p>\n<p>第二行</p>\n "}
	if err := service.Send(input); err != nil {
		t.Fatal(err)
	}
	want := SendRequest{To: "user@example.com", Subject: "邮件主题", Body: "<p>正文</p>\n<p>第二行</p>"}
	if repo.calls != 1 || repo.input != want {
		t.Fatalf("calls=%d input=%+v", repo.calls, repo.input)
	}

	core, logs := observer.New(zap.ErrorLevel)
	restore := zap.ReplaceGlobals(zap.New(core))
	t.Cleanup(restore)
	repo.err = errors.New("SMTP authentication failed: private configuration")
	if err := service.Send(input); !errors.Is(err, ErrSendFailed) {
		t.Fatalf("unexpected error: %v", err)
	}
	if logs.Len() != 1 || logs.All()[0].ContextMap()["error"] != repo.err.Error() {
		t.Fatal("SMTP error was not logged")
	}
}

func TestControllerSend(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		smtpError  bool
		code       int
		calls      int
	}{
		{"success", `{"to":"user@example.com","subject":"主题","body":"<p>正文</p>"}`, false, 200, 1},
		{"smtp failure", `{"to":"user@example.com","subject":"主题","body":"<p>正文</p>"}`, true, -1, 1},
		{"missing fields", `{}`, false, -1, 0},
		{"missing to", `{"subject":"s","body":"b"}`, false, -1, 0},
		{"missing subject", `{"to":"user@example.com","body":"b"}`, false, -1, 0},
		{"missing body", `{"to":"user@example.com","subject":"s"}`, false, -1, 0},
		{"blank to", `{"to":" \t ","subject":"s","body":"b"}`, false, -1, 0},
		{"blank subject", `{"to":"user@example.com","subject":" \t ","body":"b"}`, false, -1, 0},
		{"blank body", `{"to":"user@example.com","subject":"s","body":" \n\t "}`, false, -1, 0},
		{"invalid email", `{"to":"invalid","subject":"s","body":"b"}`, false, -1, 0},
		{"multiple recipients", `{"to":"a@example.com,b@example.com","subject":"s","body":"b"}`, false, -1, 0},
		{"semicolon recipients", `{"to":"a@example.com;b@example.com","subject":"s","body":"b"}`, false, -1, 0},
		{"display name", `{"to":"User <user@example.com>","subject":"s","body":"b"}`, false, -1, 0},
		{"to injection", `{"to":"user@example.com\r\nBcc: a@example.com","subject":"s","body":"b"}`, false, -1, 0},
		{"subject injection", `{"to":"user@example.com","subject":"s\r\nBcc: a@example.com","body":"b"}`, false, -1, 0},
		{"trailing to newline", `{"to":"user@example.com\n","subject":"s","body":"b"}`, false, -1, 0},
		{"leading subject newline", `{"to":"user@example.com","subject":"\rs","body":"b"}`, false, -1, 0},
		{"wrong to type", `{"to":123,"subject":"s","body":"b"}`, false, -1, 0},
		{"wrong subject type", `{"to":"user@example.com","subject":[],"body":"b"}`, false, -1, 0},
		{"wrong body type", `{"to":"user@example.com","subject":"s","body":{}}`, false, -1, 0},
		{"malformed JSON", `{"to":`, false, -1, 0},
		{"null", `null`, false, -1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{}
			if tc.smtpError {
				repo.err = errors.New("private SMTP error")
			}
			r := gin.New()
			r.POST("/mail", NewController(NewService(repo)).Send)
			req := httptest.NewRequest("POST", "/mail", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			var result response.Response
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || result.Code != tc.code || repo.calls != tc.calls {
				t.Fatalf("status=%d response=%s calls=%d", w.Code, w.Body.String(), repo.calls)
			}
			if tc.smtpError && result.Message != "邮件发送失败" {
				t.Fatal(w.Body.String())
			}
			if tc.code == 200 && result.Data != nil {
				t.Fatal("unexpected business data")
			}
		})
	}
}

func TestPostalRepository(t *testing.T) {
	want := SendRequest{To: "user@example.com", Subject: "主题", Body: "<p>正文</p>"}
	smtpErr := errors.New("SMTP unavailable")
	calls := 0
	restore := postal.SetSendMailImplForTest(func(to, subject, body string) error {
		calls++
		if (SendRequest{To: to, Subject: subject, Body: body}) != want {
			t.Fatal("unexpected fields")
		}
		return smtpErr
	})
	t.Cleanup(restore)
	if err := NewPostalRepository().Send(want.To, want.Subject, want.Body); !errors.Is(err, smtpErr) || calls != 1 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}
