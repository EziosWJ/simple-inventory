package partner

import (
	"errors"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
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
	g := r.Group("/partners")
	g.GET("", h.page)
	g.POST("", h.create)
	g.GET("/:id", h.detail)
	g.PUT("/:id", h.update)
	g.PUT("/:id/status", h.status)
}

// @Summary 往来单位分页
// @Tags 往来单位
// @Security BearerAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Param keyword query string false "编码/名称/联系人/电话片段；去除首尾空白，ASCII 字母忽略大小写，% 和 _ 字面匹配"
// @Param type query string false "COMPANY 或 PERSON"
// @Param identity query string false "CUSTOMER 或 SUPPLIER"
// @Param status query int false "状态"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/partners [get]
func (h *Handler) page(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	var st *int
	if x, ok := c.GetQuery("status"); ok {
		n, e := strconv.Atoi(x)
		if e != nil || n < 0 || n > 1 {
			fail(c, ErrInvalid)
			return
		}
		st = &n
	}
	v, e := h.s.Page(c.Request.Context(), Query{Page: page, PageSize: size, Keyword: c.Query("keyword"), Type: c.Query("type"), Identity: c.Query("identity"), Status: st})
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 新建往来单位
// @Tags 往来单位
// @Security BearerAuth
// @Accept json
// @Param body body Input true "资料"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/partners [post]
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

// @Summary 往来单位详情
// @Tags 往来单位
// @Security BearerAuth
// @Param id path int true "ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/partners/{id} [get]
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

// @Summary 编辑往来单位
// @Tags 往来单位
// @Security BearerAuth
// @Param id path int true "ID"
// @Param body body Input true "资料"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/partners/{id} [put]
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

// @Summary 设置往来单位状态
// @Tags 往来单位
// @Security BearerAuth
// @Param id path int true "ID"
// @Param body body statusRequest true "目标状态"
// @Success 200 {object} ApiEnvelope
// @Router /api/v1/partners/{id}/status [put]
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
	st, code, msg := http.StatusInternalServerError, 500, "服务暂不可用"
	if errors.Is(e, ErrInvalid) {
		st, code, msg = 400, 400, "参数错误"
	} else if errors.Is(e, ErrNotFound) {
		st, code, msg = 404, 404, "资料不存在"
	} else if errors.Is(e, ErrConflict) {
		st, code, msg = 409, 409, "编码已存在"
	}
	platform.WriteError(c, st, code, msg, nil)
}
