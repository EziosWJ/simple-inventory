package sale

import (
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// @Summary 导出销售明细 Excel
// @Tags 销售出库
// @Security BearerAuth
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param page query int false "页码（导出忽略分页）"
// @Param pageSize query int false "每页条数（导出忽略分页）"
// @Param documentNo query string false "单号"
// @Param partnerId query int false "客户ID"
// @Param partnerKeyword query string false "客户关键词"
// @Param productId query int false "商品ID"
// @Param productKeyword query string false "商品及历史快照关键词"
// @Param status query string false "状态" Enums(DRAFT,POSTED,CANCELLED)
// @Param directDelivery query string false "交付方式" Enums(true,false)
// @Param businessFrom query string false "业务日期起"
// @Param businessTo query string false "业务日期止"
// @Param sortBy query string false "排序字段" Enums(businessDate,unitPrice,amount,netAmount)
// @Param sortDir query string false "排序方向" Enums(asc,desc)
// @Success 200 {file} file "明细及合计工作簿"
// @Failure 400 {object} ApiEnvelope "参数错误或超过100000行上限"
// @Failure 503 {object} ApiEnvelope
// @Router /api/v1/sales/items/export [get]
func (h *Handler) itemExport(c *gin.Context) {
	q, err := parseItemQuery(c)
	if err != nil {
		fail(c, err)
		return
	}
	bytes, name, err := h.s.ExportItems(c.Request.Context(), q, meta(c).ActorID)
	if err != nil {
		fail(c, err)
		return
	}
	platform.XLSX(c, name, bytes)
}
