package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

var mealInclude = url.Values{"include": {"meal_category"}}

type recipeFields struct {
	Summary        *string        `json:"summary,omitempty" jsonschema:"recipe name"`
	Description    *string        `json:"description,omitempty" jsonschema:"ingredients and instructions (free text)"`
	MealCategoryID *string        `json:"meal_category_id,omitempty" jsonschema:"breakfast/lunch/dinner/snack category id (see list_meal_categories)"`
	Extra          map[string]any `json:"extra,omitempty"`
}

func (f recipeFields) body() body {
	b := body{}
	put(b, "summary", f.Summary)
	put(b, "description", f.Description)
	put(b, "meal_category_id", f.MealCategoryID)
	return b.merge(f.Extra)
}

func (r *reg) meals() {
	add(r, "list_meal_categories", "List meal categories (Breakfast, Lunch, Dinner, Snack...).", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/meals/categories", nil, nil)
		})

	add(r, "update_meal_category", "Rename, recolor, enable/disable a meal category.", write,
		func(ctx context.Context, in struct {
			ID      string         `json:"id"`
			Label   *string        `json:"label,omitempty"`
			Color   *string        `json:"color,omitempty"`
			Enabled *bool          `json:"enabled,omitempty"`
			Extra   map[string]any `json:"extra,omitempty"`
		}) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			b := body{}
			put(b, "label", in.Label)
			put(b, "color", in.Color)
			put(b, "enabled", in.Enabled)
			return r.call(ctx, http.MethodPatch, "/meals/categories/"+id, nil, b.merge(in.Extra))
		})

	add(r, "list_recipes", "List recipes in the recipe box.", readOnly,
		func(ctx context.Context, _ noInput) (any, error) {
			return r.call(ctx, http.MethodGet, "/meals/recipes", mealInclude, nil)
		})
	add(r, "get_recipe", "Get one recipe.", readOnly,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodGet, "/meals/recipes/"+id, mealInclude, nil)
		})
	add(r, "create_recipe", "Add a recipe to the recipe box.", write,
		func(ctx context.Context, in recipeFields) (any, error) {
			if in.Summary == nil {
				return nil, fmt.Errorf("summary is required")
			}
			return r.call(ctx, http.MethodPost, "/meals/recipes", mealInclude, in.body())
		})
	add(r, "update_recipe", "Update a recipe.", write,
		func(ctx context.Context, in struct {
			ID string `json:"id"`
			recipeFields
		}) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPatch, "/meals/recipes/"+id, mealInclude, in.body())
		})
	add(r, "delete_recipe", "Delete a recipe.", destructive,
		func(ctx context.Context, in struct {
			ID              string `json:"id"`
			ApplyToSittings bool   `json:"apply_to_sittings,omitempty" jsonschema:"also remove it from the meal plan"`
		}) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			q := url.Values{"apply_to_sittings": {""}}
			if in.ApplyToSittings {
				q.Set("apply_to_sittings", "true")
			}
			return r.call(ctx, http.MethodDelete, "/meals/recipes/"+id, q, nil)
		})
	add(r, "add_recipe_to_grocery_list", "Add a recipe's ingredients to the default grocery list (processed by Skylight's AI importer).", write,
		func(ctx context.Context, in idIn) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPost, "/meals/recipes/"+id+"/add_to_grocery_list", nil, nil)
		})

	add(r, "list_meal_plan", "List planned meals (sittings) in a date range.", readOnly,
		func(ctx context.Context, in struct {
			DateMin string `json:"date_min" jsonschema:"YYYY-MM-DD"`
			DateMax string `json:"date_max" jsonschema:"YYYY-MM-DD"`
		}) (any, error) {
			q := url.Values{"date_min": {in.DateMin}, "date_max": {in.DateMax}, "include": {"meal_category,meal_recipe,profiles"}}
			return r.call(ctx, http.MethodGet, "/meals/sittings", q, nil)
		})

	add(r, "add_meal_to_plan", "Plan a meal on a date, from a saved recipe (meal_recipe_id) or ad hoc (summary).", write,
		func(ctx context.Context, in struct {
			Date             string         `json:"date" jsonschema:"YYYY-MM-DD"`
			MealCategoryID   string         `json:"meal_category_id"`
			MealRecipeID     string         `json:"meal_recipe_id,omitempty"`
			Summary          string         `json:"summary,omitempty" jsonschema:"meal name when not using a saved recipe"`
			Description      string         `json:"description,omitempty"`
			Note             string         `json:"note,omitempty"`
			RRule            []string       `json:"rrule,omitempty" jsonschema:"repeat rule, e.g. [\"RRULE:FREQ=WEEKLY\"]"`
			AddToGroceryList bool           `json:"add_to_grocery_list,omitempty"`
			SaveToRecipeBox  bool           `json:"save_to_recipe_box,omitempty"`
			Extra            map[string]any `json:"extra,omitempty"`
		}) (any, error) {
			b := body{"date": in.Date, "meal_category_id": in.MealCategoryID, "note": in.Note,
				"add_to_grocery_list": in.AddToGroceryList, "saveToRecipeBox": in.SaveToRecipeBox,
				"meal_recipe_id": nil, "summary": nil, "description": nil, "rrule": nil}
			putStr(b, "meal_recipe_id", in.MealRecipeID)
			putStr(b, "summary", in.Summary)
			putStr(b, "description", in.Description)
			if len(in.RRule) > 0 {
				b["rrule"] = in.RRule
			}
			q := url.Values{"date_min": {in.Date}, "date_max": {in.Date}, "include": {"meal_category,meal_recipe,profiles"}}
			return r.call(ctx, http.MethodPost, "/meals/sittings", q, b.merge(in.Extra))
		})

	add(r, "update_planned_meal", "Update a planned meal (sitting).", write,
		func(ctx context.Context, in struct {
			ID     string         `json:"id"`
			Fields map[string]any `json:"fields" jsonschema:"fields to change, e.g. note, summary, meal_recipe_id, meal_category_id, rrule, date"`
		}) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			return r.call(ctx, http.MethodPatch, "/meals/sittings/"+id, nil, in.Fields)
		})

	add(r, "delete_planned_meal", "Remove a planned meal (sitting) from the meal plan.", destructive,
		func(ctx context.Context, in struct {
			ID      string `json:"id"`
			ApplyTo string `json:"apply_to,omitempty" jsonschema:"for recurring meals: this, this_and_future, or all"`
			Date    string `json:"date,omitempty" jsonschema:"occurrence date for recurring meals"`
		}) (any, error) {
			id, err := seg(in.ID)
			if err != nil {
				return nil, err
			}
			q := url.Values{}
			if in.ApplyTo != "" {
				q.Set("apply_to", in.ApplyTo)
			}
			if in.Date != "" {
				q.Set("date", in.Date)
			}
			return r.call(ctx, http.MethodDelete, "/meals/sittings/"+id, q, nil)
		})
}
