package contacts

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

const customCategoryPrefix = "mog.custom."

type CustomField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func ParseCustomFields(values []string) ([]CustomField, error) {
	dedup := map[string]string{}

	for _, raw := range values {
		parts := strings.Split(raw, ",")
		for _, part := range parts {
			token := strings.TrimSpace(part)
			if token == "" {
				continue
			}

			key, value, ok := strings.Cut(token, "=")
			if !ok {
				return nil, fmt.Errorf("invalid custom field %q (expected key=value)", token)
			}

			key = strings.TrimSpace(key)
			if key == "" {
				return nil, fmt.Errorf("invalid custom field %q (key must not be empty)", token)
			}

			dedup[key] = strings.TrimSpace(value)
		}
	}

	fields := make([]CustomField, 0, len(dedup))
	for key, value := range dedup {
		fields = append(fields, CustomField{Key: key, Value: value})
	}
	sortCustomFields(fields)

	return fields, nil
}

func EncodeCustomFieldCategories(fields []CustomField) []string {
	if len(fields) == 0 {
		return nil
	}

	normalized := make([]CustomField, len(fields))
	copy(normalized, fields)
	sortCustomFields(normalized)

	out := make([]string, 0, len(normalized))
	for _, field := range normalized {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			continue
		}
		out = append(out, customCategoryPrefix+url.QueryEscape(key)+"="+url.QueryEscape(strings.TrimSpace(field.Value)))
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func MergeCustomFieldCategories(existing any, fields []CustomField) []string {
	keep := make([]string, 0)
	for _, category := range parseCategories(existing) {
		if _, ok := decodeCustomFieldCategory(category); ok {
			continue
		}
		keep = append(keep, category)
	}

	custom := EncodeCustomFieldCategories(fields)
	if len(keep) == 0 && len(custom) == 0 {
		return nil
	}

	merged := make([]string, 0, len(keep)+len(custom))
	merged = append(merged, keep...)
	merged = append(merged, custom...)
	return merged
}

func decodeCustomFields(categories any) []CustomField {
	decoded := make([]CustomField, 0)
	for _, category := range parseCategories(categories) {
		field, ok := decodeCustomFieldCategory(category)
		if ok {
			decoded = append(decoded, field)
		}
	}
	sortCustomFields(decoded)
	return decoded
}

func parseCategories(value any) []string {
	if value == nil {
		return nil
	}

	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, category := range typed {
			category = strings.TrimSpace(category)
			if category != "" {
				out = append(out, category)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, raw := range typed {
			if category, ok := raw.(string); ok {
				category = strings.TrimSpace(category)
				if category != "" {
					out = append(out, category)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func decodeCustomFieldCategory(category string) (CustomField, bool) {
	category = strings.TrimSpace(category)
	if !strings.HasPrefix(category, customCategoryPrefix) {
		return CustomField{}, false
	}

	raw := strings.TrimPrefix(category, customCategoryPrefix)
	rawKey, rawValue, ok := strings.Cut(raw, "=")
	if !ok {
		return CustomField{}, false
	}

	key, keyErr := url.QueryUnescape(strings.TrimSpace(rawKey))
	value, valueErr := url.QueryUnescape(strings.TrimSpace(rawValue))
	if keyErr != nil || valueErr != nil {
		return CustomField{}, false
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return CustomField{}, false
	}

	return CustomField{Key: key, Value: strings.TrimSpace(value)}, true
}

func addNormalizedCustomFields(item map[string]any) {
	if item == nil {
		return
	}

	fields := decodeCustomFields(item["categories"])
	if len(fields) == 0 {
		return
	}

	item["custom"] = fields
}

func sortCustomFields(fields []CustomField) {
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Key == fields[j].Key {
			return fields[i].Value < fields[j].Value
		}
		return fields[i].Key < fields[j].Key
	})
}
