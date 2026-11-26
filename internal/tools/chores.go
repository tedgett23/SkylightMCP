package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type listChoresIn struct {
	After       string `json:"after" jsonschema:"first date, YYYY-MM-DD"`
	Before      string `json:"before" jsonschema:"last date, YYYY-MM-DD (same as after for a single day)"`
	IncludeLate *bool  `json:"include_late,omitempty" jsonschema:"include overdue chores (default true)"`
	ProfileID   string `json:"profile_id,omitempty" jsonschema:"only chores assigned to this profile"`
}

type choreFields struct {
	Summary         *string        `json:"summary,omitempty" jsonschema:"chore / task title"`
	Start           *string        `json:"start,omitempty" jsonschema:"date YYYY-MM-DD"`
	StartTime       *int           `json:"start_time,omitempty" jsonschema:"time of day; the app sends values like 900 (unverified encoding: likely HHMM)"`
	Recurring       *bool          `json:"recurring,omitempty"`
	RecurrenceSet   []string       `json:"recurrence_set,omitempty" jsonschema:"rrules, e.g. [\"RRULE:FREQ=DAILY;INTERVAL=1\"]; routines may use BYHOUR"`
	RecurringUntil  *string        `json:"recurring_until,omitempty" jsonschema:"YYYY-MM-DD"`
	RewardPoints    *int           `json:"reward_points,omitempty" jsonschema:"points earned on completion"`
	Description     *string        `json:"description,omitempty"`
	UpForGrabs      *bool          `json:"up_for_grabs,omitempty" jsonschema:"anyone can claim it instead of one assigned person"`
	RenewalUnit     *string        `json:"renewal_unit,omitempty" jsonschema:"for chores that renew after completion instead of a fixed schedule (e.g. day, week)"`
	RenewalInterval *int           `json:"renewal_interval,omitempty"`
	TimerSeconds    *int           `json:"timer_seconds,omitempty" jsonschema:"built-in timer length in seconds"`
	Routine         *bool          `json:"routine,omitempty" jsonschema:"true for a routine rather than a one-off chore"`
	EmojiIcon       *string        `json:"emoji_icon,omitempty"`
	Extra           map[string]any `json:"extra,omitempty"`
}

func (f choreFields) body() body {
	b := body{}
	put(b, "summary", f.Summary)
	put(b, "start", f.Start)
	put(b, "start_time", f.StartTime)
	put(b, "recurring", f.Recurring)
	putSlice(b, "recurrence_set", f.RecurrenceSet)
	put(b, "recurring_until", f.RecurringUntil)
	put(b, "reward_points", f.RewardPoints)
	put(b, "routine", f.Routine)
	put(b, "emoji_icon", f.EmojiIcon)
	put(b, "description", f.Description)
	put(b, "up_for_grabs", f.UpForGrabs)
	put(b, "renewal_unit", f.RenewalUnit)
	put(b, "renewal_interval", f.RenewalInterval)
	put(b, "timer_seconds", f.TimerSeconds)
	return b.merge(f.Extra)
}

type taskBoxFields struct {
	Summary      *string        `json:"summary,omitempty"`
	EmojiIcon    *string        `json:"emoji_icon,omitempty"`
	RewardPoints *int           `json:"reward_points,omitempty"`
	Routine      *bool          `json:"routine,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
}

func (f taskBoxFields) body() body {
	b := body{}
	put(b, "summary", f.Summary)
	put(b, "emoji_icon", f.EmojiIcon)
	put(b, "reward_points", f.RewardPoints)
	put(b, "routine", f.Routine)
	return b.merge(f.Extra)
}

func (r *reg) chores() {
	add(r, "list_chores", "List chores/tasks/routines for a date range with their assigned profile, status and reward points.", readOnly,
		func(ctx context.Context, in listChoresIn) (any, error) {
			late := true
			if in.IncludeLate != nil {
				late = *in.IncludeLate
			}
			q := url.Values{"after": {in.After}, "before": {in.Before}, "include_late": {strconv.FormatBool(late)},
				"include_up_for_grabs": {"true"}, "include_schedule_fields": {"true"}}
			v, err := r.call(ctx, http.MethodGet, "/chores", q, nil)
			if err != nil {
				return nil, err
			}
			return filterByRel(v, in.ProfileID, "category"), nil
		})

	add(r, "search_chores", "Search chores by text.", readOnly,
		func(ctx context.Context, in searchIn) (any, error) {
			return r.call(ctx, http.MethodGet, "/chores/search", url.Values{"search_query": {in.Query}}, nil)
		})

	add(r, "create_chore",
		"Create a chore, to-do task or routine for one or more family profiles (one chore per profile is created). Requires summary, start and profile_ids.",
		write, func(ctx context.Context, in struct {
			ProfileIDs []string `json:"profile_ids" jsonschema:"profiles to assign; one chore is created for each"`
			choreFields
		}) (any, error) {
			if in.Summary == nil || len(in.ProfileIDs) == 0 {
				return nil, fmt.Errorf("summary and profile_ids are required")
			}
			b := in.body()
			b["category_ids"] = in.ProfileIDs

			for k, v := range map[string]any{"recurring_until": nil, "routine": false, "start_time": nil, "recurrence_set": nil,
				"up_for_grabs": false, "renewal_interval": nil, "renewal_unit": nil} {
				if _, ok := b[k]; !ok {
					b[k] = v
				}
			}
			return r.call(ctx, http.MethodPost, "/chores/create_multiple", nil, b)
		})

	add(r, "update_chore", "Update a chore. For recurring chores set apply_to (this, this_and_future, all).", write,
		func(ctx context.Context, in struct {
			ChoreID   string `json:"chore_id"`
			ApplyTo   string `json:"apply_to,omitempty"`
			ProfileID string `json:"profile_id,omitempty" jsonschema:"reassign to this profile"`
			choreFields
		}) (any, error) {
			id, err := seg(in.ChoreID)
			if err != nil {
				return nil, err
			}
			b := in.body()
			putStr(b, "apply_to", in.ApplyTo)
			putStr(b, "category_id", in.ProfileID)
			return r.call(ctx, http.MethodPut, "/chores/"+id, nil, b)
		})

	add(r, "complete_chore",
		"Mark a chore completed, skipped, or back to pending. For a specific occurrence of a recurring chore pass instance_date (and profile_id).",
		write, func(ctx context.Context, in struct {
			ChoreID      string `json:"chore_id" jsonschema:"chore id; occurrences of recurring chores look like <seriesId>-YYYY-MM-DD"`
			Status       string `json:"status,omitempty" jsonschema:"completed (default), skipped, or pending"`
			InstanceDate string `json:"instance_date,omitempty" jsonschema:"YYYY-MM-DD of the occurrence (recurring chores)"`
			InstanceTime *int   `json:"instance_time,omitempty"`
			ProfileID    string `json:"profile_id,omitempty"`
		}) (any, error) {
			id, err := seg(in.ChoreID)
			if err != nil {
				return nil, err
			}
			status := in.Status
			if status == "" {
				status = "completed"
			}
			if in.InstanceDate == "" {
				return r.call(ctx, http.MethodPut, "/chores/"+id, nil, body{"status": status})
			}
			b := body{"status": status, "instance_date": in.InstanceDate, "completed_on": in.InstanceDate}
			put(b, "instance_time", in.InstanceTime)
			putStr(b, "category_id", in.ProfileID)
			return r.call(ctx, http.MethodPut, "/chores/"+id+"/completions", nil, b)
		})

	add(r, "delete_chore", "Delete a chore. For recurring chores set apply_to (this, this_and_future, all; default all).", destructive,
		func(ctx context.Context, in struct {
			ChoreID string `json:"chore_id"`
			ApplyTo string `json:"apply_to,omitempty"`
		}) (any, error) {
			id, err := seg(in.ChoreID)
			if err != nil {
				return nil, err
			}
			at := in.ApplyTo
			if at == "" {
				at = "all"
			}
			return r.call(ctx, http.MethodDelete, "/chores/"+id, url.Values{"apply_to": {at}}, body{"apply_to": at})
		})

	add(r, "list_task_box", "List items in the Task Box (unscheduled task inbox / templates).", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/task_box/items", nil, nil)
		})
	add(r, "create_task_box_item", "Add an item to the Task Box.", write,
		func(ctx context.Context, in taskBoxFields) (any, error) {
			return r.call(ctx, http.MethodPost, "/task_box/items", nil, in.body())
		})
	add(r, "update_task_box_item", "Update a Task Box item.", write,
		func(ctx context.Context, in struct {
			ID string `json:"id"`
			taskBoxFields
		}) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPatch, "/task_box/items/"+id, nil, in.body())
		})
	add(r, "delete_task_box_item", "Delete a Task Box item.", destructive,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodDelete, "/task_box/items/"+id, nil, nil)
		})
}
