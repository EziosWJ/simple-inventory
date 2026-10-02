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
	g.GET("/save-requests/:operation/:requestKey", h.saveResult)
	g.POST("/save-requests/:operation/:requestKey/resolve", h.resolveSave)
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
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
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

// @Description 可选 requestKey 保证同操作人同内容重试不重复建单；回执 savedVersion 表示原保存版本，返回单据为当前状态。
// @Summary 新建采购入库草稿
// @Tags 采购入库
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body Input true "采购供应商、业务日期和至少一条实物明细"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope "同标识用于不同内容"
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
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

// @Summary 查询当前操作人的采购草稿保存结果
// @Description COMMITTED 表示原保存成功；回执版本与单据最新状态分别展示。UNCONFIRMED 不证明失败，可能仍在执行；明确未提交由保存接口的校验/冲突错误表示。
// @Tags 采购入库
// @Security BearerAuth
// @Produce json
// @Param operation path string true "创建或编辑" Enums(CREATE,EDIT)
// @Param requestKey path string true "原保存标识"
// @Success 200 {object} ApiEnvelope{data=SaveResult}
// @Failure 400 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope
// @Router /api/v1/purchases/save-requests/{operation}/{requestKey} [get]
func (h *Handler) saveResult(c *gin.Context) {
	v, e := h.s.SaveResult(c.Request.Context(), meta(c).ActorID, c.Param("operation"), c.Param("requestKey"))
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 核实采购草稿保存并关闭尚未提交的标识
// @Description 等待同标识原事务结束；已提交返回 COMMITTED，尚未提交则持久关闭原标识并返回 NOT_COMMITTED，防止迟到请求再次建单。
// @Tags 采购入库
// @Security BearerAuth
// @Produce json
// @Param operation path string true "创建或编辑" Enums(CREATE,EDIT)
// @Param requestKey path string true "原保存标识"
// @Success 200 {object} ApiEnvelope{data=SaveResult}
// @Failure 400 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope
// @Router /api/v1/purchases/save-requests/{operation}/{requestKey}/resolve [post]
func (h *Handler) resolveSave(c *gin.Context) {
	v, e := h.s.ResolveSave(c.Request.Context(), meta(c), c.Param("operation"), c.Param("requestKey"))
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Description 单号按去首尾空白、忽略ASCII大小写的字面片段查询；对象和商品按保存ID组合过滤，包含停用与历史身份变化对象。
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
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
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
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
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

// @Description 可选 requestKey 绑定目标 ID、提交版本和业务内容；相同提交重试返回原回执及单据最新状态，不重复编辑或写成功审计。
// @Summary 编辑采购入库草稿
// @Tags 采购入库
// @Security BearerAuth
// @Accept json
// @Param id path int true "采购单ID"
// @Param body body EditInput true "带版本的草稿内容"
// @Success 200 {object} ApiEnvelope{data=Draft}
// @Failure 409 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
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
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
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
	if platform.IsTemporaryUnavailable(e) {
		platform.TemporaryUnavailable(c)
		return
	}
	status, code, msg := http.StatusInternalServerError, 500, "服务暂不可用"
	if errors.Is(e, ErrInvalid) {
		status, code, msg = 400, 400, e.Error()
	} else if errors.Is(e, ErrNotFound) {
		status, code, msg = 404, 404, "采购单不存在"
	} else if errors.Is(e, ErrConflict) || errors.Is(e, ErrRequestConflict) {
		status, code, msg = 409, 409, e.Error()
	}
	platform.WriteError(c, status, code, msg, nil)
}
