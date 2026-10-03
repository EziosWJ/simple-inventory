package auth

import (
	"context"

	"gorm.io/gorm"
)

// HasPermission resolves current role/menu grants instead of trusting menu visibility in the client.
func HasPermission(ctx context.Context, db *gorm.DB, code string) (bool, error) {
	p, ok := PrincipalFromContext(ctx)
	if !ok || p.UserID <= 0 {
		return false, nil
	}
	var count int64
	err := db.WithContext(ctx).Table("sys_menu m").Joins("JOIN sys_role_menu rm ON rm.menu_id=m.id").Joins("JOIN sys_user_role ur ON ur.role_id=rm.role_id").Joins("JOIN sys_role r ON r.id=ur.role_id").Joins("JOIN sys_user u ON u.id=ur.user_id").Where("ur.user_id=? AND m.permission_code=? AND m.status=1 AND m.deleted=0 AND r.status=1 AND r.deleted=0 AND u.status=1 AND u.deleted=0", p.UserID, code).Count(&count).Error
	return count > 0, err
}
