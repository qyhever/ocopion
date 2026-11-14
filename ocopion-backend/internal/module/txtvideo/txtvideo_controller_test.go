package txtvideo

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"ocopion/internal/middleware"
)

func TestImageFormats(t *testing.T) {
	var jpegImage bytes.Buffer
	if err := jpeg.Encode(&jpegImage, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	webpImage, err := os.ReadFile("testdata/cover.webp")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"jpeg": jpegImage.Bytes(), "png": pngCover(t), "webp": webpImage} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := validate("你好", "1", data); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestControllerSubmissionAndIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := NewService(testRepo(t, &fakeRunner{}))
	ctl := NewController(s)
	r := gin.New()
	r.POST("/tasks", func(c *gin.Context) { c.Set(middleware.ContextUserIDKey, uint64(1)); ctl.Create(c) })
	r.POST("/anonymous", ctl.Create)
	r.GET("/tasks/:id", func(c *gin.Context) { c.Set(middleware.ContextUserIDKey, uint64(2)); ctl.Get(c) })
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("text", "  你好  ")
	file, err := form.CreateFormFile("cover", "misleading.txt")
	if err != nil {
		t.Fatal(err)
	}
	file.Write(pngCover(t))
	form.Close()
	req := httptest.NewRequest("POST", "/tasks", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result struct {
		Code int
		Data Task
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 200 || result.Data.Status != "running" {
		t.Fatal(w.Body.String())
	}
	task := awaitTask(t, s, 1, result.Data.ID)
	if task.Status != "succeeded" {
		t.Fatal(task)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/tasks/"+task.ID, nil))
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 404 {
		t.Fatal("another user can read task", w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/anonymous", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/tasks", bytes.NewBufferString("invalid")))
	if !bytes.Contains(w.Body.Bytes(), []byte(`"code":-1`)) {
		t.Fatal(w.Body.String())
	}
}
