package vectordb

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"
)

// cleanContent replaces null characters with Unicode replacement character.
func cleanContent(content string) string {
	return strings.ReplaceAll(content, "\x00", "�")
}

// computeDocID 计算确定性文档 ID.
// partition_fields 指定的 key 参与 hash 以区分不同分区。
func computeDocID(content string, meta map[string]any, partitionFields []string) string {
	var extra []map[string]any
	for _, k := range partitionFields {
		if v, ok := meta[k]; ok {
			extra = append(extra, map[string]any{k: v})
		}
	}

	var idKey string
	if len(extra) > 0 {
		parts := make([]any, 0, 1+len(extra))
		parts = append(parts, content)
		for _, e := range extra {
			parts = append(parts, e)
		}
		b, _ := json.Marshal(parts)
		idKey = string(b)
	} else {
		idKey = content
	}

	return fmt.Sprintf("%x", md5.Sum([]byte(idKey)))
}

// buildFilterExpr 构建腾讯云向量数据库的过滤表达式 (meta_data.field = ...)。
// 与 Python 版 _build_expr 完全对齐。
func buildFilterExpr(filters map[string]any) string {
	if len(filters) == 0 {
		return ""
	}

	var parts []string
	for field, value := range filters {
		if value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			parts = append(parts, fmt.Sprintf(`meta_data.%s = "%s"`, field, v))
		case int:
			parts = append(parts, fmt.Sprintf(`meta_data.%s = %d`, field, v))
		case int64:
			parts = append(parts, fmt.Sprintf(`meta_data.%s = %d`, field, v))
		case float64:
			if v == float64(int64(v)) {
				parts = append(parts, fmt.Sprintf(`meta_data.%s = %d`, field, int64(v)))
			} else {
				parts = append(parts, fmt.Sprintf(`meta_data.%s = %v`, field, v))
			}
		case []any:
			escaped := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					escaped = append(escaped, fmt.Sprintf(`"%s"`, s))
				} else {
					escaped = append(escaped, fmt.Sprintf("%v", item))
				}
			}
			parts = append(parts, fmt.Sprintf(`meta_data.%s in (%s)`, field, strings.Join(escaped, ", ")))
		case []string:
			escaped := make([]string, 0, len(v))
			for _, s := range v {
				escaped = append(escaped, fmt.Sprintf(`"%s"`, s))
			}
			parts = append(parts, fmt.Sprintf(`meta_data.%s in (%s)`, field, strings.Join(escaped, ", ")))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " and ")
}
