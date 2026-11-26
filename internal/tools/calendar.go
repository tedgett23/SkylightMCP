package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type listEventsIn struct {
	DateMin   string `json:"date_min" jsonschema:"first day to include, YYYY-MM-DD"`
	DateMax   string `json:"date_max" jsonschema:"last day to include, YYYY-MM-DD"`
	Timezone  string `json:"timezone,omitempty" jsonschema:"IANA timezone; defaults to the configured/frame timezone"`
	ProfileID string `json:"profile_id,omitempty" jsonschema:"only return events tagged with this profile (category) id"`
}

type eventFields struct {
	Summary          *string        `json:"summary,omitempty" jsonschema:"event title"`
	StartsAt         *string        `json:"starts_at,omitempty" jsonschema:"start: ISO 8601 with offset (2026-01-05T09:30:00-05:00) or YYYY-MM-DD for all-day events"`
	EndsAt           *string        `json:"ends_at,omitempty" jsonschema:"end: ISO 8601 with offset, or YYYY-MM-DD (exclusive) for all-day events"`
	AllDay           *bool          `json:"all_day,omitempty"`
	Description      *string        `json:"description,omitempty"`
	Location         *string        `json:"location,omitempty"`
	ProfileIDs       []string       `json:"profile_ids,omitempty" jsonschema:"family profile (category) ids to tag the event with"`
	RRule            []string       `json:"rrule,omitempty" jsonschema:"recurrence rules, e.g. [\"RRULE:FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,WE\"]. Pass an empty array to remove recurrence"`
	InvitedEmails    []string       `json:"invited_emails,omitempty" jsonschema:"email addresses to invite"`
	EmojiIcon        *string        `json:"emoji_icon,omitempty"`
	CountdownEnabled *bool          `json:"countdown_enabled,omitempty" jsonschema:"show a countdown to this event on the frame"`
	Timezone         string         `json:"timezone,omitempty" jsonschema:"IANA timezone for the event; defaults to the configured/frame timezone"`
	Lat              *float64       `json:"lat,omitempty"`
	Lng              *float64       `json:"lng,omitempty"`
	ReminderMinutes  *int           `json:"reminder_minutes_before,omitempty" jsonschema:"send an early notification this many minutes before the event"`
	NotifyAtStart    *bool          `json:"notify_at_start,omitempty" jsonschema:"send a notification when the event starts"`
	Extra            map[string]any `json:"extra,omitempty" jsonschema:"additional raw fields merged into the request body"`
}

func (f eventFields) body(tz string) body {
	b := body{}
	put(b, "summary", f.Summary)
	put(b, "starts_at", f.StartsAt)
	put(b, "ends_at", f.EndsAt)
	put(b, "all_day", f.AllDay)
	put(b, "description", f.Description)
	put(b, "location", f.Location)
	putSlice(b, "category_ids", f.ProfileIDs)
	if f.RRule != nil {
		if len(f.RRule) == 0 {
			b["rrule"] = nil
		} else {
			b["rrule"] = f.RRule
		}
	}
	putSlice(b, "invited_emails", f.InvitedEmails)
	put(b, "countdown_enabled", f.CountdownEnabled)
	put(b, "emoji_icon", f.EmojiIcon)
	put(b, "lat", f.Lat)
	put(b, "lng", f.Lng)
	if f.Timezone != "" {
		tz = f.Timezone
	}
	b["timezone"] = tz
	if f.ReminderMinutes != nil || f.NotifyAtStart != nil {
		ns := body{"early": false, "early_minutes_before": nil, "on_time": false}
		if f.ReminderMinutes != nil {
			ns["early"] = true
			ns["early_minutes_before"] = *f.ReminderMinutes
		}
		if f.NotifyAtStart != nil {
			ns["on_time"] = *f.NotifyAtStart
		}
		b["event_notification_setting_attributes"] = ns
	}
	return b
}

type createEventIn struct {
	eventFields
	CalendarAccountID string `json:"calendar_account_id,omitempty" jsonschema:"calendar account to create the event in (see list_calendars). Defaults to the source calendar marked default_for_new_events, else the built-in Skylight calendar"`
	CalendarID        string `json:"calendar_id,omitempty" jsonschema:"calendar id within the account (often an email address). Defaults alongside calendar_account_id"`
}

type updateEventIn struct {
	EventID string `json:"event_id" jsonschema:"event id; recurring instances look like <masterId>-<epoch>"`
	ApplyTo string `json:"apply_to,omitempty" jsonschema:"for recurring events: this, this_and_future, or all"`
	eventFields
}

type deleteEventIn struct {
	EventID string `json:"event_id"`
	ApplyTo string `json:"apply_to,omitempty" jsonschema:"for recurring events: this, this_and_future, or all (default all)"`
}

type searchIn struct {
	Query string `json:"query" jsonschema:"search text"`
}

type idIn struct {
	ID string `json:"id"`
}

type notifSettingsIn struct {
	Early              *bool          `json:"early,omitempty" jsonschema:"send an early reminder"`
	EarlyMinutesBefore *int           `json:"early_minutes_before,omitempty"`
	OnTime             *bool          `json:"on_time,omitempty" jsonschema:"notify when events start"`
	SoundOn            *bool          `json:"sound_on,omitempty"`
	Extra              map[string]any `json:"extra,omitempty"`
}

func (r *reg) calendar() {
	add(r, "list_events",
		"List calendar events in a date range, including tagged family profiles (categories), calendar account and notification settings.",
		readOnly, func(ctx context.Context, in listEventsIn) (any, error) {
			tz := in.Timezone
			if tz == "" {
				tz = r.c.Timezone(ctx)
			}
			q := url.Values{"date_min": {in.DateMin}, "date_max": {in.DateMax}, "timezone": {tz},
				"include": {"categories,calendar_account,event_notification_setting"}}
			v, err := r.call(ctx, http.MethodGet, "/calendar_events", q, nil)
			if err != nil {
				return nil, err
			}
			return filterByRel(v, in.ProfileID, "categories", "category"), nil
		})

	add(r, "search_events", "Search calendar events by text.", readOnly,
		func(ctx context.Context, in searchIn) (any, error) {
			q := url.Values{"search_query": {in.Query}, "include": {"categories"}, "timezone": {r.c.Timezone(ctx)}}
			return r.call(ctx, http.MethodGet, "/calendar_events/search", q, nil)
		})

	add(r, "list_countdowns", "List events that have countdowns enabled.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			q := url.Values{"timezone": {r.c.Timezone(ctx)}, "include": {"categories"}}
			return r.call(ctx, http.MethodGet, "/calendar_events/countdowns", q, nil)
		})

	add(r, "create_event",
		"Create a calendar event. Requires summary, starts_at and ends_at. Tag family members with profile_ids.",
		write, func(ctx context.Context, in createEventIn) (any, error) {
			if in.Summary == nil || in.StartsAt == nil || in.EndsAt == nil {
				return nil, fmt.Errorf("summary, starts_at and ends_at are required")
			}
			b := in.body(r.c.Timezone(ctx))
			defaults := body{"all_day": false, "kind": "standard", "invited_emails": []string{},
				"category_ids": []string{}, "rrule": nil, "countdown_enabled": false,
				"event_notification_setting_attributes": nil, "location": "", "lat": nil, "lng": nil, "emoji_icon": nil, "description": ""}
			for k, v := range defaults {
				if _, ok := b[k]; !ok {
					b[k] = v
				}
			}
			acct, cal := in.CalendarAccountID, in.CalendarID
			if acct == "" && cal == "" {
				acct, cal = r.defaultCalendar(ctx)
			}
			// The built-in Skylight calendar is addressed with explicit nulls.
			b["calendar_account_id"], b["calendar_id"] = nullIfEmpty(acct), nullIfEmpty(cal)
			return r.call(ctx, http.MethodPost, "/calendar_events", nil, b.merge(in.Extra))
		})

	add(r, "update_event",
		"Update a calendar event. Only the fields you pass are changed. For recurring events set apply_to.",
		write, func(ctx context.Context, in updateEventIn) (any, error) {
			id, err := seg(in.EventID)
			if err != nil {
				return nil, err
			}
			b := in.body(r.c.Timezone(ctx)).merge(in.Extra)
			var q url.Values
			if in.ApplyTo != "" {
				b["apply_to"] = in.ApplyTo
				q = url.Values{"apply_to": {in.ApplyTo}}
			}
			return r.call(ctx, http.MethodPut, "/calendar_events/"+id, q, b)
		})

	add(r, "delete_event", "Delete a calendar event (or recurring series/instances via apply_to).", destructive,
		func(ctx context.Context, in deleteEventIn) (any, error) {
			id, err := seg(in.EventID)
			if err != nil {
				return nil, err
			}
			at := in.ApplyTo
			if at == "" {
				at = "all"
			}
			return r.call(ctx, http.MethodDelete, "/calendar_events/"+id, url.Values{"apply_to": {at}}, nil)
		})

	add(r, "list_calendars",
		"List synced calendar accounts and source calendars (Google, Apple, Outlook, iCal), including which one is the default for new events and which profiles they are linked to.",
		readOnly, func(ctx context.Context, _ noInput) (any, error) {
			out := map[string]any{}
			for k, s := range map[string]string{"source_calendars": "/source_calendars", "calendar_accounts": "/calendars", "webcal_accounts": "/webcal_accounts"} {
				v, err := r.call(ctx, http.MethodGet, s, nil, nil)
				if err != nil {
					out[k] = map[string]string{"error": err.Error()}
				} else {
					out[k] = v
				}
			}
			return out, nil
		})

	add(r, "set_default_calendar", "Set which source calendar new events are created in.", write,
		func(ctx context.Context, in idIn) (any, error) {
			return r.call(ctx, http.MethodPost, "/source_calendars/set_default_for_new_events", nil,
				body{"id": in.ID, "source_calendar_id": in.ID})
		})

	add(r, "add_webcal_subscription", "Subscribe the frame to an iCal/webcal URL.", write,
		func(ctx context.Context, in struct {
			URL   string `json:"url"`
			Label string `json:"label,omitempty"`
		}) (any, error) {
			b := body{"url": in.URL}
			putStr(b, "label", in.Label)
			return r.call(ctx, http.MethodPost, "/webcal_accounts", nil, b)
		})

	add(r, "remove_source_calendar", "Remove a synced source calendar from the frame.", destructive,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodDelete, "/source_calendars/"+id, nil, nil)
		})

	add(r, "link_profiles_to_calendar", "Set which family profiles a source calendar's events are tagged with.", write,
		func(ctx context.Context, in struct {
			SourceCalendarID string   `json:"source_calendar_id"`
			ProfileIDs       []string `json:"profile_ids"`
		}) (any, error) {
			id, err := seg(in.SourceCalendarID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPut, "/source_calendars/"+id+"/source_calendar_categorizations", nil,
				body{"category_ids": in.ProfileIDs})
		})

	add(r, "get_event_notification_settings", "Get the frame-wide default event notification settings.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/event_notification_settings", nil, nil)
		})

	add(r, "update_event_notification_settings", "Update the frame-wide default event notification settings.", write,
		func(ctx context.Context, in notifSettingsIn) (any, error) {
			b := body{}
			put(b, "early", in.Early)
			put(b, "early_minutes_before", in.EarlyMinutesBefore)
			put(b, "on_time", in.OnTime)
			put(b, "sound_on", in.SoundOn)
			return r.call(ctx, http.MethodPut, "/event_notification_settings", nil, b.merge(in.Extra))
		})
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// defaultCalendar finds the source calendar flagged default_for_new_events.
func (r *reg) defaultCalendar(ctx context.Context) (acct, cal string) {
	v, err := r.call(ctx, http.MethodGet, "/source_calendars", nil, nil)
	if err != nil {
		return "", ""
	}
	m, _ := v.(map[string]any)
	items, _ := m["data"].([]any)
	for _, it := range items {
		sc, _ := it.(map[string]any)
		if d, _ := sc["default_for_new_events"].(bool); !d {
			continue
		}
		cal, _ = sc["source_id"].(string)
		for _, rel := range []string{"calendar_account", "calendar"} {
			if ref, ok := sc[rel].(map[string]any); ok {
				acct, _ = ref["id"].(string)
			}
		}
		if v, ok := sc["calendar_account_id"]; ok && acct == "" {
			acct = fmt.Sprint(v)
		}
		return acct, cal
	}
	return "", ""
}

type profileFields struct {
	Label                 *string        `json:"label,omitempty" jsonschema:"display name"`
	Color                 *string        `json:"color,omitempty" jsonschema:"hex color, e.g. #408257 (see list_colors)"`
	AvatarID              *string        `json:"avatar_id,omitempty" jsonschema:"avatar id (see list_avatars)"`
	LinkedToProfile       *bool          `json:"linked_to_profile,omitempty" jsonschema:"whether this category is a family member profile"`
	SelectedForChoreChart *bool          `json:"selected_for_chore_chart,omitempty" jsonschema:"show on the chore chart"`
	Extra                 map[string]any `json:"extra,omitempty"`
}

func (f profileFields) body() body {
	b := body{}
	put(b, "label", f.Label)
	put(b, "color", f.Color)
	put(b, "avatar_id", f.AvatarID)
	put(b, "linked_to_profile", f.LinkedToProfile)
	put(b, "selected_for_chore_chart", f.SelectedForChoreChart)
	return b.merge(f.Extra)
}

func (r *reg) profiles() {
	add(r, "list_profiles",
		"List family member profiles (called 'categories' in the API). Their ids are used to tag events, assign chores and award rewards.",
		readOnly, func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/categories", nil, nil)
		})
	add(r, "get_profile", "Get one family profile with linked calendars and family member details.", readOnly,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodGet, "/categories/"+id, nil, nil)
		})
	add(r, "create_profile", "Create a family profile.", write,
		func(ctx context.Context, in profileFields) (any, error) {
			b := in.body()
			if _, ok := b["linked_to_profile"]; !ok {
				b["linked_to_profile"] = true
			}
			if _, ok := b["selected_for_chore_chart"]; !ok {
				b["selected_for_chore_chart"] = true
			}
			return r.call(ctx, http.MethodPost, "/categories", nil, b)
		})
	add(r, "update_profile", "Update a family profile (name, color, avatar, chore chart visibility).", write,
		func(ctx context.Context, in struct {
			ID string `json:"id"`
			profileFields
		}) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPut, "/categories/"+id, nil, in.body())
		})
	add(r, "delete_profile", "Delete a family profile.", destructive,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodDelete, "/categories/"+id, nil, nil)
		})
	add(r, "update_family_member", "Set a profile's family member details (birthday, dietary preferences).", write,
		func(ctx context.Context, in struct {
			ProfileID          string         `json:"profile_id"`
			Birthday           *string        `json:"birthday,omitempty" jsonschema:"YYYY-MM-DD"`
			DietaryPreferences *string        `json:"dietary_preferences,omitempty"`
			Extra              map[string]any `json:"extra,omitempty"`
		}) (any, error) {
			id, err := seg(in.ProfileID)
			if err != nil {
				return nil, err
			}
			b := body{}
			put(b, "birthday", in.Birthday)
			put(b, "dietary_preferences", in.DietaryPreferences)
			return r.call(ctx, http.MethodPut, "/categories/"+id+"/family_member", nil, b.merge(in.Extra))
		})
	add(r, "link_calendars_to_profile", "Set which source calendars feed a family profile.", write,
		func(ctx context.Context, in struct {
			ProfileID         string   `json:"profile_id"`
			SourceCalendarIDs []string `json:"source_calendar_ids"`
		}) (any, error) {
			id, err := seg(in.ProfileID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPut, "/categories/"+id+"/source_calendar_categorizations", nil,
				body{"source_calendar_ids": in.SourceCalendarIDs})
		})
}
