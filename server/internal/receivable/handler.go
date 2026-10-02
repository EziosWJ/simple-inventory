package receivable

import (
	"context"
	"errors"
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
	g := r.Group("/partner-balances")
	g.GET("", h.balances)
	g.GET("/entries", h.page)
	g.GET("/entries/:id", h.detail)
	g.GET("/statement", h.statement)
	g.POST("/opening", h.create)
	g.POST("/settlements", h.settle)
	g.POST("/refunds", h.refund)
	g.POST("/entries/:id/reverse", h.reverse)
}

// @Summary 按往来余额记录客户收款或供应商付款
// @Tags 往来余额
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body SettlementInput true "收付款信息"
// @Success 200 {object} ApiEnvelope{data=Entry}
// @Failure 400 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/partner-balances/settlements [post]
func (h *Handler) settle(c *gin.Context) {
	var in SettlementInput
	if c.ShouldBindJSON(&in) != nil {
		platform.WriteError(c, 400, 400, "请求参数无效", nil)
		return
	}
	v, e := h.s.Settle(c.Request.Context(), metadata(c.Request.Context()), in)
	if e != nil {
		if platform.IsTemporaryUnavailable(e) {
			platform.TemporaryUnavailable(c)
			return
		}
		if errors.Is(e, ErrInvalid) {
			platform.WriteError(c, 400, 400, e.Error(), nil)
		} else if errors.Is(e, ErrConflict) {
			platform.WriteError(c, 409, 409, e.Error(), nil)
		} else {
			platform.WriteError(c, 500, 500, "保存收付款失败", nil)
		}
		return
	}
	platform.OK(c, v)
}

// @Summary 向客户退款或收到供应商退款（保存即生效）
// @Tags 往来余额
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body SettlementInput true "退款方式、业务日期与幂等键；金额不得超过对应方向当前待退款"
// @Success 200 {object} ApiEnvelope{data=Entry}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/partner-balances/refunds [post]
func (h *Handler) refund(c *gin.Context) {
	var in SettlementInput
	if c.ShouldBindJSON(&in) != nil {
		platform.WriteError(c, 400, 400, "请求参数无效", nil)
		return
	}
	v, e := h.s.Refund(c.Request.Context(), metadata(c.Request.Context()), in)
	if e != nil {
		if platform.IsTemporaryUnavailable(e) {
			platform.TemporaryUnavailable(c)
			return
		}
		if errors.Is(e, ErrInvalid) {
			platform.WriteError(c, 400, 400, e.Error(), nil)
		} else if errors.Is(e, ErrConflict) {
			platform.WriteError(c, 409, 409, e.Error(), nil)
		} else {
			platform.WriteError(c, 500, 500, "保存退款失败", nil)
		}
		return
	}
	platform.OK(c, v)
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

// @Summary 查询客户应收与供应商应付余额
// @Tags 往来余额
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Param direction query string false "方向" Enums(CUSTOMER,SUPPLIER)
// @Param partnerId query int false "往来单位ID"
// @Success 200 {object} ApiEnvelope{data=BalancePage}
// @Failure 401 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/partner-balances [get]
func (h *Handler) balances(c *gin.Context) {
	p, e := paramInt(c, "page", 1)
	if e != nil {
		platform.WriteError(c, 400, 400, "页码无效", nil)
		return
	}
	size, e := paramInt(c, "pageSize", 20)
	if e != nil {
		platform.WriteError(c, 400, 400, "分页大小无效", nil)
		return
	}
	partnerID := int64(0)
	if raw := c.Query("partnerId"); raw != "" {
		partnerID, e = strconv.ParseInt(raw, 10, 64)
		if e != nil || partnerID <= 0 {
			platform.WriteError(c, 400, 400, "往来单位无效", nil)
			return
		}
	}
	v, e := h.s.Balances(c.Request.Context(), partnerID, p, size, c.Query("direction"))
	if e != nil {
		if platform.IsTemporaryUnavailable(e) {
			platform.TemporaryUnavailable(c)
			return
		}
		platform.WriteError(c, 400, 400, e.Error(), nil)
		return
	}
	platform.OK(c, v)
}

// @Summary 按实际生效时间分页查询完整来源往来明细
// @Tags 往来余额
// @Security BearerAuth
// @Produce json
// @Param partnerId query int false "往来单位ID"
// @Param direction query string false "方向" Enums(CUSTOMER,SUPPLIER)
// @Param category query string false "原始记录分类" Enums(OPENING,SETTLEMENT,REFUND)
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} ApiEnvelope{data=Page}
// @Failure 401 {object} ApiEnvelope
// @Param from query string false "RFC3339生效起点（包含）"
// @Param to query string false "RFC3339生效终点（不包含）"
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/partner-balances/entries [get]
func (h *Handler) page(c *gin.Context) {
	p, e := paramInt(c, "page", 1)
	if e != nil {
		platform.WriteError(c, 400, 400, "页码无效", nil)
		return
	}
	size, e := paramInt(c, "pageSize", 20)
	if e != nil {
		platform.WriteError(c, 400, 400, "分页大小无效", nil)
		return
	}
	partner := int64(0)
	if x := c.Query("partnerId"); x != "" {
		partner, e = strconv.ParseInt(x, 10, 64)
		if e != nil {
			platform.WriteError(c, 400, 400, "往来单位无效", nil)
			return
		}
	}
	v, e := h.s.FilterPage(c.Request.Context(), EntryFilter{PartnerID: partner, Direction: c.Query("direction"), Category: c.Query("category"), From: c.Query("from"), To: c.Query("to"), Page: p, PageSize: size})
	if e != nil {
		if platform.IsTemporaryUnavailable(e) {
			platform.TemporaryUnavailable(c)
			return
		}
		platform.WriteError(c, 400, 400, e.Error(), nil)
		return
	}
	platform.OK(c, v)
}

// @Summary 查询往来金额记录详情
// @Tags 往来余额
// @Security BearerAuth
// @Produce json
// @Param id path int true "金额记录ID"
// @Success 200 {object} ApiEnvelope{data=Entry}
// @Failure 404 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/partner-balances/entries/{id} [get]
func (h *Handler) detail(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		platform.WriteError(c, 400, 400, "记录ID无效", nil)
		return
	}
	v, e := h.s.Detail(c.Request.Context(), id)
	if e != nil {
		if platform.IsTemporaryUnavailable(e) {
			platform.TemporaryUnavailable(c)
			return
		}
		platform.WriteError(c, 404, 404, e.Error(), nil)
		return
	}
	platform.OK(c, v)
}

// @Summary 录入期初应收应付
// @Tags 往来余额
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body Input true "requestKey幂等键、人民币正金额、业务日期和必填说明"
// @Success 200 {object} ApiEnvelope{data=Entry}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/partner-balances/opening [post]
func (h *Handler) create(c *gin.Context) {
	var in Input
	if c.ShouldBindJSON(&in) != nil {
		platform.WriteError(c, 400, 400, "请求参数无效", nil)
		return
	}
	v, e := h.s.Create(c.Request.Context(), metadata(c.Request.Context()), in)
	if e != nil {
		if platform.IsTemporaryUnavailable(e) {
			platform.TemporaryUnavailable(c)
			return
		}
		if errors.Is(e, ErrInvalid) {
			platform.WriteError(c, 400, 400, e.Error(), nil)
		} else if errors.Is(e, ErrConflict) {
			platform.WriteError(c, 409, 409, e.Error(), nil)
		} else {
			platform.WriteError(c, 500, 500, "保存期初余额失败", nil)
		}
		return
	}
	platform.OK(c, v)
}

// @Summary 冲销期初、收付款或退款记录
// @Tags 往来余额
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "往来流水ID"
// @Param body body object true "冲销原因"
// @Success 200 {object} ApiEnvelope{data=Entry}
// @Failure 409 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/partner-balances/entries/{id}/reverse [post]
func (h *Handler) reverse(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	var in struct {
		Reason string `json:"reason"`
	}
	if e != nil || c.ShouldBindJSON(&in) != nil {
		platform.WriteError(c, 400, 400, "请求参数无效", nil)
		return
	}
	v, e := h.s.Reverse(c.Request.Context(), metadata(c.Request.Context()), id, in.Reason)
	if e != nil {
		if platform.IsTemporaryUnavailable(e) {
			platform.TemporaryUnavailable(c)
			return
		}
		if errors.Is(e, ErrInvalid) {
			platform.WriteError(c, 400, 400, e.Error(), nil)
		} else if errors.Is(e, ErrNotFound) {
			platform.WriteError(c, 404, 404, e.Error(), nil)
		} else if errors.Is(e, ErrConflict) {
			platform.WriteError(c, 409, 409, e.Error(), nil)
		} else {
			platform.WriteError(c, 500, 500, "冲销往来记录失败", nil)
		}
		return
	}
	platform.OK(c, v)
}
func paramInt(c *gin.Context, key string, def int) (int, error) {
	if x := c.Query(key); x != "" {
		v, e := strconv.Atoi(x)
		if e != nil || v < 1 {
			return 0, ErrInvalid
		}
		return v, nil
	}
	return def, nil
}

// @Summary 查询完整期间往来对账单（含历史期初与完整流水）
// @Tags 往来余额
// @Security BearerAuth
// @Produce json
// @Param partnerId query int true "往来单位ID"
// @Param direction query string true "方向" Enums(CUSTOMER,SUPPLIER)
// @Param from query string true "RFC3339生效起点（包含）"
// @Param to query string true "RFC3339生效终点（不包含）"
// @Success 200 {object} ApiEnvelope{data=Statement}
// @Failure 400 {object} ApiEnvelope
// @Failure 401 {object} ApiEnvelope
// @Failure 503 {object} ApiEnvelope "数据库暂时不可用，可稍后重试"
// @Router /api/v1/partner-balances/statement [get]
func (h *Handler) statement(c *gin.Context) {
	id, e := strconv.ParseInt(c.Query("partnerId"), 10, 64)
	if e != nil || id <= 0 {
		platform.WriteError(c, 400, 400, "往来单位无效", nil)
		return
	}
	v, e := h.s.Statement(c.Request.Context(), EntryFilter{PartnerID: id, Direction: c.Query("direction"), From: c.Query("from"), To: c.Query("to")})
	if e != nil {
		if platform.IsTemporaryUnavailable(e) {
			platform.TemporaryUnavailable(c)
			return
		}
		if errors.Is(e, ErrInvalid) {
			platform.WriteError(c, 400, 400, "期间或方向无效，请使用RFC3339且开始早于结束", nil)
		} else if errors.Is(e, ErrNotFound) {
			platform.WriteError(c, 404, 404, e.Error(), nil)
		} else {
			platform.WriteError(c, 500, 500, "读取对账单失败", nil)
		}
		return
	}
	platform.OK(c, v)
}
