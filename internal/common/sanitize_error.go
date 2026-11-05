package common

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

// SanitizeError translates database driver and internal errors into user-friendly messages,
// preventing any database structure, SQL queries, or technical leaks from reaching clients.
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// 1. Record not found
	if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(errStr, "record not found") {
		return "请求的资源或数据不存在"
	}

	// 2. Specific uniqueness violations
	if strings.Contains(errStr, "users.email") || strings.Contains(errStr, "UNIQUE constraint failed: users.email") {
		return "该邮箱地址已被占用，请使用其他邮箱"
	}
	if strings.Contains(errStr, "users.username") || strings.Contains(errStr, "UNIQUE constraint failed: users.username") {
		return "该用户名已存在，请使用其他用户名"
	}
	if strings.Contains(errStr, "roles.name") || strings.Contains(errStr, "UNIQUE constraint failed: roles.name") {
		return "该角色标识已存在，请指定不同的角色标识"
	}
	if strings.Contains(errStr, "storage_policies.name") || strings.Contains(errStr, "UNIQUE constraint failed: storage_policies.name") {
		return "该存储策略名称已存在"
	}

	// 3. Generic uniqueness or duplicate entry
	if strings.Contains(errStr, "UNIQUE constraint failed") || strings.Contains(errStr, "Duplicate entry") {
		return "输入的内容已存在重复项，请修改后重试"
	}

	// 4. Foreign key constraint
	if strings.Contains(errStr, "FOREIGN KEY constraint failed") || strings.Contains(errStr, "a foreign key constraint fails") {
		return "关联的数据不存在或受约束，无法进行此项操作"
	}

	// 5. Shield internal database syntax, driver connection, or low-level constraint errors
	if strings.Contains(errStr, "constraint failed") ||
		strings.Contains(errStr, "syntax error") ||
		strings.Contains(errStr, "pq:") ||
		strings.Contains(errStr, "Error 1064") ||
		strings.Contains(errStr, "Error 1062") ||
		strings.Contains(errStr, "driver") ||
		strings.Contains(errStr, "sql:") {
		return "数据操作失败，请核对输入信息或稍后重试"
	}

	return errStr
}
