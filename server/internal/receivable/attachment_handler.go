package receivable

import (
	"errors"
	"strconv"

	"github.com/EziosWJ/simple-inventory/server/internal/filemgmt"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"github.com/gin-gonic/gin"
)

// @Summary 查询往来业务有效附件及移除历史
// @Tags 往来余额
// @Security BearerAuth
// @Produce json
// @Param id path int true "往来记录ID"
// @Success 200 {object} ApiEnvelope{data=[]Attachment}
// @Failure 403 {object} ApiEnvelope
// @Router /api/v1/partner-balances/entries/{id}/attachments [get]
func (h *Handler) attachments(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		attachmentError(c, ErrInvalid)
		return
	}
	v, err := h.s.Attachments(c.Request.Context(), id)
	if err != nil {
		attachmentError(c, err)
		return
	}
	platform.OK(c, v)
}

type AttachmentInput struct {
	FileIDs []int64 `json:"fileIds"`
}

// @Summary 补传往来业务附件（不影响往来金额）
// @Tags 往来余额
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "往来记录ID"
// @Param body body AttachmentInput true "已上传的文件ID"
// @Success 200 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Router /api/v1/partner-balances/entries/{id}/attachments [post]
func (h *Handler) addAttachments(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	var in AttachmentInput
	if err != nil || id <= 0 || c.ShouldBindJSON(&in) != nil {
		attachmentError(c, ErrInvalid)
		return
	}
	if err := h.s.AddAttachments(c.Request.Context(), metadata(c.Request.Context()), id, in.FileIDs); err != nil {
		attachmentError(c, err)
		return
	}
	platform.OK(c, nil)
}

// @Summary 移除往来附件关联，保留原文件与操作历史
// @Tags 往来余额
// @Security BearerAuth
// @Produce json
// @Param id path int true "往来记录ID"
// @Param attachmentId path int true "附件关联ID"
// @Success 200 {object} ApiEnvelope
// @Failure 403 {object} ApiEnvelope
// @Failure 409 {object} ApiEnvelope
// @Router /api/v1/partner-balances/entries/{id}/attachments/{attachmentId} [delete]
func (h *Handler) removeAttachment(c *gin.Context) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	a, err := strconv.ParseInt(c.Param("attachmentId"), 10, 64)
	if e != nil || err != nil || id <= 0 || a <= 0 {
		attachmentError(c, ErrInvalid)
		return
	}
	if err := h.s.RemoveAttachment(c.Request.Context(), metadata(c.Request.Context()), id, a); err != nil {
		attachmentError(c, err)
		return
	}
	platform.OK(c, nil)
}
func attachmentError(c *gin.Context, err error) {
	code := 500
	switch {
	case platform.IsTemporaryUnavailable(err):
		platform.TemporaryUnavailable(c)
		return
	case errors.Is(err, ErrInvalid):
		code = 400
	case errors.Is(err, ErrNotFound):
		code = 404
	case errors.Is(err, ErrConflict):
		code = 409
	case errors.Is(err, filemgmt.ErrForbidden):
		code = 403
	}
	message := err.Error()
	if code == 500 {
		message = "附件操作失败"
	}
	platform.WriteError(c, code, code, message, nil)
}
