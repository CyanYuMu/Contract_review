package knowledgeapi

import (
	"context"

	"contract_review/app/internal/global"
	"contract_review/app/pkg/response"

	"github.com/cloudwego/hertz/pkg/app"
	"go.uber.org/zap"
)

// Handler 知识库文档管理处理器。
type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// IngestDocument 入库一篇长文档（做父子分块后写入审阅知识库）。
func (h *Handler) IngestDocument(ctx context.Context, c *app.RequestContext) {
	var req IngestDocumentRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}
	if req.Title == "" || req.Content == "" {
		c.JSON(400, response.FailWithMsg("标题和内容不能为空"))
		return
	}

	result, err := h.service.IngestDocument(ctx, req)
	if err != nil {
		global.Log.Error("知识库文档入库失败", zap.Error(err))
		c.JSON(500, response.FailWithMsg(err.Error()))
		return
	}

	c.JSON(200, response.OkWithData(IngestDocumentResponse{
		DocID:       result.DocID,
		ChunkCount:  result.ChunkCount,
		ParentCount: result.ParentCount,
		ChildCount:  result.ChildCount,
	}))
}
