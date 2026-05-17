package riskconfig

import (
	"context"
	"strconv"
	"strings"

	"contract_review/app/internal/middleware"
	"contract_review/app/pkg/response"

	"github.com/cloudwego/hertz/pkg/app"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(ctx context.Context, c *app.RequestContext) {
	var req RiskPointRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}
	creator := middleware.GetCurrentUserID(c)
	riskPoint, err := h.service.Create(ctx, creator, req)
	if err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(200, response.OkWithData(ToResponse(riskPoint)))
}

func (h *Handler) Update(ctx context.Context, c *app.RequestContext) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的风险点ID"))
		return
	}
	var req RiskPointRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}
	riskPoint, err := h.service.Update(ctx, id, req)
	if err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(200, response.OkWithData(ToResponse(riskPoint)))
}

func (h *Handler) Detail(ctx context.Context, c *app.RequestContext) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的风险点ID"))
		return
	}
	riskPoint, err := h.service.Get(ctx, id)
	if err != nil {
		c.JSON(404, response.FailWithMsg("风险点不存在"))
		return
	}
	c.JSON(200, response.OkWithData(ToResponse(riskPoint)))
}

func (h *Handler) List(ctx context.Context, c *app.RequestContext) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", c.DefaultQuery("pageSize", "10")))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	filters := ListFilters{
		RiskID:           parseRiskQueryID(c.DefaultQuery("riskId", "")),
		RiskContent:      c.DefaultQuery("riskContent", ""),
		Status:           normalizeStatus(c.DefaultQuery("status", ""), ""),
		ContractTypeName: c.DefaultQuery("contractType", ""),
		Creator:          c.DefaultQuery("creator", ""),
		StartDate:        c.DefaultQuery("startDate", ""),
		EndDate:          c.DefaultQuery("endDate", ""),
	}
	if strings.TrimSpace(c.DefaultQuery("status", "")) == "" {
		filters.Status = ""
	}

	riskPoints, total, err := h.service.List(ctx, filters, page, pageSize)
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}
	list := make([]RiskPointResponse, 0, len(riskPoints))
	for _, riskPoint := range riskPoints {
		rp := riskPoint
		list = append(list, ToResponse(&rp))
	}
	c.JSON(200, response.OkWithData(map[string]interface{}{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"pageSize":  pageSize,
	}))
}

func (h *Handler) Delete(ctx context.Context, c *app.RequestContext) {
	id, err := parseID(c.Param("id"))
	if err != nil {
		c.JSON(400, response.FailWithMsg("无效的风险点ID"))
		return
	}
	if err := h.service.Delete(ctx, id); err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(200, response.Ok())
}

func (h *Handler) BatchDelete(ctx context.Context, c *app.RequestContext) {
	var req struct {
		IDs []uint64 `json:"ids"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, response.FailWithMsg("请求参数错误"))
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(400, response.FailWithMsg("请选择要删除的风险点"))
		return
	}
	if err := h.service.BatchDelete(ctx, req.IDs); err != nil {
		c.JSON(400, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(200, response.Ok())
}

func (h *Handler) Stats(ctx context.Context, c *app.RequestContext) {
	stats, err := h.service.Stats(ctx, c.DefaultQuery("contractType", ""))
	if err != nil {
		c.JSON(500, response.ServerError())
		return
	}
	c.JSON(200, response.OkWithData(stats))
}

func parseID(raw string) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
}

func parseRiskQueryID(raw string) uint64 {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(raw), "RP"))
	if raw == "" {
		return 0
	}
	id, _ := strconv.ParseUint(raw, 10, 64)
	return id
}
