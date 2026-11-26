package skylight

import "encoding/json"

// noisyAttrs are dropped from simplified output to keep tool results compact.
var noisyAttrs = map[string]bool{"profile_picture_urls": true}

// Simplify flattens a JSON:API document into plain objects: attributes are
// lifted to the top level next to id/type, and relationships are replaced by
// the related included resources (or bare ids when not included). Non-JSON:API
// payloads are returned unchanged.
func Simplify(raw json.RawMessage) any {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		var v any
		if json.Unmarshal(raw, &v) == nil {
			return v
		}
		return string(raw)
	}
	data, ok := doc["data"]
	if !ok {
		return doc
	}
	index := map[string]map[string]any{}
	if inc, ok := doc["included"].([]any); ok {
		for _, r := range inc {
			if m, ok := r.(map[string]any); ok {
				index[key(m)] = flat(m, nil)
			}
		}
	}
	out := map[string]any{}
	switch d := data.(type) {
	case []any:
		items := make([]any, 0, len(d))
		for _, r := range d {
			if m, ok := r.(map[string]any); ok {
				items = append(items, flat(m, index))
			} else {
				items = append(items, r)
			}
		}
		out["data"] = items
		out["count"] = len(items)
	case map[string]any:
		if _, isRes := d["type"]; isRes {
			out["data"] = flat(d, index)
		} else {
			out["data"] = d
		}
	default:
		out["data"] = d
	}
	if meta, ok := doc["meta"]; ok {
		out["meta"] = meta
	}
	return out
}

func key(m map[string]any) string {
	t, _ := m["type"].(string)
	id, _ := m["id"].(string)
	return t + "/" + id
}

// flat flattens one resource. When index is nil, relationships become ids only.
func flat(m map[string]any, index map[string]map[string]any) map[string]any {
	out := map[string]any{"id": m["id"], "type": m["type"]}
	if attrs, ok := m["attributes"].(map[string]any); ok {
		for k, v := range attrs {
			if noisyAttrs[k] {
				continue
			}
			if k == "id" { // numeric duplicate of the string id
				continue
			}
			out[k] = v
		}
	}
	rels, _ := m["relationships"].(map[string]any)
	for name, r := range rels {
		rm, _ := r.(map[string]any)
		switch d := rm["data"].(type) {
		case map[string]any:
			out[name] = resolve(d, index)
		case []any:
			list := make([]any, 0, len(d))
			for _, x := range d {
				if xm, ok := x.(map[string]any); ok {
					list = append(list, resolve(xm, index))
				}
			}
			out[name] = list
		}
	}
	return out
}

func resolve(ref map[string]any, index map[string]map[string]any) any {
	if index != nil {
		if full, ok := index[key(ref)]; ok {
			return full
		}
	}
	return map[string]any{"id": ref["id"], "type": ref["type"]}
}
