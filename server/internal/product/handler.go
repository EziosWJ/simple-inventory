package product

import (
	"errors"
	"net/http"
	"strconv"

	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type Handler struct{ s *Service }
type statusRequest struct {
	Status *int `json:"status"`
}
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewHandler(s *Service) *Handler { return &Handler{s} }
func RegisterRoutes(r gin.IRouter, h *Handler) {
	g := r.Group("/products")
	g.GET("", h.page)
	g.POST("", h.create)
	g.GET("/:id", h.detail)
	g.PUT("/:id", h.update)
	g.PUT("/:id/status", h.status)
}

// @Summary 商品与服务分页
// @Tags 商品与服务
// @Security BearerAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Param keyword query string false "关键词"
// @Param type query string false "GOODS 或 SERVICE"
// @Param category query string false "分类"
// @Param status query int false "状态"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/products [get]
func (h *Handler) page(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	var status *int
	if x, ok := c.GetQuery("status"); ok {
		v, e := strconv.Atoi(x)
		if e != nil || v < 0 || v > 1 {
			fail(c, ErrInvalid)
			return
		}
		status = &v
	}
	v, e := h.s.Page(c.Request.Context(), Query{Page: page, PageSize: size, Keyword: c.Query("keyword"), Type: c.Query("type"), Category: c.Query("category"), Status: status})
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 新建商品与服务
// @Tags 商品与服务
// @Security BearerAuth
// @Accept json
// @Param body body Input true "资料"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/products [post]
func (h *Handler) create(c *gin.Context) {
	var in Input
	if c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Save(c.Request.Context(), meta(c.Request.Context()), 0, in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 商品与服务详情
// @Tags 商品与服务
// @Security BearerAuth
// @Param id path int true "ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/products/{id} [get]
func (h *Handler) detail(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Detail(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 编辑商品与服务
// @Tags 商品与服务
// @Security BearerAuth
// @Param id path int true "ID"
// @Param body body Input true "资料"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/products/{id} [put]
func (h *Handler) update(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	var in Input
	if e != nil || id <= 0 || c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Save(c.Request.Context(), meta(c.Request.Context()), id, in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 设置商品状态
// @Tags 商品与服务
// @Security BearerAuth
// @Param id path int true "ID"
// @Param body body statusRequest true "目标状态"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/products/{id}/status [put]
func (h *Handler) status(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	var in statusRequest
	if e != nil || id <= 0 || c.ShouldBindJSON(&in) != nil || in.Status == nil || (*in.Status != 0 && *in.Status != 1) {
		fail(c, ErrInvalid)
		return
	}
	e = h.s.SetStatus(c.Request.Context(), meta(c.Request.Context()), id, *in.Status)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, gin.H{"success": true})
}
func fail(c *gin.Context, e error) {
	status, code := http.StatusInternalServerError, 500
	msg := "服务暂不可用"
	if errors.Is(e, ErrInvalid) {
		status, code, msg = http.StatusBadRequest, 400, "参数错误"
	} else if errors.Is(e, ErrNotFound) {
		status, code, msg = http.StatusNotFound, 404, "资料不存在"
	} else if errors.Is(e, ErrConflict) {
		status, code, msg = http.StatusConflict, 409, "编码已存在"
	}
	platform.WriteError(c, status, code, msg, nil)
}
