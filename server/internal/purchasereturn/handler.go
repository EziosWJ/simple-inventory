package purchasereturn

import (
	"errors"
	"strconv"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}
type Handler struct{ s *Service }

func NewHandler(s *Service) *Handler { return &Handler{s} }
func RegisterRoutes(r gin.IRouter, h *Handler) {
	g := r.Group("/purchase-returns")
	g.POST("", h.create)
	g.GET("/source/:purchaseId", h.source)
	g.GET("", h.page)
	g.GET("/:id", h.detail)
	g.PUT("/:id", h.edit)
	g.POST("/:id/post", h.post)
	g.POST("/:id/cancel", h.cancel)
}
func meta(c *gin.Context) audit.Metadata {
	m := audit.Metadata{RequestID: platform.RequestIDFromContext(c.Request.Context())}
	if p, ok := auth.PrincipalFromContext(c.Request.Context()); ok {
		m.ActorID = p.UserID
	}
	if r, ok := platform.RequestMetaFromContext(c.Request.Context()); ok {
		m.ClientIP = r.ClientIP
		m.UserAgent = r.UserAgent
		m.RequestMethod = r.RequestMethod
		m.RequestURL = r.RequestURL
	}
	return m
}

// @Description 直送业务包含两边原单及退货的数量金额和操作记录追溯；先销售退货入库，再单独办理采购退货出库。
// @Summary 新建采购退货草稿
// @Tags 采购退货
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body Input true "原采购单与退货明细"
// @Success 200 {object} ApiEnvelope{data=Document}
// @Failure 401 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/purchase-returns [post]
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

// @Summary 采购退货分页
// @Tags 采购退货
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Param purchaseId query int false "原采购单ID"
// @Param partnerId query int false "供应商ID"
// @Param status query string false "状态"
// @Success 200 {object} ApiEnvelope{data=Page}
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/purchase-returns [get]
func (h *Handler) page(c *gin.Context) {
	q := Query{Page: 1, PageSize: 10, DocumentNo: c.Query("documentNo"), Status: c.Query("status"), BusinessFrom: c.Query("businessFrom"), BusinessTo: c.Query("businessTo")}
	var e error
	if q.Page, e = intQ(c, "page", 1); e != nil {
		fail(c, e)
		return
	}
	if q.PageSize, e = intQ(c, "pageSize", 10); e != nil {
		fail(c, e)
		return
	}
	if q.PurchaseID, e = int64Q(c, "purchaseId"); e != nil {
		fail(c, e)
		return
	}
	if q.PartnerID, e = int64Q(c, "partnerId"); e != nil {
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

// @Summary 采购退货详情
// @Tags 采购退货
// @Security BearerAuth
// @Param id path int true "退货单ID"
// @Success 200 {object} ApiEnvelope{data=Document}
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/purchase-returns/{id} [get]
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

// @Summary 编辑采购退货草稿
// @Tags 采购退货
// @Security BearerAuth
// @Accept json
// @Param id path int true "退货单ID"
// @Param body body EditInput true "带版本草稿"
// @Success 200 {object} ApiEnvelope{data=Document}
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/purchase-returns/{id} [put]
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

// @Description 直送采购必须先有已过账的关联销售退货；不满足时返回 409，整笔不生效。两侧退货分别过账。
// @Summary 整单过账采购退货
// @Tags 采购退货
// @Security BearerAuth
// @Accept json
// @Param id path int true "退货单ID"
// @Param body body PostInput true "版本"
// @Success 200 {object} ApiEnvelope{data=Document}
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/purchase-returns/{id}/post [post]
func (h *Handler) post(c *gin.Context) {
	id, e := pathID(c)
	var in PostInput
	if e != nil || c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Post(c.Request.Context(), meta(c), id, in.Version)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 取消采购退货草稿
// @Tags 采购退货
// @Security BearerAuth
// @Accept json
// @Param id path int true "退货单ID"
// @Param body body CancelInput true "版本和原因"
// @Success 200 {object} ApiEnvelope{data=Document}
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/purchase-returns/{id}/cancel [post]
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
	n, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || n < 1 {
		return 0, ErrInvalid
	}
	return n, nil
}
func intQ(c *gin.Context, k string, d int) (int, error) {
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
func int64Q(c *gin.Context, k string) (int64, error) {
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
	if platform.IsTemporaryUnavailable(e) {
		platform.TemporaryUnavailable(c)
		return
	}
	status, code, msg := 500, 500, "服务暂不可用"
	if errors.Is(e, ErrInvalid) {
		status, code, msg = 400, 400, e.Error()
	} else if errors.Is(e, ErrNotFound) {
		status, code, msg = 404, 404, "采购退货单不存在"
	} else if errors.Is(e, ErrConflict) {
		status, code, msg = 409, 409, e.Error()
	}
	c.JSON(status, gin.H{"code": code, "message": msg, "data": nil})
}

// @Summary 读取可退货的已过账采购单明细
// @Tags 采购退货
// @Security BearerAuth
// @Param purchaseId path int true "原采购单ID"
// @Success 200 {object} ApiEnvelope{data=Document}
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/purchase-returns/source/{purchaseId} [get]
func (h *Handler) source(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("purchaseId"), 10, 64)
	if e != nil || id < 1 {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Source(c.Request.Context(), id)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}
