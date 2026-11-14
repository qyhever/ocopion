package router

import (
	"net/http/httptest"
	"ocopion/internal/config"
	jwtpkg "ocopion/internal/pkg/jwt"
	"strings"
	"testing"
)

func TestAttachRoutesRequireLogin(t *testing.T) {
	old := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = old })
	config.GlobalConfig = &config.Config{JWT: config.JWTConfig{Secret: "attach-router"}}
	token, _, e := jwtpkg.GenToken(1)
	if e != nil {
		t.Fatal(e)
	}
	r := newTestRouter(t, nil, nil, nil, nil, nil, nil, nil)
	for _, path := range []string{"upload", "verify", "chunk", "merge"} {
		for _, auth := range []string{"", token} {
			req := httptest.NewRequest("POST", "/api/attach/"+path, strings.NewReader("{}"))
			req.Header.Set("Authorization", "Bearer "+auth)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if auth == "" && w.Code != 401 {
				t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
			}
			if auth != "" && (w.Code != 200 || !strings.Contains(w.Body.String(), "\"code\":-1")) {
				t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
			}
		}
	}
}
