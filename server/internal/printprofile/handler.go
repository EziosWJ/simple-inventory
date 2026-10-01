package printprofile

import (
	"errors"
	"net/http"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service *Service }

type envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func RegisterRoutes(r gin.IRouter, h *Handler) {
	g := r.Group("/print-profile")
	g.GET("", h.get)
	g.PUT("", h.update)
}

// @Summary 查询经营者打印资料
// @Tags 经营者打印资料
// @Security BearerAuth
// @Success 200 {object} envelope
// @Router /api/v1/print-profile [get]
func (h *Handler) get(c *gin.Context) {
	profile, err := h.service.Get(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	platform.OK(c, profile)
}

// @Summary 更新经营者打印资料
// @Tags 经营者打印资料
// @Security BearerAuth
// @Accept json
// @Param body body Profile true "经营者名称、电话和地址"
// @Success 200 {object} envelope
// @Router /api/v1/print-profile [put]
func (h *Handler) update(c *gin.Context) {
	var profile Profile
	if c.ShouldBindJSON(&profile) != nil {
		fail(c, ErrInvalid)
		return
	}
	updated, err := h.service.Update(c.Request.Context(), metadata(c), profile)
	if err != nil {
		fail(c, err)
		return
	}
	platform.OK(c, updated)
}

func metadata(c *gin.Context) audit.Metadata {
	ctx := c.Request.Context()
	m := audit.Metadata{RequestID: platform.RequestIDFromContext(ctx)}
	if principal, ok := auth.PrincipalFromContext(ctx); ok {
		m.ActorID = principal.UserID
	}
	if request, ok := platform.RequestMetaFromContext(ctx); ok {
		m.ClientIP, m.UserAgent = request.ClientIP, request.UserAgent
		m.RequestMethod, m.RequestURL = request.RequestMethod, request.RequestURL
	}
	return m
}

func fail(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, http.StatusInternalServerError, "服务暂不可用"
	if errors.Is(err, ErrInvalid) {
		status, code, message = http.StatusBadRequest, http.StatusBadRequest, "经营者名称必填；名称、电话或地址超过长度限制"
	}
	platform.WriteError(c, status, code, message, nil)
}
