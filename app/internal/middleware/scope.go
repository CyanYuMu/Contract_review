package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"contract_review/app/internal/middleware/redis"
	"contract_review/app/pkg/response"

	"github.com/cloudwego/hertz/pkg/app"
)

// DefaultOrganizationID 是平台默认组织占位。多组织落地前，所有资源归入默认组织，
// 便于后续按 organization_id 做资源隔离迁移而不破坏现有 owner 级守卫。
const DefaultOrganizationID uint64 = 1

// ResourceScope 是一次请求中已解析的资源访问域，贯穿 handler -> service -> repo。
//
// 设计说明（Phase 0）：
//   - UserID 是 review/qa/session/comparison/gateway 的权威 owner 键（user_id 列）。
//   - Account 是 contracts 表的 legacy owner 键（account 列），在 contracts 迁移到
//     owner_user_id 之前作为过渡保留。新表必须使用 UserID/owner_user_id。
//   - SystemRole 是缓存值，仅供非管理路径展示与判断；管理接口的权限以 RequireSystemRole
//     每次请求的权威 DB 查询为准，保证降权即时生效。
//   - OrganizationID 预留多租户迁移路径，当前恒为 DefaultOrganizationID。
type ResourceScope struct {
	OrganizationID uint64 `json:"organization_id"`
	UserID         uint64 `json:"user_id"`
	Account        string `json:"account"`
	SystemRole     string `json:"system_role"`
}

// IsAdmin reports whether the scope carries an administrative system role.
// This is a cached hint only; privileged decisions must go through RequireSystemRole.
func (s ResourceScope) IsAdmin() bool {
	switch s.SystemRole {
	case SystemRoleAdmin, SystemRoleOwner:
		return true
	default:
		return false
	}
}

// UserLookup resolves a login account to its numeric owner id and system role.
// It is a functional dependency to avoid importing the user package into middleware
// (which would risk a circular import).
type UserLookup func(ctx context.Context, account string) (userID uint64, systemRole string, err error)

// IdentityResolver resolves an authenticated account into a ResourceScope.
type IdentityResolver interface {
	Resolve(ctx context.Context, account string) (ResourceScope, error)
}

// cachedIdentity is the Redis-cached subset of a resolved identity.
type cachedIdentity struct {
	UserID     uint64 `json:"user_id"`
	SystemRole string `json:"system_role"`
}

// dbIdentityResolver resolves account -> {UserID, SystemRole} with a short Redis
// cache to avoid a database round-trip on every authenticated request. A cache miss
// falls back to UserLookup; a Redis failure degrades gracefully to a lookup.
type dbIdentityResolver struct {
	lookup UserLookup
	cache  *redis.RedisClient
	ttl    time.Duration
}

// NewDBIdentityResolver builds an IdentityResolver backed by UserLookup and Redis.
func NewDBIdentityResolver(lookup UserLookup, cache *redis.RedisClient) IdentityResolver {
	return &dbIdentityResolver{
		lookup: lookup,
		cache:  cache,
		ttl:    120 * time.Second,
	}
}

func scopeCacheKey(account string) string {
	return "scope:ident:" + account
}

// Resolve returns the ResourceScope for account, using the cache when available.
func (r *dbIdentityResolver) Resolve(ctx context.Context, account string) (ResourceScope, error) {
	if account == "" {
		return ResourceScope{}, errors.New("empty account")
	}

	scope := ResourceScope{Account: account, OrganizationID: DefaultOrganizationID}

	if r.cache != nil {
		if raw, err := r.cache.GetString(ctx, scopeCacheKey(account)); err == nil {
			var hit cachedIdentity
			if json.Unmarshal([]byte(raw), &hit) == nil && hit.UserID != 0 {
				scope.UserID = hit.UserID
				scope.SystemRole = hit.SystemRole
				return scope, nil
			}
		}
	}

	userID, systemRole, err := r.lookup(ctx, account)
	if err != nil {
		return ResourceScope{}, err
	}
	scope.UserID = userID
	scope.SystemRole = systemRole

	if r.cache != nil {
		if data, err := json.Marshal(cachedIdentity{UserID: userID, SystemRole: systemRole}); err == nil {
			_ = r.cache.SetBytes(ctx, scopeCacheKey(account), data, r.ttl)
		}
	}
	return scope, nil
}

const scopeContextKey = "resourceScope"

// ResolveScope is a middleware that runs after JWT auth. It resolves the authenticated
// account into a ResourceScope once per request and stores it in the request context so
// handlers and services can read it via GetScope instead of re-resolving per call.
func ResolveScope(resolver IdentityResolver) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		account := GetCurrentUserID(c)
		if account == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Unauthorized())
			return
		}
		scope, err := resolver.Resolve(ctx, account)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, response.Unauthorized())
			return
		}
		c.Set(scopeContextKey, scope)
		c.Next(ctx)
	}
}

// GetScope reads the ResourceScope resolved by ResolveScope. The second return is false
// when the scope was not set (e.g. the route did not run ResolveScope).
func GetScope(c *app.RequestContext) (ResourceScope, bool) {
	v, ok := c.Get(scopeContextKey)
	if !ok {
		return ResourceScope{}, false
	}
	scope, ok := v.(ResourceScope)
	return scope, ok
}
