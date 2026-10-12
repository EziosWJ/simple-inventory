package directorder

import (
	"errors"
	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
)

type Handler struct{ s *Service }
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewHandler(s *Service) *Handler { return &Handler{s} }
func RegisterRoutes(r gin.IRouter, h *Handler) {
	g := r.Group("/direct-orders")
	g.Use(func(c *gin.Context) {
		if e := h.s.Authorize(c.Request.Context()); e != nil {
			fail(c, e)
			c.Abort()
			return
		}
		c.Next()
	})
	g.POST("", h.create)
	g.GET("", h.page)
	g.GET("/save-requests/:operation/:requestKey", h.saveResult)
	g.POST("/save-requests/:operation/:requestKey/resolve", h.resolveSave)
	g.GET("/:id", h.detail)
	g.PUT("/:id", h.edit)
	g.POST("/:id/post", h.post)
	g.POST("/:id/cancel", h.cancel)
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

// @Summary 新建直送草稿
// @Description 一个供应商一个客户，至少一条实物；草稿不改变库存或往来。requestKey 用于保存结果核实和防重。
// @Tags 直送单
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body Input true "直送资料"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 400 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Router /api/v1/direct-orders [post]
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

// @Summary 编辑直送草稿
// @Tags 直送单
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "直送单ID"
// @Param body body EditInput true "当前版本及直送资料"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 409 {object} ApiEnvelope
// @Router /api/v1/direct-orders/{id} [put]
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

// @Summary 查询直送详情
// @Tags 直送单
// @Security BearerAuth
// @Produce json
// @Param id path int true "直送单ID"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 404 {object} ApiEnvelope
// @Router /api/v1/direct-orders/{id} [get]
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

// @Summary 分页查询直送单
// @Tags 直送单
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param pageSize query int false "每页数量"
// @Param documentNo query string false "单号"
// @Param supplierId query int false "供应商ID"
// @Param customerId query int false "客户ID"
// @Param status query string false "状态"
// @Param businessFrom query string false "业务日期起"
// @Param businessTo query string false "业务日期止"
// @Success 200 {object} ApiEnvelope{data=Page}
// @Router /api/v1/direct-orders [get]
func (h *Handler) page(c *gin.Context) {
	q := Query{Page: 1, PageSize: 10, DocumentNo: c.Query("documentNo"), Status: c.Query("status"), BusinessFrom: c.Query("businessFrom"), BusinessTo: c.Query("businessTo")}
	var e error
	for _, x := range []struct {
		Name   string
		Target *int
	}{{"page", &q.Page}, {"pageSize", &q.PageSize}} {
		if v := c.Query(x.Name); v != "" {
			*x.Target, e = strconv.Atoi(v)
			if e != nil {
				fail(c, ErrInvalid)
				return
			}
		}
	}
	for _, x := range []struct {
		Name   string
		Target *int64
	}{{"supplierId", &q.SupplierID}, {"customerId", &q.CustomerID}} {
		if v := c.Query(x.Name); v != "" {
			*x.Target, e = strconv.ParseInt(v, 10, 64)
			if e != nil {
				fail(c, ErrInvalid)
				return
			}
		}
	}
	v, e := h.s.Page(c.Request.Context(), q)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 核实直送草稿保存结果
// @Tags 直送单
// @Security BearerAuth
// @Param operation path string true "CREATE 或 EDIT"
// @Param requestKey path string true "保存标识"
// @Success 200 {object} ApiEnvelope{data=SaveResult}
// @Router /api/v1/direct-orders/save-requests/{operation}/{requestKey} [get]
func (h *Handler) saveResult(c *gin.Context) {
	v, e := h.s.SaveResult(c.Request.Context(), meta(c).ActorID, c.Param("operation"), c.Param("requestKey"))
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 核实并关闭未提交的直送保存标识
// @Tags 直送单
// @Security BearerAuth
// @Param operation path string true "CREATE 或 EDIT"
// @Param requestKey path string true "保存标识"
// @Success 200 {object} ApiEnvelope{data=SaveResult}
// @Router /api/v1/direct-orders/save-requests/{operation}/{requestKey}/resolve [post]
func (h *Handler) resolveSave(c *gin.Context) {
	v, e := h.s.ResolveSave(c.Request.Context(), meta(c), c.Param("operation"), c.Param("requestKey"))
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}
func pathID(c *gin.Context) (int64, error) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id < 1 {
		return 0, ErrInvalid
	}
	return id, nil
}
func fail(c *gin.Context, e error) {
	if platform.IsTemporaryUnavailable(e) {
		platform.TemporaryUnavailable(c)
		return
	}
	status, code, msg := http.StatusInternalServerError, 500, "服务暂不可用"
	if errors.Is(e, ErrForbidden) {
		status, code, msg = 403, 403, e.Error()
	} else if errors.Is(e, ErrInvalid) {
		status, code, msg = 400, 400, e.Error()
	} else if errors.Is(e, ErrNotFound) {
		status, code, msg = 404, 404, "直送单不存在"
	} else if errors.Is(e, ErrConflict) || errors.Is(e, ErrRequestConflict) {
		status, code, msg = 409, 409, e.Error()
	}
	platform.WriteError(c, status, code, msg, nil)
}

// @Summary 一次确认直送并过账采购销售
// @Description 两边单据、库存、往来及审计同事务提交；实物库存净变化为零。重复原版本请求不重复生效。
// @Tags 直送单
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "直送单ID"
// @Param body body PostInput true "当前版本"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 400 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Router /api/v1/direct-orders/{id}/post [post]
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

// @Summary 整笔取消直送
// @Description 有生效退货时拒绝；冲销采购销售同事务完成，保留真实收付款，后续独立新建不与原取消关联。
// @Tags 直送单
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "直送单ID"
// @Param body body CancelInput true "版本与取消原因"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 409 {object} ApiEnvelope
// @Router /api/v1/direct-orders/{id}/cancel [post]
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
