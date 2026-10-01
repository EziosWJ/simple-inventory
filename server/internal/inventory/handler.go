package inventory

import (
	"context"
	"errors"
	"strconv"
	"time"

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
	g := r.Group("/inventory")
	g.GET("/balances", h.balancePage)
	g.GET("/entries", h.entryPage)
	a := g.Group("/adjustments")
	a.POST("", h.create)
	a.GET("", h.page)
	a.GET("/:id", h.detail)
	a.PUT("/:id", h.edit)
	a.POST("/:id/post", h.post)
	a.POST("/:id/cancel", h.cancel)
}

// @Summary 新建库存调整草稿
// @Tags 库存调整
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body Input true "至少一条明细；数量为非零十进制字符串，最多三位小数"
// @Success 200 {object} ApiEnvelope{data=Adjustment}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Failure 500 {object} ApiEnvelope
// @Router /api/v1/inventory/adjustments [post]
func (h *Handler) create(c *gin.Context) {
	var in Input
	if c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Create(c.Request.Context(), metadata(c.Request.Context()), in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 库存调整单详情
// @Tags 库存调整
// @Security BearerAuth
// @Produce json
// @Param id path int true "调整单ID"
// @Success 200 {object} ApiEnvelope{data=Adjustment}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 404 {object} ApiEnvelope
// @Router /api/v1/inventory/adjustments/{id} [get]
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

// @Summary 库存调整单分页
// @Tags 库存调整
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码，默认1"
// @Param pageSize query int false "每页条数，默认10，最大500"
// @Param documentNo query string false "单号子串"
// @Param status query string false "单据状态" Enums(DRAFT,POSTED,CANCELLED)
// @Param productId query int false "商品内部ID"
// @Param createdFrom query string false "创建时间下界，包含，RFC3339"
// @Param createdTo query string false "创建时间上界，不包含，RFC3339"
// @Success 200 {object} ApiEnvelope{data=Page}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/v1/inventory/adjustments [get]
func (h *Handler) page(c *gin.Context) {
	q := Query{Page: 1, PageSize: 10, DocumentNo: c.Query("documentNo"), Status: c.Query("status")}
	for name, target := range map[string]*int{"page": &q.Page, "pageSize": &q.PageSize} {
		if x, ok := c.GetQuery(name); ok {
			n, e := strconv.Atoi(x)
			if e != nil || n < 1 {
				fail(c, ErrInvalid)
				return
			}
			*target = n
		}
	}
	if x, ok := c.GetQuery("productId"); ok {
		n, e := strconv.ParseInt(x, 10, 64)
		if e != nil || n <= 0 {
			fail(c, ErrInvalid)
			return
		}
		q.ProductID = n
	}
	for name, target := range map[string]**time.Time{"createdFrom": &q.CreatedFrom, "createdTo": &q.CreatedTo} {
		if x, ok := c.GetQuery(name); ok {
			v, e := time.Parse(time.RFC3339Nano, x)
			if e != nil {
				fail(c, invalid("创建时间必须为RFC3339"))
				return
			}
			v = v.UTC()
			*target = &v
		}
	}
	v, e := h.s.Page(c.Request.Context(), q)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 当前库存分页查询
// @Tags 库存
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码，默认1"
// @Param pageSize query int false "每页条数，默认10，最大500"
// @Param keyword query string false "商品编码、名称、品牌、型号或规格子串"
// @Param category query string false "商品分类"
// @Param status query int false "启用状态：1启用，0停用"
// @Param stock query string false "库存范围，默认nonzero" Enums(nonzero,all,zero)
// @Success 200 {object} ApiEnvelope{data=BalancePage}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/v1/inventory/balances [get]
func (h *Handler) balancePage(c *gin.Context) {
	q := BalanceQuery{Page: 1, PageSize: 10, Keyword: c.Query("keyword"), Category: c.Query("category"), Stock: c.Query("stock")}
	for name, target := range map[string]*int{"page": &q.Page, "pageSize": &q.PageSize} {
		if x, ok := c.GetQuery(name); ok {
			n, e := strconv.Atoi(x)
			if e != nil || n < 1 {
				fail(c, ErrInvalid)
				return
			}
			*target = n
		}
	}
	if x, ok := c.GetQuery("status"); ok {
		n, e := strconv.Atoi(x)
		if e != nil || n < 0 || n > 1 {
			fail(c, ErrInvalid)
			return
		}
		q.Status = &n
	}
	v, e := h.s.BalancePage(c.Request.Context(), q)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 库存流水分页查询
// @Tags 库存
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码，默认1"
// @Param pageSize query int false "每页条数，默认10，最大500"
// @Param productId query int false "商品内部ID"
// @Param entryType query string false "流水类型" Enums(ORIGINAL,REVERSAL)
// @Param occurredFrom query string false "发生时间下界，包含，RFC3339"
// @Param occurredTo query string false "发生时间上界，不包含，RFC3339"
// @Success 200 {object} ApiEnvelope{data=EntryPage}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Router /api/v1/inventory/entries [get]
func (h *Handler) entryPage(c *gin.Context) {
	q := EntryQuery{Page: 1, PageSize: 10, EntryType: c.Query("entryType")}
	for name, target := range map[string]*int{"page": &q.Page, "pageSize": &q.PageSize} {
		if x, ok := c.GetQuery(name); ok {
			n, e := strconv.Atoi(x)
			if e != nil || n < 1 {
				fail(c, ErrInvalid)
				return
			}
			*target = n
		}
	}
	if x, ok := c.GetQuery("productId"); ok {
		n, e := strconv.ParseInt(x, 10, 64)
		if e != nil || n <= 0 {
			fail(c, ErrInvalid)
			return
		}
		q.ProductID = n
	}
	for name, target := range map[string]**time.Time{"occurredFrom": &q.OccurredFrom, "occurredTo": &q.OccurredTo} {
		if x, ok := c.GetQuery(name); ok {
			v, e := time.Parse(time.RFC3339Nano, x)
			if e != nil {
				fail(c, invalid("发生时间必须为RFC3339"))
				return
			}
			v = v.UTC()
			*target = &v
		}
	}
	v, e := h.s.EntryPage(c.Request.Context(), q)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

func fail(c *gin.Context, e error) {
	status, msg := 500, "服务暂不可用"
	switch {
	case errors.Is(e, ErrInvalid):
		status, msg = 400, e.Error()
	case errors.Is(e, ErrNotFound):
		status, msg = 404, e.Error()
	case errors.Is(e, ErrConflict), errors.Is(e, ErrStockInsufficient), errors.Is(e, ErrReversalInsufficient), errors.Is(e, ErrStockOverflow):
		// A shortage or an unrepresentable sum is a conflict with the stock the
		// document was confirmed against: nothing changed, and the operator can
		// correct it after checking the current quantity.
		status, msg = 409, e.Error()
	case platform.IsTemporaryUnavailable(e):
		platform.TemporaryUnavailable(c)
		return
	}
	platform.WriteError(c, status, status, msg, nil)
}
func metadata(ctx context.Context) audit.Metadata {
	m := audit.Metadata{RequestID: platform.RequestIDFromContext(ctx)}
	if p, ok := auth.PrincipalFromContext(ctx); ok {
		m.ActorID = p.UserID
	}
	if r, ok := platform.RequestMetaFromContext(ctx); ok {
		m.ClientIP = r.ClientIP
		m.UserAgent = r.UserAgent
		m.RequestMethod = r.RequestMethod
		m.RequestURL = r.RequestURL
	}
	return m
}

// @Summary 编辑库存调整草稿
// @Tags 库存调整
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "调整单ID"
// @Param body body EditInput true "核对的版本与全部新明细；仅DRAFT可编辑，成功版本加1"
// @Success 200 {object} ApiEnvelope{data=Adjustment}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 404 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Failure 500 {object} ApiEnvelope
// @Router /api/v1/inventory/adjustments/{id} [put]
func (h *Handler) edit(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	var in EditInput
	if e != nil || id <= 0 || c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Edit(c.Request.Context(), metadata(c.Request.Context()), id, in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 过账库存调整单
// @Tags 库存调整
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "调整单ID"
// @Param body body PostInput true "核对的版本；仅DRAFT可过账，成功版本加1并写入库存余额与流水"
// @Success 200 {object} ApiEnvelope{data=Adjustment}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 404 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Failure 500 {object} ApiEnvelope
// @Router /api/v1/inventory/adjustments/{id}/post [post]
func (h *Handler) post(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	var in PostInput
	if e != nil || id <= 0 || c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Post(c.Request.Context(), metadata(c.Request.Context()), id, in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}

// @Summary 取消库存调整单
// @Tags 库存调整
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "调整单ID"
// @Param body body CancelInput true "核对的版本和必填取消原因（最多500字）；DRAFT取消不改变库存，POSTED取消整单生成反向流水；冲销会负库存、重复或版本过期返回409"
// @Success 200 {object} ApiEnvelope{data=Adjustment}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 404 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Failure 500 {object} ApiEnvelope
// @Router /api/v1/inventory/adjustments/{id}/cancel [post]
func (h *Handler) cancel(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	var in CancelInput
	if e != nil || id <= 0 || c.ShouldBindJSON(&in) != nil {
		fail(c, ErrInvalid)
		return
	}
	v, e := h.s.Cancel(c.Request.Context(), metadata(c.Request.Context()), id, in)
	if e != nil {
		fail(c, e)
		return
	}
	platform.OK(c, v)
}
