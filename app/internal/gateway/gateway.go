package gateway

import (
	"context"
	"errors"
	"time"

	"contract_review/app/internal/middleware/redis"
	"contract_review/app/internal/modelconfig"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Config 网关运行配置
type Config struct {
	EnableRateLimit  bool          `mapstructure:"enable_rate_limit"`
	RateLimitPerMin  int           `mapstructure:"rate_limit_per_min"` // 每用户+功能 每分钟请求上限
	EnableQuota      bool          `mapstructure:"enable_quota"`
	EnableCostLog    bool          `mapstructure:"enable_cost_log"`
	EnableCache      bool          `mapstructure:"enable_cache"`
	CacheThreshold   float64       `mapstructure:"cache_threshold"`   // 余弦相似度阈值
	CacheTTL         time.Duration `mapstructure:"cache_ttl"`         // 缓存存活时长
	CacheMaxEntries  int           `mapstructure:"cache_max_entries"`  // 每功能最大缓存条数
	RequestTimeoutS int           `mapstructure:"request_timeout_s"` // 单次模型请求超时（秒）
}

// DefaultConfig 默认配置（未配置 yaml 时使用）
func DefaultConfig() Config {
	return Config{
		EnableRateLimit:  true,
		RateLimitPerMin:  30,
		EnableQuota:      true,
		EnableCostLog:    true,
		EnableCache:      false, // 默认关闭，需 embedding 配置就绪后开启
		CacheThreshold:   0.92,
		CacheTTL:         24 * time.Hour,
		CacheMaxEntries:  200,
		RequestTimeoutS:  300,
	}
}

// Gateway 大模型网关：业务代码统一调用入口
type Gateway struct {
	db             *gorm.DB
	repo           *Repo
	modelRepo      *modelconfig.Repo
	redis          *redis.RedisClient
	embedder       Embedder
	config         Config
	requestTimeout time.Duration
	cache          *semanticCache
}

// NewGateway 创建网关实例
func NewGateway(db *gorm.DB, rc *redis.RedisClient, embedder Embedder, cfg Config) *Gateway {
	if cfg.CacheThreshold <= 0 {
		cfg.CacheThreshold = 0.92
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 24 * time.Hour
	}
	if cfg.CacheMaxEntries <= 0 {
		cfg.CacheMaxEntries = 200
	}
	if cfg.RateLimitPerMin <= 0 {
		cfg.RateLimitPerMin = 30
	}
	if cfg.RequestTimeoutS <= 0 {
		cfg.RequestTimeoutS = 300
	}
	g := &Gateway{
		db:             db,
		repo:           NewRepo(db),
		modelRepo:      modelconfig.NewRepo(db),
		redis:          rc,
		embedder:       embedder,
		config:         cfg,
		requestTimeout: time.Duration(cfg.RequestTimeoutS) * time.Second,
	}
	if cfg.EnableCache && embedder != nil {
		g.cache = newSemanticCache(rc, embedder, cfg.CacheThreshold, cfg.CacheTTL, cfg.CacheMaxEntries)
	}
	return g
}

// Generate 兼容 fmodel.BaseChatModel 的非流式调用入口，
// 从 context 读取 caller（用户/功能）用于成本归因与限流。
func (g *Gateway) Generate(ctx context.Context, messages []*schema.Message, _ ...any) (*schema.Message, error) {
	caller := callerFromCtx(ctx)
	resp, err := g.Chat(ctx, GatewayRequest{
		Feature:  caller.Feature,
		UserID:   caller.UserID,
		Account:  caller.Account,
		Messages: messages,
	})
	if err != nil {
		return nil, err
	}
	return resp.Message, nil
}

// Chat 非流式对话（主入口）：限流 -> 配额 -> 缓存 -> 模型 -> 成本记录
func (g *Gateway) Chat(ctx context.Context, req GatewayRequest) (*GatewayResponse, error) {
	start := time.Now()
	if req.Feature == "" {
		req.Feature = FeatureDefault
	}
	resp := &GatewayResponse{}

	cfg, err := g.resolveModel(ctx, req.Feature)
	if err != nil {
		return nil, err
	}
	resp.ModelName = cfg.ModelName
	resp.Provider = cfg.ProviderType

	query := lastUserMessage(req.Messages)

	// 1. 语义缓存命中检查
	// 审阅场景每条款 prompt 含不同条款内容，相似度几乎不可能命中，反而每次算 embedding；
	// 直接跳过缓存，避免无谓的 embedding 计算开销。
	if g.cache != nil && query != "" && req.Feature != FeatureReview {
		if cached, ok := g.cache.lookup(ctx, req.Feature, query); ok {
			resp.Message = &schema.Message{Role: schema.Assistant, Content: cached}
			resp.CacheHit = true
			g.logCostSafely(ctx, req, cfg, Usage{}, 0, "cache_hit", start)
			return resp, nil
		}
	}

	// 2. 限流
	if g.config.EnableRateLimit {
		if err := g.checkRateLimit(ctx, req.UserID, req.Feature); err != nil {
			g.logCostSafely(ctx, req, cfg, Usage{}, 0, "rate_limited", start)
			return nil, err
		}
	}

	// 3. 配额预检
	if g.config.EnableQuota {
		if err := g.checkQuota(ctx, req.UserID, req.Feature); err != nil {
			g.logCostSafely(ctx, req, cfg, Usage{}, 0, "quota_exceeded", start)
			return nil, err
		}
	}

	// 4. 调用模型
	content, usage, err := g.callGenerate(ctx, cfg, req.Messages, req.Options)
	if err != nil {
		g.logCostSafely(ctx, req, cfg, usage, 0, "error: "+truncErr(err), start)
		return nil, err
	}
	resp.Message = &schema.Message{Role: schema.Assistant, Content: content}
	resp.Usage = usage

	// 5. 成本记录 + 配额扣减
	latency := int(time.Since(start).Milliseconds())
	g.logCostSafely(ctx, req, cfg, usage, latency, "ok", start)
	if g.config.EnableQuota {
		g.deductQuota(ctx, req.UserID, req.Feature, usage.TotalTokens)
	}

	// 6. 写语义缓存
	if g.cache != nil && query != "" && req.Feature != FeatureReview {
		g.cache.store(ctx, req.Feature, query, content)
	}

	return resp, nil
}

// StreamChat 流式对话：供 SSE 场景（问答）使用，通过 onDelta 逐 token 回调。
func (g *Gateway) StreamChat(ctx context.Context, req GatewayRequest, onDelta func(string)) (*GatewayResponse, error) {
	start := time.Now()
	if req.Feature == "" {
		req.Feature = FeatureDefault
	}
	resp := &GatewayResponse{}

	cfg, err := g.resolveModel(ctx, req.Feature)
	if err != nil {
		return nil, err
	}
	resp.ModelName = cfg.ModelName
	resp.Provider = cfg.ProviderType

	query := lastUserMessage(req.Messages)

	// 缓存命中：一次性回放完整答案（审阅场景跳过，原因同 Chat）
	if g.cache != nil && query != "" && req.Feature != FeatureReview {
		if cached, ok := g.cache.lookup(ctx, req.Feature, query); ok {
			if onDelta != nil {
				onDelta(cached)
			}
			resp.Message = &schema.Message{Role: schema.Assistant, Content: cached}
			resp.CacheHit = true
			g.logCostSafely(ctx, req, cfg, Usage{}, 0, "cache_hit", start)
			return resp, nil
		}
	}

	if g.config.EnableRateLimit {
		if err := g.checkRateLimit(ctx, req.UserID, req.Feature); err != nil {
			return nil, err
		}
	}
	if g.config.EnableQuota {
		if err := g.checkQuota(ctx, req.UserID, req.Feature); err != nil {
			return nil, err
		}
	}

	content, usage, err := g.callStream(ctx, cfg, req.Messages, req.Options, onDelta)
	if err != nil {
		g.logCostSafely(ctx, req, cfg, usage, 0, "error: "+truncErr(err), start)
		return nil, err
	}
	resp.Message = &schema.Message{Role: schema.Assistant, Content: content}
	resp.Usage = usage

	latency := int(time.Since(start).Milliseconds())
	g.logCostSafely(ctx, req, cfg, usage, latency, "ok", start)
	if g.config.EnableQuota {
		g.deductQuota(ctx, req.UserID, req.Feature, usage.TotalTokens)
	}
	if g.cache != nil && query != "" && req.Feature != FeatureReview {
		g.cache.store(ctx, req.Feature, query, content)
	}
	return resp, nil
}

func lastUserMessage(messages []*schema.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Role == schema.User {
			return messages[i].Content
		}
	}
	if len(messages) > 0 && messages[len(messages)-1] != nil {
		return messages[len(messages)-1].Content
	}
	return ""
}

func truncErr(err error) string {
	s := err.Error()
	return truncate(s, 256)
}

// EnsureDefaultRoutes 确保各功能有默认路由（指向 modelconfig 默认配置）。
// 首次启动时调用，便于后续仅改路由即可切换某功能的模型。
func (g *Gateway) EnsureDefaultRoutes(ctx context.Context) error {
	defaultCfg, err := g.modelRepo.GetDefault(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // 无默认配置时跳过
		}
		return err
	}
	for _, feature := range []string{FeatureReview, FeatureQA, FeatureCompare, FeatureChat, FeatureDefault} {
		existing, err := g.repo.GetRoute(ctx, feature)
		if err == nil && existing != nil {
			continue
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			zap.L().Warn("查询路由失败", zap.String("feature", feature), zap.Error(err))
			continue
		}
		if err := g.repo.UpsertRoute(ctx, &LLMRoute{
			Feature:       feature,
			ModelConfigID: defaultCfg.ID,
			Params:        "{}", // type:json 列不接受空串，必须为合法 JSON
		}); err != nil {
			zap.L().Warn("写入默认路由失败", zap.String("feature", feature), zap.Error(err))
		}
	}
	return nil
}
