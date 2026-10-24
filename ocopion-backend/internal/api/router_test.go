package router

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ocopion/internal/config"
	"ocopion/internal/module/app"
	"ocopion/internal/module/auth"
	"ocopion/internal/module/post"
	"ocopion/internal/module/user"
	"ocopion/internal/module/video"
	jwtpkg "ocopion/internal/pkg/jwt"
	"ocopion/internal/pkg/pagination"
	"ocopion/internal/pkg/response"
)

type postRepository struct{ calls int }

func (r *postRepository) Create(_ context.Context, p *post.Post) error {
	r.calls++
	p.ID = 1
	return nil
}
func (r *postRepository) Get(_ context.Context, id int64) (*post.Post, error) {
	r.calls++
	return &post.Post{ID: id}, nil
}
func (r *postRepository) Update(_ context.Context, id int64, _ map[string]interface{}) (*post.Post, error) {
	r.calls++
	return &post.Post{ID: id}, nil
}
func (r *postRepository) Delete(context.Context, []int64) error { r.calls++; return nil }
func (r *postRepository) List(context.Context, pagination.Params, string) ([]post.Post, int64, error) {
	r.calls++
	return []post.Post{}, 0, nil
}

type userRepository struct {
	user.Repository
	calls int
}

func (r *userRepository) Get(_ context.Context, id int64) (*user.User, error) {
	r.calls++
	return &user.User{ID: id, Status: "active"}, nil
}
func (r *userRepository) List(context.Context, pagination.Params, string) ([]user.User, int64, error) {
	r.calls++
	return []user.User{}, 0, nil
}

type videoRepository struct{}

func (videoRepository) GetVideoList() ([]video.VideoConfig, error) { return []video.VideoConfig{}, nil }

func TestRouterAuthentication(t *testing.T) {
	t.Setenv("OCOPION_ENV", "dev")
	t.Chdir(t.TempDir())
	if err := os.Mkdir("public", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("public/test.txt", []byte("public asset"), 0644); err != nil {
		t.Fatal(err)
	}
	previous := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previous })
	config.GlobalConfig = &config.Config{JWT: config.JWTConfig{Secret: "test-secret"}, Auth: config.AuthConfig{Whitelist: []string{"/api/post", "GET:/api/post/:id"}}}
	pr, ur := &postRepository{}, &userRepository{}
	r := newRouter(post.NewController(post.NewService(pr)), user.NewController(user.NewService(ur)), auth.NewAuthController(auth.NewAuthService(ur)), app.NewController(), video.NewVideoController(video.NewVideoService(videoRepository{}, config.GetConfig())))
	// This identity has no repository user lookup for post authorization.
	access, refresh, err := jwtpkg.GenToken(999)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range r.Routes() {
		if !strings.HasPrefix(route.Path, "/api/post") {
			continue
		}
		path := strings.ReplaceAll(route.Path, ":id", "1")
		body := `{"title":"test","content":"test","ids":[1]}`
		for _, token := range []string{"", refresh, access} {
			req := httptest.NewRequest(route.Method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			before := pr.calls
			r.ServeHTTP(w, req)
			var result response.Response
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if token == access {
				if w.Code != 200 || result.Code != 200 || pr.calls != before+1 {
					t.Fatalf("authorized %s %s: %s", route.Method, path, w.Body.String())
				}
			} else if w.Code != 401 || result.Code != 1006 || pr.calls != before {
				t.Fatalf("unprotected %s %s: %s", route.Method, path, w.Body.String())
			}
		}
	}
	if ur.calls != 0 {
		t.Fatal("post authorization looked up user")
	}
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/auth/login", `{}`}, {"POST", "/api/auth/refresh", `{}`},
		{"GET", "/api/meta", ""},
		{"GET", "/api/cloudflare/fast-ip", ""}, {"GET", "/public/test.txt", ""},
		{"GET", "/api/video", ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("public route %s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	// User CRUD requires an access token, while validation remains in the controller.
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{
		{"POST", "/api/user", `{}`, -1},
		{"PUT", "/api/user/1", `{}`, -1},
		{"POST", "/api/user/delete", `{}`, -1},
		{"GET", "/api/user/1", "", 200},
		{"GET", "/api/user", "", 200},
	} {
		for _, token := range []string{"", refresh, access} {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			before := ur.calls
			r.ServeHTTP(w, req)
			var result response.Response
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if token == access {
				if w.Code != 200 || result.Code != tc.code {
					t.Fatalf("authorized %s %s: %s", tc.method, tc.path, w.Body.String())
				}
			} else if w.Code != 401 || result.Code != 1006 || ur.calls != before {
				t.Fatalf("unprotected %s %s: %s", tc.method, tc.path, w.Body.String())
			}
		}
	}
	for _, token := range []string{"", access} {
		req := httptest.NewRequest("GET", "/api/user/me?id=1", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if token == "" {
			if w.Code != 401 {
				t.Fatal(w.Body.String())
			}
		} else {
			var result struct {
				Code int               `json:"code"`
				Data user.UserResponse `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || result.Data.ID != 999 {
				t.Fatal(w.Body.String())
			}
		}
	}
}
