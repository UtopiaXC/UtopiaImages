package http

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// FilterRule defines a single condition in advanced filtering
type FilterRule struct {
	PrefixParens string      `json:"prefix_parens,omitempty"` // "(", "((", etc.
	Field        string      `json:"field"`                   // format, watermark_mode, hotlink_mode, policy_id, size, dimensions, is_image, created_at, temp_links, calls, uv, display_name, public_path, sha256, uploader
	Op           string      `json:"op"`                      // eq, neq, in, not_in, contains, not_contains, starts_with, ends_with, gt, gte, lt, lte, between, is_zero, gt_zero, has_active, has_any, none, landscape, portrait, square, etc.
	Value        interface{} `json:"value"`                   // scalar or array
	ValueTo      interface{} `json:"value_to,omitempty"`      // for between
	Unit         string      `json:"unit,omitempty"`          // for size: B, KB, MB, GB
	SubType      string      `json:"sub_type,omitempty"`      // for dimensions: width, height, aspect_ratio
	SuffixParens string      `json:"suffix_parens,omitempty"` // ")", "))", etc.
	Connector    string      `json:"connector,omitempty"`     // "AND", "OR", "NOT" (connects this rule to next)
	Negate       bool        `json:"negate,omitempty"`
}

// AdvancedFilterPayload defines the full filter structure
type AdvancedFilterPayload struct {
	Logic string       `json:"logic,omitempty"` // legacy fallback: "and" or "or"
	Rules []FilterRule `json:"rules"`
}

type parsedRuleChunk struct {
	prefixParens string
	chunk        string
	args         []interface{}
	suffixParens string
	connector    string
}

func sanitizeParens(s string, char rune) string {
	var b strings.Builder
	for _, r := range s {
		if r == char && b.Len() < 5 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func buildCombinedFilterSQL(rules []FilterRule, defaultLogic string, db *gorm.DB, isAdmin bool) (string, []interface{}) {
	var validChunks []parsedRuleChunk
	for _, rule := range rules {
		chunk, args := buildRuleSQL(rule, db, isAdmin)
		if chunk == "" {
			continue
		}
		if rule.Negate {
			chunk = "NOT (" + chunk + ")"
		}
		validChunks = append(validChunks, parsedRuleChunk{
			prefixParens: sanitizeParens(rule.PrefixParens, '('),
			chunk:        chunk,
			args:         args,
			suffixParens: sanitizeParens(rule.SuffixParens, ')'),
			connector:    strings.ToUpper(strings.TrimSpace(rule.Connector)),
		})
	}

	if len(validChunks) == 0 {
		return "", nil
	}

	var sql strings.Builder
	var allArgs []interface{}
	openParensCount := 0

	for i, item := range validChunks {
		if i > 0 {
			conn := validChunks[i-1].connector
			if conn == "" {
				if strings.ToUpper(strings.TrimSpace(defaultLogic)) == "OR" {
					conn = "OR"
				} else {
					conn = "AND"
				}
			}

			switch conn {
			case "OR":
				sql.WriteString(" OR ")
			case "NOT", "AND NOT":
				sql.WriteString(" AND NOT ")
			case "OR NOT":
				sql.WriteString(" OR NOT ")
			default:
				sql.WriteString(" AND ")
			}
		}

		// Emit left parens
		for _, ch := range item.prefixParens {
			sql.WriteRune(ch)
			openParensCount++
		}

		// Emit condition
		sql.WriteString(item.chunk)
		allArgs = append(allArgs, item.args...)

		// Emit right parens (only if we have open parens to close)
		for _, ch := range item.suffixParens {
			if openParensCount > 0 {
				sql.WriteRune(ch)
				openParensCount--
			}
		}
	}

	// Auto-close any unclosed parens to ensure SQL syntax correctness
	for openParensCount > 0 {
		sql.WriteString(")")
		openParensCount--
	}

	return "(" + sql.String() + ")", allArgs
}

// ApplyAdvancedFileFilters processes both advanced JSON rules and legacy query params
func ApplyAdvancedFileFilters(query *gorm.DB, db *gorm.DB, c *gin.Context, isAdmin bool) *gorm.DB {
	filterRulesParam := c.Query("filter_rules")
	if filterRulesParam == "" {
		filterRulesParam = c.Query("filters")
	}

	var payload AdvancedFilterPayload
	hasAdvancedRules := false

	if filterRulesParam != "" {
		unescaped, err := url.QueryUnescape(filterRulesParam)
		if err == nil {
			filterRulesParam = unescaped
		}
		if err := json.Unmarshal([]byte(filterRulesParam), &payload); err == nil && len(payload.Rules) > 0 {
			hasAdvancedRules = true
		}
	}

	// 1. Process Advanced Filter Rules if provided
	if hasAdvancedRules {
		combinedSQL, allArgs := buildCombinedFilterSQL(payload.Rules, payload.Logic, db, isAdmin)
		if combinedSQL != "" {
			query = query.Where(combinedSQL, allArgs...)
		}
	} else {
		// 2. Fallback to Legacy Query Params
		query = applyLegacyFileFilters(query, db, c, isAdmin)
	}

	// 3. Search parameter applies on top of both advanced and legacy filters
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		like := "%" + search + "%"
		if isAdmin {
			var matchingUserIDs []uint
			db.Model(&model.User{}).Where("username LIKE ? OR nickname LIKE ? OR email LIKE ?", like, like, like).Pluck("id", &matchingUserIDs)
			if len(matchingUserIDs) > 0 {
				query = query.Where("(display_name LIKE ? OR public_path LIKE ? OR user_id IN (?))", like, like, matchingUserIDs)
			} else {
				query = query.Where("(display_name LIKE ? OR public_path LIKE ?)", like, like)
			}
		} else {
			query = query.Where("(display_name LIKE ? OR public_path LIKE ?)", like, like)
		}
	}

	return query
}

func buildRuleSQL(rule FilterRule, db *gorm.DB, isAdmin bool) (string, []interface{}) {
	field := strings.ToLower(strings.TrimSpace(rule.Field))
	op := strings.ToLower(strings.TrimSpace(rule.Op))
	val := rule.Value

	switch field {
	case "format":
		return buildFormatRuleSQL(op, val)

	case "watermark_mode":
		sVal := fmt.Sprintf("%v", val)
		if op == "neq" || op == "not_equal" || op == "is_not" {
			return "watermark_mode != ?", []interface{}{sVal}
		}
		return "watermark_mode = ?", []interface{}{sVal}

	case "hotlink_mode":
		sVal := fmt.Sprintf("%v", val)
		if op == "neq" || op == "not_equal" || op == "is_not" {
			return "hotlink_mode != ?", []interface{}{sVal}
		}
		return "hotlink_mode = ?", []interface{}{sVal}

	case "policy_id":
		return buildPolicyRuleSQL(op, val)

	case "size":
		return buildSizeRuleSQL(op, val, rule.ValueTo, rule.Unit)

	case "dimensions":
		return buildDimensionsRuleSQL(op, val, rule.ValueTo, rule.SubType)

	case "is_image":
		sVal := fmt.Sprintf("%v", val)
		if sVal == "false" || sVal == "0" || sVal == "non_image" {
			return "is_image = 0", nil
		}
		return "is_image = 1", nil

	case "created_at":
		return buildTimeRuleSQL(op, val, rule.ValueTo)

	case "temp_links":
		sVal := strings.ToLower(fmt.Sprintf("%v", val))
		if op == "has_active" || sVal == "has_active" {
			return "id IN (SELECT DISTINCT file_id FROM temporary_links WHERE revoked_at IS NULL AND expires_at > ?)", []interface{}{time.Now()}
		} else if op == "has_any" || sVal == "has_any" {
			return "id IN (SELECT DISTINCT file_id FROM temporary_links)", nil
		} else if op == "none" || sVal == "none" {
			return "id NOT IN (SELECT DISTINCT file_id FROM temporary_links)", nil
		}
		return "", nil

	case "calls":
		return buildCallsRuleSQL(op, val, rule.ValueTo)

	case "uv":
		return buildUVRuleSQL(op, val, rule.ValueTo)

	case "display_name":
		return buildStringMatchSQL("display_name", op, fmt.Sprintf("%v", val))

	case "public_path":
		return buildStringMatchSQL("public_path", op, fmt.Sprintf("%v", val))

	case "sha256":
		sVal := strings.TrimSpace(fmt.Sprintf("%v", val))
		if sVal == "" {
			return "", nil
		}
		if op == "starts_with" {
			return "sha256 LIKE ?", []interface{}{sVal + "%"}
		}
		return "sha256 = ?", []interface{}{sVal}

	case "uploader":
		if !isAdmin {
			return "", nil
		}
		return buildUploaderRuleSQL(op, val, db)
	}

	return "", nil
}

func buildFormatRuleSQL(op string, val interface{}) (string, []interface{}) {
	formatCondition := func(fmtStr string) string {
		switch strings.ToLower(strings.TrimSpace(fmtStr)) {
		case "jpg", "jpeg":
			return "(mime_type IN ('image/jpeg', 'image/jpg') OR public_path LIKE '%.jpg' OR public_path LIKE '%.jpeg')"
		case "png":
			return "(mime_type = 'image/png' OR public_path LIKE '%.png')"
		case "webp":
			return "(mime_type = 'image/webp' OR public_path LIKE '%.webp')"
		case "gif":
			return "(mime_type = 'image/gif' OR public_path LIKE '%.gif')"
		case "svg":
			return "(mime_type = 'image/svg+xml' OR public_path LIKE '%.svg')"
		case "ico":
			return "(mime_type IN ('image/x-icon', 'image/vnd.microsoft.icon') OR public_path LIKE '%.ico')"
		case "heif", "heic":
			return "(mime_type IN ('image/heif', 'image/heic') OR public_path LIKE '%.heif' OR public_path LIKE '%.heic')"
		case "other":
			return "(is_image = 0)"
		default:
			if fmtStr != "" {
				cleanExt := strings.TrimPrefix(fmtStr, ".")
				return fmt.Sprintf("(public_path LIKE '%%.%s')", cleanExt)
			}
			return ""
		}
	}

	if slice, ok := val.([]interface{}); ok && len(slice) > 0 {
		chunks := make([]string, 0, len(slice))
		for _, item := range slice {
			c := formatCondition(fmt.Sprintf("%v", item))
			if c != "" {
				chunks = append(chunks, c)
			}
		}
		if len(chunks) == 0 {
			return "", nil
		}
		joined := "(" + strings.Join(chunks, " OR ") + ")"
		if op == "not_in" || op == "neq" || op == "is_not" {
			return "NOT " + joined, nil
		}
		return joined, nil
	}

	sVal := fmt.Sprintf("%v", val)
	cond := formatCondition(sVal)
	if cond == "" {
		return "", nil
	}
	if op == "neq" || op == "is_not" || op == "not_equal" {
		return "NOT " + cond, nil
	}
	return cond, nil
}

func buildPolicyRuleSQL(op string, val interface{}) (string, []interface{}) {
	if slice, ok := val.([]interface{}); ok && len(slice) > 0 {
		var ids []uint
		for _, item := range slice {
			if idNum, err := strconv.ParseUint(fmt.Sprintf("%v", item), 10, 64); err == nil {
				ids = append(ids, uint(idNum))
			}
		}
		if len(ids) == 0 {
			return "", nil
		}
		if op == "not_in" || op == "neq" || op == "is_not" {
			return "policy_id NOT IN (?)", []interface{}{ids}
		}
		return "policy_id IN (?)", []interface{}{ids}
	}

	idNum, err := strconv.ParseUint(fmt.Sprintf("%v", val), 10, 64)
	if err != nil {
		return "", nil
	}
	if op == "neq" || op == "is_not" || op == "not_equal" {
		return "policy_id != ?", []interface{}{uint(idNum)}
	}
	return "policy_id = ?", []interface{}{uint(idNum)}
}

func parseSizeBytes(val interface{}, unit string) int64 {
	f, err := strconv.ParseFloat(fmt.Sprintf("%v", val), 64)
	if err != nil {
		return 0
	}
	unit = strings.ToUpper(strings.TrimSpace(unit))
	switch unit {
	case "GB":
		return int64(f * 1024 * 1024 * 1024)
	case "MB":
		return int64(f * 1024 * 1024)
	case "KB":
		return int64(f * 1024)
	default:
		return int64(f)
	}
}

func buildSizeRuleSQL(op string, val, valTo interface{}, unit string) (string, []interface{}) {
	b1 := parseSizeBytes(val, unit)
	switch op {
	case "gt":
		return "size > ?", []interface{}{b1}
	case "gte":
		return "size >= ?", []interface{}{b1}
	case "lt":
		return "size < ?", []interface{}{b1}
	case "lte":
		return "size <= ?", []interface{}{b1}
	case "between":
		b2 := parseSizeBytes(valTo, unit)
		if b1 > b2 {
			b1, b2 = b2, b1
		}
		return "size BETWEEN ? AND ?", []interface{}{b1, b2}
	default:
		return "size = ?", []interface{}{b1}
	}
}

func buildDimensionsRuleSQL(op string, val, valTo interface{}, subType string) (string, []interface{}) {
	subType = strings.ToLower(strings.TrimSpace(subType))
	if subType == "aspect_ratio" || op == "landscape" || op == "portrait" || op == "square" {
		targetRatio := op
		if targetRatio == "" {
			targetRatio = strings.ToLower(fmt.Sprintf("%v", val))
		}
		switch targetRatio {
		case "landscape":
			return "is_image = 1 AND width > height", nil
		case "portrait":
			return "is_image = 1 AND height > width", nil
		case "square":
			return "is_image = 1 AND width = height AND width > 0", nil
		}
		return "", nil
	}

	targetCol := "width"
	if subType == "height" {
		targetCol = "height"
	}

	num1, _ := strconv.Atoi(fmt.Sprintf("%v", val))
	switch op {
	case "gt":
		return targetCol + " > ?", []interface{}{num1}
	case "gte":
		return targetCol + " >= ?", []interface{}{num1}
	case "lt":
		return targetCol + " < ?", []interface{}{num1}
	case "lte":
		return targetCol + " <= ?", []interface{}{num1}
	case "between":
		num2, _ := strconv.Atoi(fmt.Sprintf("%v", valTo))
		if num1 > num2 {
			num1, num2 = num2, num1
		}
		return targetCol + " BETWEEN ? AND ?", []interface{}{num1, num2}
	default:
		return targetCol + " = ?", []interface{}{num1}
	}
}

func buildTimeRuleSQL(op string, val, valTo interface{}) (string, []interface{}) {
	now := time.Now()
	switch op {
	case "today":
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return "created_at >= ?", []interface{}{startOfDay}
	case "yesterday":
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		startOfYesterday := startOfDay.Add(-24 * time.Hour)
		return "created_at >= ? AND created_at < ?", []interface{}{startOfYesterday, startOfDay}
	case "7d":
		return "created_at >= ?", []interface{}{now.Add(-7 * 24 * time.Hour)}
	case "30d":
		return "created_at >= ?", []interface{}{now.Add(-30 * 24 * time.Hour)}
	case "this_month":
		startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return "created_at >= ?", []interface{}{startOfMonth}
	case "between":
		t1 := parseTimeString(fmt.Sprintf("%v", val))
		t2 := parseTimeString(fmt.Sprintf("%v", valTo))
		if t1.IsZero() && t2.IsZero() {
			return "", nil
		}
		if t1.IsZero() {
			return "created_at <= ?", []interface{}{t2}
		}
		if t2.IsZero() {
			return "created_at >= ?", []interface{}{t1}
		}
		if t1.After(t2) {
			t1, t2 = t2, t1
		}
		return "created_at BETWEEN ? AND ?", []interface{}{t1, t2}
	case "after", "gte":
		t1 := parseTimeString(fmt.Sprintf("%v", val))
		if !t1.IsZero() {
			return "created_at >= ?", []interface{}{t1}
		}
	case "before", "lte":
		t1 := parseTimeString(fmt.Sprintf("%v", val))
		if !t1.IsZero() {
			return "created_at <= ?", []interface{}{t1}
		}
	}
	return "", nil
}

func parseTimeString(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		if ts > 1000000000000 { // ms
			return time.UnixMilli(ts)
		}
		return time.Unix(ts, 0)
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func buildCallsRuleSQL(op string, val, valTo interface{}) (string, []interface{}) {
	if op == "is_zero" {
		return "id NOT IN (SELECT DISTINCT file_id FROM image_call_stats WHERE count > 0)", nil
	}
	if op == "gt_zero" {
		return "id IN (SELECT DISTINCT file_id FROM image_call_stats WHERE count > 0)", nil
	}

	num1, _ := strconv.ParseInt(fmt.Sprintf("%v", val), 10, 64)
	if op == "eq" && num1 == 0 {
		return "id NOT IN (SELECT DISTINCT file_id FROM image_call_stats WHERE count > 0)", nil
	}

	switch op {
	case "gt":
		return "id IN (SELECT file_id FROM image_call_stats GROUP BY file_id HAVING SUM(count) > ?)", []interface{}{num1}
	case "gte":
		return "id IN (SELECT file_id FROM image_call_stats GROUP BY file_id HAVING SUM(count) >= ?)", []interface{}{num1}
	case "lt":
		return "(id NOT IN (SELECT DISTINCT file_id FROM image_call_stats) OR id IN (SELECT file_id FROM image_call_stats GROUP BY file_id HAVING SUM(count) < ?))", []interface{}{num1}
	case "lte":
		return "(id NOT IN (SELECT DISTINCT file_id FROM image_call_stats) OR id IN (SELECT file_id FROM image_call_stats GROUP BY file_id HAVING SUM(count) <= ?))", []interface{}{num1}
	case "between":
		num2, _ := strconv.ParseInt(fmt.Sprintf("%v", valTo), 10, 64)
		if num1 > num2 {
			num1, num2 = num2, num1
		}
		return "id IN (SELECT file_id FROM image_call_stats GROUP BY file_id HAVING SUM(count) BETWEEN ? AND ?)", []interface{}{num1, num2}
	default:
		return "id IN (SELECT file_id FROM image_call_stats GROUP BY file_id HAVING SUM(count) = ?)", []interface{}{num1}
	}
}

func buildUVRuleSQL(op string, val, valTo interface{}) (string, []interface{}) {
	if op == "is_zero" {
		return "id NOT IN (SELECT DISTINCT file_id FROM image_call_visitors)", nil
	}
	if op == "gt_zero" {
		return "id IN (SELECT DISTINCT file_id FROM image_call_visitors)", nil
	}

	num1, _ := strconv.ParseInt(fmt.Sprintf("%v", val), 10, 64)
	if op == "eq" && num1 == 0 {
		return "id NOT IN (SELECT DISTINCT file_id FROM image_call_visitors)", nil
	}

	switch op {
	case "gt":
		return "id IN (SELECT file_id FROM image_call_visitors GROUP BY file_id HAVING COUNT(DISTINCT visitor_id) > ?)", []interface{}{num1}
	case "gte":
		return "id IN (SELECT file_id FROM image_call_visitors GROUP BY file_id HAVING COUNT(DISTINCT visitor_id) >= ?)", []interface{}{num1}
	case "lt":
		return "(id NOT IN (SELECT DISTINCT file_id FROM image_call_visitors) OR id IN (SELECT file_id FROM image_call_visitors GROUP BY file_id HAVING COUNT(DISTINCT visitor_id) < ?))", []interface{}{num1}
	case "lte":
		return "(id NOT IN (SELECT DISTINCT file_id FROM image_call_visitors) OR id IN (SELECT file_id FROM image_call_visitors GROUP BY file_id HAVING COUNT(DISTINCT visitor_id) <= ?))", []interface{}{num1}
	case "between":
		num2, _ := strconv.ParseInt(fmt.Sprintf("%v", valTo), 10, 64)
		if num1 > num2 {
			num1, num2 = num2, num1
		}
		return "id IN (SELECT file_id FROM image_call_visitors GROUP BY file_id HAVING COUNT(DISTINCT visitor_id) BETWEEN ? AND ?)", []interface{}{num1, num2}
	default:
		return "id IN (SELECT file_id FROM image_call_visitors GROUP BY file_id HAVING COUNT(DISTINCT visitor_id) = ?)", []interface{}{num1}
	}
}

func buildStringMatchSQL(column, op, val string) (string, []interface{}) {
	val = strings.TrimSpace(val)
	if val == "" {
		return "", nil
	}
	switch op {
	case "contains":
		return column + " LIKE ?", []interface{}{"%" + val + "%"}
	case "not_contains":
		return column + " NOT LIKE ?", []interface{}{"%" + val + "%"}
	case "starts_with":
		return column + " LIKE ?", []interface{}{val + "%"}
	case "ends_with":
		return column + " LIKE ?", []interface{}{"%" + val}
	default:
		return column + " = ?", []interface{}{val}
	}
}

func buildUploaderRuleSQL(op string, val interface{}, db *gorm.DB) (string, []interface{}) {
	sVal := strings.TrimSpace(fmt.Sprintf("%v", val))
	if sVal == "" {
		return "", nil
	}

	if uid, err := strconv.ParseUint(sVal, 10, 64); err == nil && op == "eq" {
		return "user_id = ?", []interface{}{uint(uid)}
	}
	if uid, err := strconv.ParseUint(sVal, 10, 64); err == nil && (op == "neq" || op == "is_not") {
		return "user_id != ?", []interface{}{uint(uid)}
	}

	like := "%" + sVal + "%"
	var userIDs []uint
	db.Model(&model.User{}).Where("username LIKE ? OR nickname LIKE ? OR email LIKE ?", like, like, like).Pluck("id", &userIDs)

	if op == "not_contains" || op == "neq" || op == "is_not" {
		if len(userIDs) > 0 {
			return "user_id NOT IN (?)", []interface{}{userIDs}
		}
		return "", nil
	}

	if len(userIDs) > 0 {
		return "user_id IN (?)", []interface{}{userIDs}
	}
	// No user matched
	return "1 = 0", nil
}

func applyLegacyFileFilters(query *gorm.DB, db *gorm.DB, c *gin.Context, isAdmin bool) *gorm.DB {
	if isImage := c.Query("is_image"); isImage != "" {
		query = query.Where("is_image = ?", isImage == "true" || isImage == "1")
	}

	if format := strings.ToLower(c.Query("format")); format != "" && format != "all" {
		cSQL, _ := buildFormatRuleSQL("eq", format)
		if cSQL != "" {
			query = query.Where(cSQL)
		}
	}

	if policyID := c.Query("policy_id"); policyID != "" && policyID != "all" {
		query = query.Where("policy_id = ?", policyID)
	}

	if wm := c.Query("watermark_mode"); wm != "" && wm != "all" {
		query = query.Where("watermark_mode = ?", wm)
	}

	if hl := c.Query("hotlink_mode"); hl != "" && hl != "all" {
		query = query.Where("hotlink_mode = ?", hl)
	}

	if timeRange := c.Query("time_range"); timeRange != "" && timeRange != "all" {
		cSQL, args := buildTimeRuleSQL(timeRange, nil, nil)
		if cSQL != "" {
			query = query.Where(cSQL, args...)
		}
	}

	if hasLinks := c.Query("has_temp_links"); hasLinks == "true" {
		query = query.Where("id IN (SELECT DISTINCT file_id FROM temporary_links WHERE revoked_at IS NULL AND expires_at > ?)", time.Now())
	}

	if isAdmin {
		if uid := c.Query("user_id"); uid != "" && uid != "all" {
			query = query.Where("user_id = ?", uid)
		}
	}

	return query
}
