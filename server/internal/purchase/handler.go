package purchase

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type Handler struct{ s *Service }
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewHandler(s *Service) *Handler { return &Handler{s} }
func RegisterRoutes(r gin.IRouter, h *Handler) {
	g := r.Group("/purchases")
	g.POST("", h.create)
	g.GET("", h.page)
	g.GET("/:id", h.detail)
	g.PUT("/:id", h.edit)
	g.POST("/:id/post", h.post)
	g.POST("/:id/cancel", h.cancel)
}

// @Description 采购单独过账；直送采购取消前必须先取消全部未取消关联销售。
// @Summary 过账采购入库单
// @Tags 采购入库
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "采购单ID"
// @Param body body PostInput true "确认当前版本"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 409 {object} ApiEnvelope
// @Router /api/v1/purchases/{id}/post [post]
func (h *Handler) post(c *gin.Context) {
	id, e := pathID(c)
	var in PostInput
	if e != nil || c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Post(c.Request.Context(), meta(c), id, in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}
func meta(ctx *gin.Context) audit.Metadata {
	m := audit.Metadata{RequestID: platform.RequestIDFromContext(ctx.Request.Context())}
	if p, ok := auth.PrincipalFromContext(ctx.Request.Context()); ok {
		m.ActorID = p.UserID
	}
	if r, ok := platform.RequestMetaFromContext(ctx.Request.Context()); ok {
		m.ClientIP = r.ClientIP
		m.UserAgent = r.UserAgent
		m.RequestMethod = r.RequestMethod
		m.RequestURL = r.RequestURL
	}
	return m
}

// @Summary 新建采购入库草稿
// @Tags 采购入库
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body Input true "采购供应商、业务日期和至少一条实物明细"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/v1/purchases [post]
func (h *Handler) create(c *gin.Context) {
	var in Input
	if c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Create(c.Request.Context(), meta(c), in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 采购入库草稿分页
// @Tags 采购入库
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数，上限500"
// @Param documentNo query string false "单号"
// @Param partnerId query int false "供应商ID"
// @Param productId query int false "商品ID"
// @Param status query string false "状态" Enums(DRAFT,POSTED,CANCELLED)
// @Param businessFrom query string false "业务日期起"
// @Param businessTo query string false "业务日期止"
// @Success 200 {object} ApiEnvelope{data=Page}
// @Router /api/v1/purchases [get]
func (h *Handler) page(c *gin.Context) {
	q := Query{Page: 1, PageSize: 10, DocumentNo: c.Query("documentNo"), Status: c.Query("status"), BusinessFrom: c.Query("businessFrom"), BusinessTo: c.Query("businessTo")}
	var e error
	if q.Page, e = intQuery(c, "page", 1); e != nil {
		fail(c, e)
		return
	}
	if q.PageSize, e = intQuery(c, "pageSize", 10); e != nil {
		fail(c, e)
		return
	}
	if q.PartnerID, e = int64Query(c, "partnerId"); e != nil {
		fail(c, e)
		return
	}
	if q.ProductID, e = int64Query(c, "productId"); e != nil {
		fail(c, e)
		return
	}
	v, e := h.s.Page(c.Request.Context(), q)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 采购入库单详情
// @Tags 采购入库
// @Security BearerAuth
// @Param id path int true "采购单ID"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 404 {object} ApiEnvelope
// @Router /api/v1/purchases/{id} [get]
func (h *Handler) detail(c *gin.Context) {
	id, e := pathID(c)
	if e != nil {
		fail(c, e)
		return
	}
	v, e := h.s.Detail(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 编辑采购入库草稿
// @Tags 采购入库
// @Security BearerAuth
// @Accept json
// @Param id path int true "采购单ID"
// @Param body body EditInput true "带版本的草稿内容"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 409 {object} ApiEnvelope
// @Router /api/v1/purchases/{id} [put]
func (h *Handler) edit(c *gin.Context) {
	id, e := pathID(c)
	var in EditInput
	if e != nil || c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Edit(c.Request.Context(), meta(c), id, in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 取消采购入库草稿
// @Tags 采购入库
// @Security BearerAuth
// @Accept json
// @Param id path int true "采购单ID"
// @Param body body CancelInput true "版本和必填原因"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 409 {object} ApiEnvelope
// @Router /api/v1/purchases/{id}/cancel [post]
func (h *Handler) cancel(c *gin.Context) {
	id, e := pathID(c)
	var in CancelInput
	if e != nil || c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Cancel(c.Request.Context(), meta(c), id, in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}
func pathID(c *gin.Context) (int64, error) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		return 0, ErrInvalid
	}
	return id, nil
}
func intQuery(c *gin.Context, k string, d int) (int, error) {
	s, ok := c.GetQuery(k)
	if !ok {
		return d, nil
	}
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 {
		return 0, ErrInvalid
	}
	return n, nil
}
func int64Query(c *gin.Context, k string) (int64, error) {
	s, ok := c.GetQuery(k)
	if !ok {
		return 0, nil
	}
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < 1 {
		return 0, ErrInvalid
	}
	return n, nil
}
func fail(c *gin.Context, e error) {
	status, code, msg := http.StatusInternalServerError, 500, "服务暂不可用"
	if errors.Is(e, ErrInvalid) {
		status, code, msg = 400, 400, e.Error()
	} else if errors.Is(e, ErrNotFound) {
		status, code, msg = 404, 404, "采购单不存在"
	} else if errors.Is(e, ErrConflict) {
		status, code, msg = 409, 409, e.Error()
	}
	platform.WriteError(c, status, code, msg, nil)
}
