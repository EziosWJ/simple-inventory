package warehouse

import (
	"errors"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"github.com/gin-gonic/gin"
	"net/http"
)

type Handler struct{ s *Service }
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewHandler(s *Service) *Handler { return &Handler{s} }
func RegisterRoutes(r gin.IRouter, h *Handler) {
	g := r.Group("/warehouse")
	g.GET("", h.get)
	g.PUT("", h.update)
}

// @Summary 查询唯一仓库
// @Tags 仓库
// @Security BearerAuth
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/warehouse [get]
func (h *Handler) get(c *gin.Context) {
	v, e := h.s.Get(c.Request.Context())
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 更新唯一仓库
// @Tags 仓库
// @Security BearerAuth
// @Accept json
// @Param body body Input true "资料"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/warehouse [put]
func (h *Handler) update(c *gin.Context) {
	var in Input
	if c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Update(c.Request.Context(), meta(c.Request.Context()), in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}
func fail(c *gin.Context, e error) {
	st, code, msg := http.StatusInternalServerError, 500, "服务暂不可用"
	if errors.Is(e, ErrInvalid) {
		st, code, msg = 400, 400, "参数错误"
	} else if errors.Is(e, ErrNotFound) {
		st, code, msg = 404, 404, "仓库不存在"
	}
	platform.WriteError(c, st, code, msg, nil)
}
