package tools

import (
	"context"
	"net/http"
	"net/url"
)

type fieldsIn struct {
	Fields map[string]any `json:"fields" jsonschema:"request body fields"`
}

type idFieldsIn struct {
	ID     string         `json:"id"`
	Fields map[string]any `json:"fields" jsonschema:"fields to change"`
}

func (r *reg) misc() {
	// Nudges (reminders)
	add(r, "list_nudges", "List nudges (reminder notifications) in a date range.", readOnly,
		func(ctx context.Context, in struct {
			After  string `json:"after" jsonschema:"first date, YYYY-MM-DD"`
			Before string `json:"before" jsonschema:"last date, YYYY-MM-DD"`
		}) (any, error) {
			return r.call(ctx, http.MethodGet, "/nudges", url.Values{"after": {in.After}, "before": {in.Before}}, nil)
		})
	add(r, "create_nudge", "Create a nudge (reminder). Body shape is not fully documented; inspect list_nudges output for field names.", write,
		func(ctx context.Context, in fieldsIn) (any, error) {
			return r.call(ctx, http.MethodPost, "/nudges", nil, in.Fields)
		})
	add(r, "update_nudge", "Update a nudge.", write,
		func(ctx context.Context, in idFieldsIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPatch, "/nudges/"+id, nil, in.Fields)
		})
	add(r, "delete_nudge", "Delete a nudge.", destructive,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodDelete, "/nudges/"+id, nil, nil)
		})

	// Devices & alarms
	add(r, "list_devices", "List physical Skylight devices on the frame.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/devices", nil, nil)
		})
	add(r, "list_alarms", "List alarms on a Skylight device.", readOnly,
		func(ctx context.Context, in struct {
			DeviceID string `json:"device_id"`
		}) (any, error) {
			id, err := seg(in.DeviceID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodGet, "/devices/"+id+"/alarms", nil, nil)
		})
	add(r, "create_alarm", "Create an alarm on a device. Inspect list_alarms output for field names.", write,
		func(ctx context.Context, in struct {
			DeviceID string         `json:"device_id"`
			Fields   map[string]any `json:"fields"`
		}) (any, error) {
			id, err := seg(in.DeviceID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPost, "/devices/"+id+"/alarms", nil, in.Fields)
		})
	add(r, "update_alarm", "Update a device alarm.", write,
		func(ctx context.Context, in struct {
			DeviceID string         `json:"device_id"`
			AlarmID  string         `json:"alarm_id"`
			Fields   map[string]any `json:"fields"`
		}) (any, error) {
			s, err := segs(in.DeviceID, in.AlarmID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPatch, "/devices/"+s[0]+"/alarms/"+s[1], nil, in.Fields)
		})
	add(r, "delete_alarm", "Delete a device alarm.", destructive,
		func(ctx context.Context, in struct {
			DeviceID string `json:"device_id"`
			AlarmID  string `json:"alarm_id"`
		}) (any, error) {
			s, err := segs(in.DeviceID, in.AlarmID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodDelete, "/devices/"+s[0]+"/alarms/"+s[1], nil, nil)
		})

	// Photos / messages
	add(r, "list_photos", "List photos/messages on the frame (paged).", readOnly,
		func(ctx context.Context, in struct {
			PageToken string `json:"page_token,omitempty" jsonschema:"page token from a previous call; omit for the first page"`
		}) (any, error) {
			pt := in.PageToken
			if pt == "" {
				pt = "__START__"
			}
			return r.call(ctx, http.MethodGet, "/messages", url.Values{"page_token": {pt}}, nil)
		})
	add(r, "get_photo", "Get a photo/message's details including its URL and caption.", readOnly,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodGet, "/messages/"+id, nil, nil)
		})
	add(r, "update_photo_caption", "Set a photo's caption.", write,
		func(ctx context.Context, in struct {
			ID      string `json:"id"`
			Caption string `json:"caption"`
		}) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPut, "/messages/"+id+"/caption", nil, body{"caption": in.Caption})
		})
	add(r, "delete_photo", "Delete a photo/message from the frame.", destructive,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodDelete, "/messages/"+id, nil, nil)
		})
	add(r, "list_albums", "List photo albums.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/albums", nil, nil)
		})

	// Misc
	add(r, "get_task_notification_settings", "Get task notification settings (task_due, task_completed).", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/task_notification_settings", nil, nil)
		})
	add(r, "update_task_notification_settings", "Update task notification settings, e.g. fields={\"task_due\":{\"enabled\":true}}.", write,
		func(ctx context.Context, in fieldsIn) (any, error) {
			return r.call(ctx, http.MethodPatch, "/task_notification_settings", nil, in.Fields)
		})
	add(r, "get_calendar_event_notification_settings", "Get 'event added' push notification settings.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/calendar_event_notification_settings", nil, nil)
		})
	add(r, "update_calendar_event_notification_settings", "Update 'event added' notification settings, e.g. fields={\"event_added\":{\"enabled\":true}}.", write,
		func(ctx context.Context, in fieldsIn) (any, error) {
			return r.call(ctx, http.MethodPatch, "/calendar_event_notification_settings", nil, in.Fields)
		})
	add(r, "get_household_config", "Get household configuration.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/household_config", nil, nil)
		})
	add(r, "list_frame_users", "List users who have access to the frame.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/users", nil, nil)
		})
	add(r, "get_activity_feed", "Get the recent activity feed.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.global(ctx, http.MethodGet, "/api/activities", nil, nil)
		})
	add(r, "list_sidekick_intents", "List Sidekick (AI auto-creation) intents and their status.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/auto_creation_intents", nil, nil)
		})
}
