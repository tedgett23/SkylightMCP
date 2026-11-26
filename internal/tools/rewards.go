package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type rewardFields struct {
	Name                *string        `json:"name,omitempty"`
	Description         *string        `json:"description,omitempty"`
	EmojiIcon           *string        `json:"emoji_icon,omitempty"`
	PointValue          *int           `json:"point_value,omitempty" jsonschema:"points needed to redeem"`
	RespawnOnRedemption *bool          `json:"respawn_on_redemption,omitempty" jsonschema:"reward stays available after being redeemed"`
	Extra               map[string]any `json:"extra,omitempty"`
}

func (f rewardFields) body() body {
	b := body{}
	put(b, "name", f.Name)
	put(b, "description", f.Description)
	put(b, "emoji_icon", f.EmojiIcon)
	put(b, "point_value", f.PointValue)
	put(b, "respawn_on_redemption", f.RespawnOnRedemption)
	return b.merge(f.Extra)
}

func (r *reg) rewards() {
	add(r, "list_rewards", "List rewards with point values, assigned profile and redemption state.", readOnly,
		func(ctx context.Context, in struct {
			RedeemedAtMin string `json:"redeemed_at_min,omitempty" jsonschema:"also include rewards redeemed since this timestamp, e.g. 2026-01-01 00:00:00-05:00"`
			RedeemedAtMax string `json:"redeemed_at_max,omitempty" jsonschema:"upper bound for redeemed_at, e.g. 2026-11-26T23:59:59-07:00"`
			ProfileID     string `json:"profile_id,omitempty"`
		}) (any, error) {
			q := url.Values{}
			if in.RedeemedAtMin != "" {
				q.Set("redeemed_at_min", in.RedeemedAtMin)
			}
			if in.RedeemedAtMax != "" {
				q.Set("redeemed_at_max", in.RedeemedAtMax)
			}
			v, err := r.call(ctx, http.MethodGet, "/rewards", q, nil)
			if err != nil {
				return nil, err
			}
			return filterByRel(v, in.ProfileID, "category"), nil
		})

	add(r, "get_reward", "Get one reward.", readOnly,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodGet, "/rewards/"+id, nil, nil)
		})

	add(r, "create_reward", "Create a reward for one or more profiles (one reward per profile is created). Requires name, point_value, profile_ids.", write,
		func(ctx context.Context, in struct {
			ProfileIDs []string `json:"profile_ids"`
			rewardFields
		}) (any, error) {
			if in.Name == nil || in.PointValue == nil || len(in.ProfileIDs) == 0 {
				return nil, fmt.Errorf("name, point_value and profile_ids are required")
			}
			b := in.body()
			b["category_ids"] = in.ProfileIDs
			if _, ok := b["respawn_on_redemption"]; !ok {
				b["respawn_on_redemption"] = false
			}
			return r.call(ctx, http.MethodPost, "/rewards", nil, b)
		})

	add(r, "update_reward", "Update a reward, or reassign it to a different profile.", write,
		func(ctx context.Context, in struct {
			ID        string `json:"id"`
			ProfileID string `json:"profile_id,omitempty"`
			rewardFields
		}) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			b := in.body()
			putStr(b, "category_id", in.ProfileID)
			return r.call(ctx, http.MethodPatch, "/rewards/"+id, nil, b)
		})

	add(r, "delete_reward", "Delete a reward.", destructive,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodDelete, "/rewards/"+id, nil, nil)
		})

	add(r, "redeem_reward", "Redeem a reward (spends the profile's points). Returns the updated point balance.", write,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPost, "/rewards/"+id+"/redeem", nil, nil)
		})

	add(r, "unredeem_reward", "Undo a reward redemption (refunds the points).", write,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPost, "/rewards/"+id+"/unredeem", nil, nil)
		})

	add(r, "get_reward_points", "Get each profile's current point balance and lifetime points earned.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/reward_points", nil, nil)
		})

	add(r, "award_points", "Give (or, with a negative number, take away) reward points for one or more profiles.", write,
		func(ctx context.Context, in struct {
			ProfileIDs []string `json:"profile_ids"`
			Points     int      `json:"points" jsonschema:"points to add; negative to deduct"`
		}) (any, error) {
			if len(in.ProfileIDs) == 0 {
				return nil, fmt.Errorf("profile_ids is required")
			}
			return r.call(ctx, http.MethodPost, "/reward_points", nil, body{"category_ids": in.ProfileIDs, "points": in.Points})
		})
}
