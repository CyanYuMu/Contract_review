package gateway

import (
	"context"
	"contract_review/app/pkg/response"
	"strconv"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// Handler 网关管理接口（成本看板 / 路由配置 / 配额配置）
type Handler struct {
	gw *Gateway
}

func NewHandler(gw *Gateway) *Handler { return &Handler{gw: gw} }

func parseDays(c *app.RequestContext, def int) int {
	d := c.Query("days")
	if d == "" {
		return def
	}
	n, err := strconv.Atoi(string(d))
	if err != nil || n <= 0 {
		return def
	}
	if n > 365 {
		n = 365
	}
	return n
}

// UsageStats GET /api/gateway/usage_stats?dimension=feature|user&days=30
func (h *Handler) UsageStats(ctx context.Context, c *app.RequestContext) {
	dimension := string(c.Query("dimension"))
	if dimension != "user" {
		dimension = "feature"
	}
	days := parseDays(c, 30)
	end := time.Now()
	start := end.AddDate(0, 0, -days)

	var stats interface{}
	var err error
	if dimension == "user" {
		stats, err = h.gw.repo.UsageStatsByUser(ctx, start, end, 50)
	} else {
		stats, err = h.gw.repo.UsageStatsByFeature(ctx, start, end)
	}
	if err != nil {
		c.JSON(consts.StatusInternalServerError, response.FailWithMsg("查询用量统计失败："+err.Error()))
		return
	}
	c.JSON(consts.StatusOK, response.OK(stats))
}

// UsageTrend GET /api/gateway/usage_trend?days=30
func (h *Handler) UsageTrend(ctx context.Context, c *app.RequestContext) {
	days := parseDays(c, 30)
	end := time.Now()
	start := end.AddDate(0, 0, -days)
	trend, err := h.gw.repo.UsageTrend(ctx, start, end)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, response.FailWithMsg("查询用量趋势失败："+err.Error()))
		return
	}
	c.JSON(consts.StatusOK, response.OK(trend))
}

// RouteView 路由 + 关联模型名
type RouteView struct {
	Feature       string `json:"feature"`
	ModelConfigID uint   `json:"model_config_id"`
	ModelName     string `json:"model_name"`
	Params        string `json:"params"`
	UpdatedAt     string `json:"updated_at"`
}

// ListRoutes GET /api/gateway/routes
func (h *Handler) ListRoutes(ctx context.Context, c *app.RequestContext) {
	routes, err := h.gw.repo.ListRoutes(ctx)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, response.FailWithMsg("查询路由失败："+err.Error()))
		return
	}
	// 取模型名映射
	modelMap := make(map[uint]string)
	if len(routes) > 0 {
		var configs []struct {
			ID        uint   `gorm:"column:id"`
			ModelName string `gorm:"column:model_name"`
		}
		ids := make([]uint, 0, len(routes))
		for _, r := range routes {
			ids = append(ids, r.ModelConfigID)
		}
		_ = h.gw.db.Table("model_configs").Where("id IN ?", ids).Scan(&configs).Error
		for _, m := range configs {
			modelMap[m.ID] = m.ModelName
		}
	}
	views := make([]RouteView, 0, len(routes))
	for _, r := range routes {
		views = append(views, RouteView{
			Feature:       r.Feature,
			ModelConfigID: r.ModelConfigID,
			ModelName:     modelMap[r.ModelConfigID],
			Params:        r.Params,
			UpdatedAt:     r.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	c.JSON(consts.StatusOK, response.OK(views))
}

// RouteUpdateRequest 更新路由请求体
type RouteUpdateRequest struct {
	Feature       string `json:"feature"`
	ModelConfigID uint   `json:"model_config_id"`
	Params        string `json:"params"`
}

// UpdateRoute PUT /api/gateway/route —— 仅改路由即可切换某功能所用模型，不动应用代码
func (h *Handler) UpdateRoute(ctx context.Context, c *app.RequestContext) {
	var req RouteUpdateRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg("路由参数错误："+err.Error()))
		return
	}
	if req.Feature == "" || req.ModelConfigID == 0 {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg("feature 与 model_config_id 不能为空"))
		return
	}
	if err := h.gw.repo.UpsertRoute(ctx, &LLMRoute{
		Feature:       req.Feature,
		ModelConfigID: req.ModelConfigID,
		Params:        req.Params,
	}); err != nil {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(consts.StatusOK, response.Ok())
}

// ListQuotas GET /api/gateway/quotas?user_id=
func (h *Handler) ListQuotas(ctx context.Context, c *app.RequestContext) {
	userID, _ := strconv.ParseUint(string(c.Query("user_id")), 10, 64)
	if userID == 0 {
		c.JSON(consts.StatusOK, response.OK([]LLMQuota{}))
		return
	}
	quotas, err := h.gw.repo.GetQuotas(ctx, userID)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, response.FailWithMsg("查询配额失败："+err.Error()))
		return
	}
	c.JSON(consts.StatusOK, response.OK(quotas))
}

// QuotaSetRequest 配额配置请求体
type QuotaSetRequest struct {
	SubjectType        string `json:"subject_type"`
	SubjectID         uint64 `json:"subject_id"`
	Feature           string `json:"feature"`
	DailyTokenLimit   int    `json:"daily_token_limit"`
	MonthlyTokenLimit int    `json:"monthly_token_limit"`
}

// SetQuota PUT /api/gateway/quota
func (h *Handler) SetQuota(ctx context.Context, c *app.RequestContext) {
	var req QuotaSetRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg("配额参数错误："+err.Error()))
		return
	}
	if req.SubjectType == "" {
		req.SubjectType = "user"
	}
	if req.SubjectID == 0 {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg("subject_id 不能为空"))
		return
	}
	quota := &LLMQuota{
		SubjectType:        req.SubjectType,
		SubjectID:         req.SubjectID,
		Feature:           req.Feature,
		DailyTokenLimit:   req.DailyTokenLimit,
		MonthlyTokenLimit: req.MonthlyTokenLimit,
	}
	if err := h.gw.db.Save(quota).Error; err != nil {
		c.JSON(consts.StatusBadRequest, response.FailWithMsg(err.Error()))
		return
	}
	c.JSON(consts.StatusOK, response.OK(quota))
}
