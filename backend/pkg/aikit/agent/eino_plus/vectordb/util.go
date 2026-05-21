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

// computeDocID 计算确定性文档 ID，与 Python 版 _compute_doc_id 完全一致。
// partition_fields 指定的 key 参与 hash 以区分不同分区。
//
// Python json.dumps 默认使用 separators=(', ', ': ')，Go json.Marshal 使用紧凑格式，
// 因此这里需要手动添加空格以保持跨语言一致性。
func computeDocID(content string, meta map[string]any, partitionFields []string) string {
	var extra []map[string]any
	for _, k := range partitionFields {
		if v, ok := meta[k]; ok {
			extra = append(extra, map[string]any{k: v})
		}
	}

	var idKey string
	if len(extra) > 0 {
		idKey = pythonJSONDumps(content, extra)
	} else {
		idKey = content
	}

	return fmt.Sprintf("%x", md5.Sum([]byte(idKey)))
}

// pythonJSONDumps 生成与 Python json.dumps([content, {k:v}, ...], ensure_ascii=False)
// 一致的输出。Python 默认 separators=(', ', ': ')。
func pythonJSONDumps(content string, extras []map[string]any) string {
	var b strings.Builder
	b.WriteByte('[')
	b.WriteString(jsonString(content))
	for _, extra := range extras {
		b.WriteString(", ")
		b.WriteByte('{')
		first := true
		for k, v := range extra {
			if !first {
				b.WriteString(", ")
			}
			first = false
			b.WriteString(jsonString(k))
			b.WriteString(": ")
			b.WriteString(jsonValue(v))
		}
		b.WriteByte('}')
	}
	b.WriteByte(']')
	return b.String()
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsonValue(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
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
