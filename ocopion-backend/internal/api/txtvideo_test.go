package router

import (
	"net/http/httptest"
	"testing"

	"ocopion/internal/config"
)

func TestTxtVideoRoutesRequireLogin(t *testing.T) {
	old := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = old })
	config.GlobalConfig = &config.Config{}
	r := newTestRouter(t, nil, nil, nil, nil, nil, nil, nil)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"POST", "/api/txt-video/tasks", 401},
		{"GET", "/api/txt-video/tasks/missing", 401},
		{"GET", "/api/txt-video/files/invalid.mp4", 404},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.want {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
