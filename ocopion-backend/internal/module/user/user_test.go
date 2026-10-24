package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"net/url"
	"ocopion/internal/pkg/pagination"
	"ocopion/internal/pkg/password"
	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type memoryRepository struct {
	users map[int64]User
	next  int64
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{users: make(map[int64]User)}
}

func (r *memoryRepository) Create(_ context.Context, user *User) error {
	for _, existing := range r.users {
		if existing.Email == user.Email {
			return ErrEmailExists
		}
	}
	r.next++
	user.ID = r.next
	r.users[user.ID] = *user
	return nil
}

func (r *memoryRepository) Get(_ context.Context, id int64) (*User, error) {
	user, ok := r.users[id]
	if !ok || user.DeletedAt.Valid {
		return nil, gorm.ErrRecordNotFound
	}
	return &user, nil
}

func (r *memoryRepository) Update(ctx context.Context, id int64, fields map[string]interface{}) (*User, error) {
	user, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if email, ok := fields["email"]; ok {
		for otherID, existing := range r.users {
			if otherID != id && existing.Email == email.(string) {
				return nil, ErrEmailExists
			}
		}
		user.Email = email.(string)
	}
	for name, value := range fields {
		switch name {
		case "phone":
			user.Phone = value.(string)
		case "password_hash":
			user.PasswordHash = value.(string)
		case "username":
			user.Username = value.(string)
		case "nickname":
			user.Nickname = value.(string)
		case "avatar_url":
			user.AvatarURL = value.(string)
		case "status":
			user.Status = value.(string)
		}
	}
	r.users[id] = *user
	return user, nil
}

func (r *memoryRepository) Delete(_ context.Context, ids []int64) error {
	for _, id := range ids {
		if user, ok := r.users[id]; ok {
			user.DeletedAt.Valid = true
			r.users[id] = user
		}
	}
	return nil
}

func (r *memoryRepository) List(_ context.Context, page pagination.Params, keyword string) ([]User, int64, error) {
	var all []User
	for _, user := range r.users {
		if !user.DeletedAt.Valid && (keyword == "" || strings.Contains(user.Email, keyword) || strings.Contains(user.Phone, keyword) || strings.Contains(user.Username, keyword) || strings.Contains(user.Nickname, keyword)) {
			all = append(all, user)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	total := int64(len(all))
	if page.Offset() >= len(all) {
		return []User{}, total, nil
	}
	end := page.Offset() + page.PageSize
	if end > len(all) {
		end = len(all)
	}
	return all[page.Offset():end], total, nil
}

func testRouter() (*gin.Engine, *memoryRepository) {
	gin.SetMode(gin.TestMode)
	repo := newMemoryRepository()
	ctl := NewController(NewService(repo))
	r := gin.New()
	r.POST("/api/user", ctl.Create)
	r.PUT("/api/user/:id", ctl.Update)
	r.GET("/api/user/:id", ctl.Get)
	r.GET("/api/user", ctl.List)
	r.POST("/api/user/delete", ctl.Delete)
	return r, repo
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
	if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "$2a$") {
		t.Fatalf("password leaked: %s", w.Body.String())
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

func TestUserLifecycle(t *testing.T) {
	r, repo := testRouter()
	created := request(t, r, "POST", "/api/user", `{"email":"one@example.com","password":"secret","username":"first","nickname":"第一位","phone":"12345","avatarUrl":"https://example.com/avatar","id":999,"passwordHash":"injected"}`)
	data := dataMap(t, created)
	if created.Code != response.CodeSuccess || data["id"] != float64(1) || data["status"] != "active" || len(data) != 9 {
		t.Fatalf("create: %#v", created)
	}
	originalHash := repo.users[1].PasswordHash
	if originalHash == "secret" || password.Compare(originalHash, "secret") != nil {
		t.Fatal("password not hashed")
	}
	fetched := dataMap(t, request(t, r, "GET", "/api/user/1", ""))
	if fetched["email"] != "one@example.com" {
		t.Fatalf("get: %#v", fetched)
	}
	second := request(t, r, "POST", "/api/user", `{"email":"two@example.com","password":"second-secret","username":"second","nickname":"第二位","status":"custom"}`)
	if second.Code != response.CodeSuccess || dataMap(t, second)["phone"] != "" || dataMap(t, second)["avatarUrl"] != "" || dataMap(t, second)["status"] != "custom" {
		t.Fatalf("create defaults: %#v", second)
	}
	updated := dataMap(t, request(t, r, "PUT", "/api/user/1", `{"nickname":"新昵称","phone":"","avatarUrl":"","id":2,"passwordHash":"bad","createdAt":"2000-01-01T00:00:00Z"}`))
	if updated["nickname"] != "新昵称" || updated["phone"] != "" || updated["avatarUrl"] != "" || updated["email"] != "one@example.com" || updated["username"] != "first" || updated["id"] != float64(1) || repo.users[1].PasswordHash != originalHash {
		t.Fatalf("partial update: %#v", updated)
	}
	changed := request(t, r, "PUT", "/api/user/1", `{"email":"new@example.com","password":"new-secret","username":"new-first","phone":"6789","status":"disabled"}`)
	if changed.Code != response.CodeSuccess || repo.users[1].PasswordHash == originalHash || password.Compare(repo.users[1].PasswordHash, "new-secret") != nil {
		t.Fatalf("password update: %#v", changed)
	}
	if dataMap(t, changed)["status"] != "disabled" {
		t.Fatal("status not updated")
	}
	listed := dataMap(t, request(t, r, "GET", "/api/user?pageNum=1&pageSize=1", ""))
	if listed["total"] != float64(2) || listed["list"].([]interface{})[0].(map[string]interface{})["id"] != float64(2) {
		t.Fatalf("page/order: %#v", listed)
	}
	next := dataMap(t, request(t, r, "GET", "/api/user?pageNum=2&pageSize=1", ""))
	if next["list"].([]interface{})[0].(map[string]interface{})["id"] != float64(1) {
		t.Fatal("second page")
	}
	for _, keyword := range []string{"new@example.com", "6789", "new-first", "新昵称"} {
		result := dataMap(t, request(t, r, "GET", "/api/user?keyword="+url.QueryEscape(" "+keyword+" "), ""))
		if result["total"] != float64(1) {
			t.Fatalf("keyword %s: %#v", keyword, result)
		}
	}
	empty := dataMap(t, request(t, r, "GET", "/api/user?pageNum=99&pageSize=1", ""))
	if empty["total"] != float64(2) || len(empty["list"].([]interface{})) != 0 {
		t.Fatalf("empty page: %#v", empty)
	}
	for i := 0; i < 2; i++ {
		if result := request(t, r, "POST", "/api/user/delete", `{"ids":[1,999]}`); result.Code != response.CodeSuccess || result.Data != nil {
			t.Fatalf("delete: %#v", result)
		}
	}
	if !repo.users[1].DeletedAt.Valid {
		t.Fatal("not soft deleted")
	}
	for _, method := range []string{"GET", "PUT"} {
		body := ""
		if method == "PUT" {
			body = `{"nickname":"hidden"}`
		}
		if result := request(t, r, method, "/api/user/1", body); result.Message != "用户不存在" {
			t.Fatalf("deleted user visible: %#v", result)
		}
	}
	if result := dataMap(t, request(t, r, "GET", "/api/user?keyword=new@example.com", "")); result["total"] != float64(0) {
		t.Fatal("deleted user found")
	}
	duplicate := request(t, r, "POST", "/api/user", `{"email":"new@example.com","password":"secret","username":"again","nickname":"again"}`)
	if duplicate.Message != "邮箱已存在" {
		t.Fatalf("deleted email reused: %#v", duplicate)
	}
}

func TestInvalidInputAndPagination(t *testing.T) {
	r, _ := testRouter()
	base := map[string]interface{}{"email": "valid@example.com", "password": "secret", "username": "user", "nickname": "用户"}
	for _, field := range []string{"email", "password", "username", "nickname"} {
		input := make(map[string]interface{})
		for k, v := range base {
			input[k] = v
		}
		delete(input, field)
		body, _ := json.Marshal(input)
		if got := request(t, r, "POST", "/api/user", string(body)); got.Message != "参数不合法" {
			t.Errorf("missing %s: %#v", field, got)
		}
	}
	cases := []struct {
		field string
		value string
	}{
		{"email", "bad"}, {"email", "Name <user@example.com>"}, {"email", " user@example.com"},
		{"email", strings.Repeat("a", 117) + "@example.com"},
		{"password", ""}, {"password", " "}, {"password", strings.Repeat("a", 73)}, {"password", strings.Repeat("中", 25)},
		{"username", " "}, {"username", strings.Repeat("中", 129)},
		{"nickname", ""}, {"nickname", strings.Repeat("中", 129)},
		{"phone", strings.Repeat("1", 33)}, {"avatarUrl", strings.Repeat("a", 513)},
		{"status", ""}, {"status", strings.Repeat("a", 33)},
	}
	for _, tc := range cases {
		t.Run(tc.field+fmt.Sprint(len(tc.value)), func(t *testing.T) {
			input := make(map[string]interface{})
			for k, v := range base {
				input[k] = v
			}
			input[tc.field] = tc.value
			body, _ := json.Marshal(input)
			if got := request(t, r, "POST", "/api/user", string(body)); got.Message != "参数不合法" {
				t.Errorf("create: %#v", got)
			}
			body, _ = json.Marshal(map[string]string{tc.field: tc.value})
			if got := request(t, r, "PUT", "/api/user/1", string(body)); got.Message != "参数不合法" {
				t.Errorf("update: %#v", got)
			}
		})
	}
	invalid := []struct{ method, path, body string }{
		{"POST", "/api/user", "{"}, {"POST", "/api/user", `{"email":1}`},
		{"PUT", "/api/user/1", `{}`}, {"PUT", "/api/user/1", `{"nickname":null}`}, {"PUT", "/api/user/1", `{"id":2,"passwordHash":"bad"}`},
		{"PUT", "/api/user/1", "{"}, {"PUT", "/api/user/0", `{"nickname":"ok"}`},
		{"GET", "/api/user/abc", ""}, {"GET", "/api/user/0", ""}, {"GET", "/api/user/-1", ""}, {"GET", "/api/user/9223372036854775808", ""},
		{"POST", "/api/user/delete", `{}`}, {"POST", "/api/user/delete", `{"ids":[]}`}, {"POST", "/api/user/delete", `{"ids":[0]}`},
		{"POST", "/api/user/delete", `{"ids":[-1]}`}, {"POST", "/api/user/delete", `{"ids":[1,"x"]}`}, {"POST", "/api/user/delete", "{"},
	}
	for _, tc := range invalid {
		if got := request(t, r, tc.method, tc.path, tc.body); got.Message != "参数不合法" {
			t.Errorf("%s %s %s: %#v", tc.method, tc.path, tc.body, got)
		}
	}
	for _, tc := range []struct{ method, body string }{{"GET", ""}, {"PUT", `{"nickname":"missing"}`}} {
		if got := request(t, r, tc.method, "/api/user/999", tc.body); got.Message != "用户不存在" {
			t.Errorf("missing: %#v", got)
		}
	}
	for _, tc := range []struct {
		query string
		size  float64
	}{{"pageNum=-3&pageSize=1000", 100}, {"pageNum=no&pageSize=0", 10}, {"pageNum=0&pageSize=-1", 10}, {"pageSize=no", 10}} {
		page := dataMap(t, request(t, r, "GET", "/api/user?"+tc.query, ""))
		if page["pageNum"] != float64(1) || page["pageSize"] != tc.size || page["total"] != float64(0) || len(page["list"].([]interface{})) != 0 {
			t.Fatalf("pagination: %#v", page)
		}
	}
}

func TestDuplicateEmail(t *testing.T) {
	r, _ := testRouter()
	for _, email := range []string{"one@example.com", "two@example.com"} {
		if got := request(t, r, "POST", "/api/user", fmt.Sprintf(`{"email":%q,"password":"secret","username":"name","nickname":"name"}`, email)); got.Code != response.CodeSuccess {
			t.Fatal(got)
		}
	}
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/user", `{"email":"one@example.com","password":"secret","username":"name","nickname":"name"}`},
		{"PUT", "/api/user/2", `{"email":"one@example.com"}`},
	} {
		if got := request(t, r, tc.method, tc.path, tc.body); got.Message != "邮箱已存在" {
			t.Fatal(got)
		}
	}
	if got := request(t, r, "PUT", "/api/user/1", `{"email":"one@example.com"}`); got.Code != response.CodeSuccess {
		t.Fatal(got)
	}
}

func TestDatabaseErrorTranslation(t *testing.T) {
	for _, err := range []error{gorm.ErrDuplicatedKey, &mysql.MySQLError{Number: 1062}, fmt.Errorf("wrapped: %w", &mysql.MySQLError{Number: 1062})} {
		if !errors.Is(translateError(err), ErrEmailExists) {
			t.Fatalf("duplicate not translated: %v", err)
		}
	}
	other := &mysql.MySQLError{Number: 1045}
	if translateError(other) != other || translateError(nil) != nil {
		t.Fatal("unrelated error translated")
	}
}

func TestPasswordByteBoundary(t *testing.T) {
	s := NewService(newMemoryRepository())
	for _, raw := range []string{strings.Repeat("a", 72), strings.Repeat("中", 24)} {
		user, err := s.Create(context.Background(), CreateRequest{Email: fmt.Sprintf("user%d@example.com", len(raw)+int(raw[0])), Password: raw, Username: strings.Repeat("中", 128), Nickname: "boundary", Phone: strings.Repeat("1", 32), AvatarURL: strings.Repeat("a", 512)})
		if err != nil {
			t.Fatal(err)
		}
		if password.Compare(user.PasswordHash, raw) != nil {
			t.Fatal("boundary password invalid")
		}
	}
}
