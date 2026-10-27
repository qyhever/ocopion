package cloudflare

import (
	"ocopion/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

type Controller struct{ service *Service }

func NewController(service *Service) *Controller { return &Controller{service: service} }

// Start godoc
// @Summary 启动 Cloudflare IPv4 最低延迟优选
// @Description 无请求体；后台执行，运行期间重复调用返回同一任务。优选 16 个 IP 后从模板新增 US/KR 共 32 个节点，依次保存 IP 文本和 Clash 配置，然后按配置上传文件，上传成功后才成功。上传失败保留本地文件。模板异常不更新文件；Clash 保存失败时 IP 文本可能已更新，旧 Clash 文件保留。失败原因通过任务 error 返回。
// @Tags cloudflare
// @Produce json
// @Success 200 {object} response.Response{data=Task}
// @Router /cloudflare/fast-ip [post]
func (c *Controller) Start(ctx *gin.Context) { response.Success(ctx, c.service.Start()) }

// Get godoc
// @Summary 查询最近一次 Cloudflare 优选任务
// @Description 状态为 idle/running/succeeded/failed；时间为 RFC3339，未发生时为 null。任务失败仍返回成功封装。
// @Tags cloudflare
// @Produce json
// @Success 200 {object} response.Response{data=Task}
// @Router /cloudflare/fast-ip [get]
func (c *Controller) Get(ctx *gin.Context) { response.Success(ctx, c.service.Get()) }

// SpeedTest godoc
// @Summary 同步 Cloudflare IPv4 测速
// @Description 无请求体；最长等待 10 分钟，返回平均延迟不超过 200ms 的 12 个去重 IPv4，不保存文件或更新后台任务。
// @Tags cloudflare
// @Produce json
// @Success 200 {object} response.Response{data=SpeedTestResult}
// @Router /cloudflare/speed-test [post]
func (c *Controller) SpeedTest(ctx *gin.Context) {
	result, err := c.service.SpeedTest(ctx.Request.Context())
	if err != nil {
		response.Fail(ctx, err.Error())
		return
	}
	response.Success(ctx, result)
}
