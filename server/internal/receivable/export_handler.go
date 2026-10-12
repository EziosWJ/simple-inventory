package receivable

import (
	"errors"

	"github.com/EziosWJ/simple-inventory/server/internal/platform/exportfile"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// @Summary 导出往来明细 Excel
// @Tags 往来余额
// @Security BearerAuth
// @Produce application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Param partnerId query int false "往来单位ID"
// @Param partnerKeyword query string false "单位关键词，与partnerId互斥"
// @Param direction query string false "方向" Enums(CUSTOMER,SUPPLIER)
// @Param category query string false "原始记录分类" Enums(OPENING,SETTLEMENT,REFUND)
// @Param from query string false "RFC3339生效起点（包含）"
// @Param to query string false "RFC3339生效终点（不包含）"
// @Param documentNo query string false "来源单号"
// @Param reversalStatus query string false "冲销状态" Enums(EFFECTIVE,REVERSED,REVERSAL)
// @Param businessFrom query string false "业务日期起"
// @Param businessTo query string false "业务日期止"
// @Param page query int false "页码（导出忽略分页）"
// @Param pageSize query int false "每页条数（导出忽略分页）"
// @Success 200 {file} file "往来明细及合计工作簿"
// @Failure 400 {object} ApiEnvelope "参数错误或超过100000行上限"
// @Failure 503 {object} ApiEnvelope
// @Router /api/v1/partner-balances/entries/export [get]
func (h *Handler) export(c *gin.Context) {
	q, err := parseEntryFilter(c)
	if err != nil {
		platform.WriteError(c, 400, 400, err.Error(), nil)
		return
	}
	bytes, name, err := h.s.ExportEntries(c.Request.Context(), q, metadata(c.Request.Context()).ActorID)
	if err != nil {
		if platform.IsTemporaryUnavailable(err) {
			platform.TemporaryUnavailable(c)
		} else if errors.Is(err, ErrInvalid) || errors.Is(err, exportfile.ErrLimit) {
			platform.WriteError(c, 400, 400, err.Error(), nil)
		} else {
			platform.WriteError(c, 500, 500, "导出往来明细失败", nil)
		}
		return
	}
	platform.XLSX(c, name, bytes)
}
