package post

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"ocopion/internal/pkg/pagination"
	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type memoryRepository struct {
	posts map[int64]Post
	next  int64
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{posts: make(map[int64]Post)}
}

func (r *memoryRepository) Create(_ context.Context, post *Post) error {
	r.next++
	post.ID = r.next
	r.posts[post.ID] = *post
	return nil
}

func (r *memoryRepository) Get(_ context.Context, id int64) (*Post, error) {
	post, ok := r.posts[id]
	if !ok || post.DeletedAt.Valid {
		return nil, gorm.ErrRecordNotFound
	}
	return &post, nil
}

func (r *memoryRepository) Update(ctx context.Context, id int64, fields map[string]interface{}) (*Post, error) {
	post, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if title, ok := fields["title"]; ok {
		post.Title = title.(string)
	}
	if content, ok := fields["content"]; ok {
		post.Content = content.(string)
	}
	r.posts[id] = *post
	return post, nil
}

func (r *memoryRepository) Delete(_ context.Context, ids []int64) error {
	for _, id := range ids {
		if post, ok := r.posts[id]; ok {
			post.DeletedAt.Valid = true
			r.posts[id] = post
		}
	}
	return nil
}

func (r *memoryRepository) List(_ context.Context, page pagination.Params, keyword string) ([]Post, int64, error) {
	var all []Post
	for _, post := range r.posts {
		if !post.DeletedAt.Valid && (keyword == "" || strings.Contains(post.Title, keyword) || strings.Contains(post.Content, keyword)) {
			all = append(all, post)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := int64(len(all))
	if page.Offset() >= len(all) {
		return []Post{}, total, nil
	}
	end := page.Offset() + page.PageSize
	if end > len(all) {
		end = len(all)
	}
	return all[page.Offset():end], total, nil
}

func testRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	ctl := NewController(NewService(newMemoryRepository()))
	r := gin.New()
	r.POST("/api/post", ctl.Create)
	r.PUT("/api/post/:id", ctl.Update)
	r.GET("/api/post/:id", ctl.Get)
	r.GET("/api/post", ctl.List)
	r.POST("/api/post/delete", ctl.Delete)
	return r
}

func request(t *testing.T, r http.Handler, method, path, body string) response.Response {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%s %s: status %d", method, path, w.Code)
	}
	var result response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func dataMap(t *testing.T, result response.Response) map[string]interface{} {
	t.Helper()
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data: %#v", result.Data)
	}
	return data
}

func TestPostLifecycle(t *testing.T) {
	r := testRouter()
	created := request(t, r, "POST", "/api/post", `{"title":"第一篇","content":"hello keyword"}`)
	if created.Code != response.CodeSuccess || dataMap(t, created)["id"] != float64(1) {
		t.Fatalf("create: %#v", created)
	}
	request(t, r, "POST", "/api/post", `{"title":"第二篇","content":"other"}`)
	contentUpdated := request(t, r, "PUT", "/api/post/2", `{"content":"revised"}`)
	if dataMap(t, contentUpdated)["title"] != "第二篇" || dataMap(t, contentUpdated)["content"] != "revised" {
		t.Fatalf("content-only update: %#v", contentUpdated)
	}

	updated := request(t, r, "PUT", "/api/post/1", `{"title":"新标题"}`)
	if dataMap(t, updated)["content"] != "hello keyword" || dataMap(t, updated)["title"] != "新标题" {
		t.Fatalf("partial update: %#v", updated)
	}
	listed := dataMap(t, request(t, r, "GET", "/api/post?pageNum=1&pageSize=1", ""))
	if listed["total"] != float64(2) || listed["pageSize"] != float64(1) || listed["list"].([]interface{})[0].(map[string]interface{})["id"] != float64(2) {
		t.Fatalf("page: %#v", listed)
	}
	filtered := dataMap(t, request(t, r, "GET", "/api/post?keyword=keyword", ""))
	if filtered["total"] != float64(1) {
		t.Fatalf("keyword: %#v", filtered)
	}
	emptyPage := dataMap(t, request(t, r, "GET", "/api/post?pageNum=10&pageSize=1", ""))
	if emptyPage["total"] != float64(2) || len(emptyPage["list"].([]interface{})) != 0 {
		t.Fatalf("out-of-range page: %#v", emptyPage)
	}
	deleted := request(t, r, "POST", "/api/post/delete", `{"ids":[1,999]}`)
	if deleted.Code != response.CodeSuccess || request(t, r, "POST", "/api/post/delete", `{"ids":[1]}`).Code != response.CodeSuccess {
		t.Fatalf("idempotent delete: %#v", deleted)
	}
	if request(t, r, "GET", "/api/post/1", "").Code != response.CodeFail || request(t, r, "PUT", "/api/post/1", `{"content":"x"}`).Code != response.CodeFail {
		t.Fatal("deleted post must be invisible")
	}
	if dataMap(t, request(t, r, "GET", "/api/post", ""))["total"] != float64(1) {
		t.Fatal("deleted post counted")
	}
}

func TestInvalidInputAndPagination(t *testing.T) {
	r := testRouter()
	invalid := []struct{ method, path, body string }{
		{"POST", "/api/post", `{"title":" ","content":"body"}`},
		{"POST", "/api/post", `{"title":"title","content":" "}`},
		{"POST", "/api/post", `{"title":"` + strings.Repeat("中", 129) + `","content":"body"}`},
		{"POST", "/api/post", `{`},
		{"PUT", "/api/post/1", `{}`},
		{"PUT", "/api/post/1", `{"title":""}`},
		{"GET", "/api/post/0", ""},
		{"GET", "/api/post/abc", ""},
		{"POST", "/api/post/delete", `{"ids":[]}`},
		{"POST", "/api/post/delete", `{"ids":[0]}`},
		{"POST", "/api/post/delete", `{"ids":[1,"x"]}`},
	}
	for _, tc := range invalid {
		if got := request(t, r, tc.method, tc.path, tc.body); got.Code != response.CodeFail {
			t.Errorf("%s %s %s: %#v", tc.method, tc.path, tc.body, got)
		}
	}
	if got := request(t, r, "GET", "/api/post/999", ""); got.Code != response.CodeFail || got.Message != "文章不存在" {
		t.Fatalf("missing post: %#v", got)
	}
	page := dataMap(t, request(t, r, "GET", "/api/post?pageNum=-3&pageSize=1000", ""))
	if page["pageNum"] != float64(1) || page["pageSize"] != float64(100) || page["total"] != float64(0) {
		t.Fatalf("pagination: %#v", page)
	}
	defaultPage := dataMap(t, request(t, r, "GET", "/api/post?pageNum=no&pageSize=0", ""))
	if defaultPage["pageNum"] != float64(1) || defaultPage["pageSize"] != float64(10) {
		t.Fatalf("default pagination: %#v", defaultPage)
	}
}
