package attach

const (
	MaxUploadSize    int64 = 100 << 20
	MaxFileSize      int64 = 10 << 30
	DefaultChunkSize int64 = 5 << 20
)

type VerifyRequest struct {
	FileMD5   string `json:"fileMd5"`
	FileName  string `json:"fileName"`
	FileSize  int64  `json:"fileSize"`
	ChunkSize int64  `json:"chunkSize"`
}
type VerifyResponse struct {
	IsExists       bool   `json:"isExists"`
	URL            string `json:"url"`
	UploadID       string `json:"uploadId"`
	UploadedChunks []int  `json:"uploadedChunks"`
}
type UploadResponse struct {
	FileName   string `json:"fileName"`
	OriginName string `json:"originName"`
	URL        string `json:"url"`
}
type MergeRequest struct {
	UploadID    string `json:"uploadId"`
	FileMD5     string `json:"fileMd5"`
	ChunkLength int    `json:"chunkLength"`
}
type MergeResponse struct {
	URL string `json:"url"`
	Msg string `json:"msg"`
}
type Task struct {
	VerifyRequest
	UserID   uint64
	Output   string
	Complete bool
}

func (t *Task) total() int { return int((t.FileSize + t.ChunkSize - 1) / t.ChunkSize) }
func (t *Task) expected(i int) int64 {
	if i == t.total()-1 {
		return t.FileSize - int64(i)*t.ChunkSize
	}
	return t.ChunkSize
}
