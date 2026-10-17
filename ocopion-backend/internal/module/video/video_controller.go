package video

import (
	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type VideoController struct {
	videoService *VideoService
}

func NewVideoController(videoService *VideoService) *VideoController {
	return &VideoController{videoService: videoService}
}

// GetVideoList godoc
// @Summary 获取视频配置列表
// @Description 获取当前视频配置列表。
// @Tags video
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Failure 200 {object} response.Response
// @Router /video [get]
func (vc *VideoController) GetVideoList(c *gin.Context) {
	list, err := vc.videoService.GetVideoList()
	if err != nil {
		zap.L().Error("get video list failed", zap.Error(err))
		response.Fail(c, "获取视频列表失败")
		return
	}

	response.Success(c, list)
}
