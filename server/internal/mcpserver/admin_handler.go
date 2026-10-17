package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	platformhttp "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
)

// AuditMetadata and AuditEvent keep the module aligned with the other system
// modules that expose audit types through their own package.
type AuditMetadata = audit.Metadata

// TokenHandlerService is the administration use-case boundary consumed by HTTP.
type TokenHandlerService interface {
	Page(context.Context, TokenPageQuery) (Page[TokenView], error)
	Create(context.Context, AuditMetadata, TokenCreateInput) (TokenSecret, error)
	Reveal(context.Context, AuditMetadata, int64) (TokenSecret, error)
	Extend(context.Context, AuditMetadata, int64, TokenExtendInput) (TokenView, error)
	SetEnabled(context.Context, AuditMetadata, int64, TokenStatusInput) (TokenView, error)
	Reset(context.Context, AuditMetadata, int64) (TokenSecret, error)
}

var _ TokenHandlerService = (*TokenService)(nil)

type TokenHandler struct{ service TokenHandlerService }

// ApiEnvelope is the Swagger representation of the established response.
//
//nolint:unused // referenced by Swaggo annotations below
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewTokenHandler(service TokenHandlerService) (*TokenHandler, error) {
	if service == nil {
		return nil, errors.New("mcp token handler service is required")
	}
	return &TokenHandler{service: service}, nil
}

// RegisterRoutes mounts the administration API on an authenticated
// /api/system router group.
func RegisterRoutes(router gin.IRouter, handler *TokenHandler) {
	tokens := router.Group("/mcp/tokens")
	tokens.GET("/page", handler.page)
	tokens.POST("", handler.create)
	tokens.GET("/:id/secret", handler.reveal)
	tokens.POST("/:id/extend", handler.extend)
	tokens.PUT("/:id/status", handler.setStatus)
	tokens.POST("/:id/reset", handler.reset)
}

// tokenPage godoc
// @Summary MCP 令牌分页
// @Tags MCP令牌
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param pageSize query int false "每页条数，1-500"
// @Param keyword query string false "名称关键字"
// @Param userId query int false "绑定用户 ID"
// @Param state query string false "状态：ENABLED/DISABLED/EXPIRED"
// @Success 200 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Router /api/system/mcp/tokens/page [get]
func (h *TokenHandler) page(c *gin.Context) {
	page, pageOK := queryInt(c, "page", 1)
	pageSize, pageSizeOK := queryInt(c, "pageSize", 10)
	userID, userOK := optionalInt64(c, "userId")
	if !pageOK || !pageSizeOK || page < 1 || pageSize < 1 || pageSize > 500 || !userOK {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return
	}
	result, err := h.service.Page(c.Request.Context(), TokenPageQuery{
		Page: page, PageSize: pageSize, UserID: userID,
		Keyword: c.Query("keyword"), State: c.Query("state"),
	})
	if err != nil {
		writeTokenError(c, err)
		return
	}
	platformhttp.OK(c, result)
}

// createToken godoc
// @Summary 创建 MCP 令牌
// @Tags MCP令牌
// @Security BearerAuth
// @Param body body TokenCreateInput true "令牌名称、绑定用户与有效期"
// @Success 200 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Router /api/system/mcp/tokens [post]
func (h *TokenHandler) create(c *gin.Context) {
	var in TokenCreateInput
	if !bindTokenJSON(c, &in) {
		return
	}
	result, err := h.service.Create(c.Request.Context(), tokenAuditMetadata(c), in)
	if err != nil {
		writeTokenError(c, err)
		return
	}
	noStore(c)
	platformhttp.OK(c, result)
}

// revealToken godoc
// @Summary 查看 MCP 令牌原值
// @Tags MCP令牌
// @Security BearerAuth
// @Param id path int true "令牌 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Router /api/system/mcp/tokens/{id}/secret [get]
func (h *TokenHandler) reveal(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	result, err := h.service.Reveal(c.Request.Context(), tokenAuditMetadata(c), id)
	if err != nil {
		writeTokenError(c, err)
		return
	}
	noStore(c)
	platformhttp.OK(c, result)
}

// extendToken godoc
// @Summary 延期 MCP 令牌
// @Tags MCP令牌
// @Security BearerAuth
// @Param id path int true "令牌 ID"
// @Param body body TokenExtendInput true "增加天数或改为永久"
// @Success 200 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Router /api/system/mcp/tokens/{id}/extend [post]
func (h *TokenHandler) extend(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in TokenExtendInput
	if !bindTokenJSON(c, &in) {
		return
	}
	result, err := h.service.Extend(c.Request.Context(), tokenAuditMetadata(c), id, in)
	if err != nil {
		writeTokenError(c, err)
		return
	}
	platformhttp.OK(c, result)
}

// setTokenStatus godoc
// @Summary 启用或停用 MCP 令牌
// @Tags MCP令牌
// @Security BearerAuth
// @Param id path int true "令牌 ID"
// @Param body body TokenStatusInput true "启用状态"
// @Success 200 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Router /api/system/mcp/tokens/{id}/status [put]
func (h *TokenHandler) setStatus(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var in TokenStatusInput
	if !bindTokenJSON(c, &in) {
		return
	}
	result, err := h.service.SetEnabled(c.Request.Context(), tokenAuditMetadata(c), id, in)
	if err != nil {
		writeTokenError(c, err)
		return
	}
	platformhttp.OK(c, result)
}

// resetToken godoc
// @Summary 重置 MCP 令牌
// @Tags MCP令牌
// @Security BearerAuth
// @Param id path int true "令牌 ID"
// @Success 200 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Router /api/system/mcp/tokens/{id}/reset [post]
func (h *TokenHandler) reset(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	result, err := h.service.Reset(c.Request.Context(), tokenAuditMetadata(c), id)
	if err != nil {
		writeTokenError(c, err)
		return
	}
	noStore(c)
	platformhttp.OK(c, result)
}

func noStore(c *gin.Context) { c.Header("Cache-Control", "no-store") }

func bindTokenJSON(c *gin.Context, value any) bool {
	if err := c.ShouldBindJSON(value); err != nil {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return false
	}
	return true
}

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		platformhttp.WriteError(c, http.StatusBadRequest, platformhttp.CodeBadRequest, "参数错误", nil)
		return 0, false
	}
	return id, true
}

func queryInt(c *gin.Context, key string, fallback int) (int, bool) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}

func optionalInt64(c *gin.Context, key string) (int64, bool) {
	raw := c.Query(key)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func tokenAuditMetadata(c *gin.Context) AuditMetadata {
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	requestMeta, _ := platformhttp.RequestMetaFromContext(c.Request.Context())
	return AuditMetadata{
		ActorID: principal.UserID, RequestID: requestMeta.RequestID,
		ClientIP: requestMeta.ClientIP, UserAgent: requestMeta.UserAgent,
		RequestMethod: c.Request.Method, RequestURL: c.Request.URL.RequestURI(),
	}
}

func writeTokenError(c *gin.Context, err error) {
	switch {
	case platformhttp.IsTemporaryUnavailable(err):
		platformhttp.TemporaryUnavailable(c)
	case errors.Is(err, ErrForbidden):
		platformhttp.WriteError(c, http.StatusForbidden, platformhttp.CodeForbidden, "无权管理 MCP 令牌", nil)
	case errors.Is(err, ErrNotFound):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeNotFound, ErrNotFound.Error(), nil)
	case errors.Is(err, ErrSecretUnavailable):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeBadRequest, tokenSecretInvisible, nil)
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrConflict), errors.Is(err, ErrUnauthorized):
		platformhttp.WriteError(c, http.StatusOK, platformhttp.CodeBadRequest, err.Error(), nil)
	default:
		platformhttp.WriteError(c, http.StatusInternalServerError, platformhttp.CodeInternalError, "系统错误", nil)
	}
}
