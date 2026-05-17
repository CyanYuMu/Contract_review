package modelconfig

import (
	"context"
	"contract_review/app/pkg/response"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(ctx context.Context, c *app.RequestContext) {
	configs, err := h.service.List(ctx)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(consts.StatusOK, response.OK(configs))
}

func (h *Handler) GetDefault(ctx context.Context, c *app.RequestContext) {
	cfg, err := h.service.GetDefault(ctx)
	if err != nil {
		c.JSON(consts.StatusNotFound, response.FailWithMsg("未找到模型配置"))
		return
	}
	c.JSON(consts.StatusOK, response.OK(cfg))
}

func (h *Handler) Create(ctx context.Context, c *app.RequestContext) {
	var req ModelConfigRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg("模型配置参数错误："+err.Error()))
		return
	}
	cfg, err := h.service.Create(ctx, req)
	if err != nil {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(consts.StatusOK, response.OK(cfg))
}

func (h *Handler) Update(ctx context.Context, c *app.RequestContext) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg("模型配置ID错误"))
		return
	}
	var req ModelConfigRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg("模型配置参数错误："+err.Error()))
		return
	}
	cfg, err := h.service.Update(ctx, uint(id), req)
	if err != nil {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(consts.StatusOK, response.OK(cfg))
}

func (h *Handler) ProviderOptions(ctx context.Context, c *app.RequestContext) {
	c.JSON(consts.StatusOK, response.OK(ProviderOptions()))
}
