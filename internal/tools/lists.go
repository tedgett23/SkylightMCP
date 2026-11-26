package tools

import (
	"context"
	"fmt"
	"net/http"
)

type listFields struct {
	Label        *string        `json:"label,omitempty"`
	Color        *string        `json:"color,omitempty" jsonschema:"hex color (see list_colors)"`
	Kind         *string        `json:"kind,omitempty" jsonschema:"shopping, to_do, or other"`
	HideOnDevice *bool          `json:"hide_on_device,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
}

func (f listFields) body() body {
	b := body{}
	put(b, "label", f.Label)
	put(b, "color", f.Color)
	put(b, "kind", f.Kind)
	put(b, "hide_on_device", f.HideOnDevice)
	return b.merge(f.Extra)
}

func (r *reg) lists() {
	add(r, "list_lists", "List all lists (grocery/shopping, to-do, other) with their items.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/lists", nil, nil)
		})

	add(r, "get_list", "Get one list with its items and section names.", readOnly,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodGet, "/lists/"+id, nil, nil)
		})

	add(r, "create_list", "Create a list. kind is shopping, to_do, or other.", write,
		func(ctx context.Context, in listFields) (any, error) {
			if in.Label == nil {
				return nil, fmt.Errorf("label is required")
			}
			b := in.body()
			if _, ok := b["kind"]; !ok {
				b["kind"] = "to_do"
			}
			if _, ok := b["color"]; !ok {
				b["color"] = "#A8D4D3"
			}
			return r.call(ctx, http.MethodPost, "/lists", nil, b)
		})

	add(r, "update_list", "Rename or recolor a list, or change its kind.", write,
		func(ctx context.Context, in struct {
			ID string `json:"id"`
			listFields
		}) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPut, "/lists/"+id, nil, in.body())
		})

	add(r, "delete_list", "Delete a list and all its items.", destructive,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodDelete, "/lists/"+id, nil, nil)
		})

	add(r, "add_list_items", "Add one or more items to a list, optionally into a section.", write,
		func(ctx context.Context, in struct {
			ListID  string   `json:"list_id"`
			Labels  []string `json:"labels" jsonschema:"item text, one entry per item"`
			Section string   `json:"section,omitempty"`
		}) (any, error) {
			lid, err := seg(in.ListID)
			if err != nil {
				return nil, err
			}
			var results []any
			for _, l := range in.Labels {
				b := body{"label": l}
				putStr(b, "section", in.Section)
				v, err := r.call(ctx, http.MethodPost, "/lists/"+lid+"/list_items", nil, b)
				if err != nil {
					results = append(results, map[string]string{"label": l, "error": err.Error()})
					continue
				}
				results = append(results, v)
			}
			return map[string]any{"results": results}, nil
		})

	add(r, "update_list_item", "Update a list item: rename it, check it off (status completed) or uncheck it (pending), or move it to a section.", write,
		func(ctx context.Context, in struct {
			ListID  string         `json:"list_id"`
			ItemID  string         `json:"item_id"`
			Label   *string        `json:"label,omitempty"`
			Status  *string        `json:"status,omitempty" jsonschema:"completed or pending"`
			Section *string        `json:"section,omitempty"`
			Extra   map[string]any `json:"extra,omitempty"`
		}) (any, error) {
			s, err := segs(in.ListID, in.ItemID)
			if err != nil {
				return nil, err
			}
			b := body{}
			put(b, "label", in.Label)
			put(b, "status", in.Status)
			put(b, "section", in.Section)
			return r.call(ctx, http.MethodPut, "/lists/"+s[0]+"/list_items/"+s[1], nil, b.merge(in.Extra))
		})

	add(r, "move_list_item", "Move a list item to a new position.", write,
		func(ctx context.Context, in struct {
			ListID   string `json:"list_id"`
			ItemID   string `json:"item_id"`
			Position int    `json:"position" jsonschema:"new 1-based position"`
		}) (any, error) {
			s, err := segs(in.ListID, in.ItemID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPost, "/lists/"+s[0]+"/list_items/"+s[1]+"/move", nil, body{"position": in.Position})
		})

	add(r, "set_list_items_section", "Move several list items into a section (creates/renames sections).", write,
		func(ctx context.Context, in struct {
			ListID  string   `json:"list_id"`
			ItemIDs []string `json:"item_ids"`
			Section string   `json:"section"`
		}) (any, error) {
			lid, err := seg(in.ListID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPut, "/lists/"+lid+"/list_items/bulk_update_section", nil,
				body{"item_ids": in.ItemIDs, "section": in.Section})
		})

	add(r, "delete_list_items", "Delete one or more list items.", destructive,
		func(ctx context.Context, in struct {
			ListID  string   `json:"list_id"`
			ItemIDs []string `json:"item_ids"`
		}) (any, error) {
			lid, err := seg(in.ListID)
			if err != nil {
				return nil, err
			}
			if len(in.ItemIDs) == 1 {
				iid, err := seg(in.ItemIDs[0])
				if err != nil {
					return nil, err
				}
				return r.call(ctx, http.MethodDelete, "/lists/"+lid+"/list_items/"+iid, nil, nil)
			}
			return r.call(ctx, http.MethodDelete, "/lists/"+lid+"/list_items/bulk_destroy", nil, body{"item_ids": in.ItemIDs})
		})
}
