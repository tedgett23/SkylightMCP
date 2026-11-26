# skylight-mcp

An MCP server (Go, streamable HTTP) for a Skylight Calendar frame: calendar events, family profiles, chores/routines, task box, lists, rewards & points, meal planning, nudges, device alarms and photos.

Built against the unofficial, reverse-engineered API (`../skylight-api`). Skylight can change it at any time.

## Run

```bash
cp config.sample.yaml config.yaml   # fill in email/password and an auth_token
go build -o skylight-mcp .
./skylight-mcp -config config.yaml -check   # log in, print frame id, exit
./skylight-mcp -config config.yaml          # serve on :8080/mcp
```

Connect a client to `http://<host>:8080/mcp` and send `Authorization: Bearer <auth_token>`. For example, with Claude Code:

```bash
claude mcp add --transport http skylight http://localhost:8080/mcp --header "Authorization: Bearer <auth_token>"
```

`GET /healthz` returns `ok`.

## Docker

Two-stage build: `golang:1.25-alpine` compiles a static `CGO_ENABLED=0` binary, which runs on `distroless/static` as a non-root user with no shell.

```bash
docker compose up -d --build        # uses ./config.yaml, token cache in the skylight-data volume
# or
docker build -t skylight-mcp .
docker run -d -p 8080:8080 -v $PWD/config.yaml:/config/config.yaml:ro -v skylight-data:/data skylight-mcp
```

`server.listen` must be `:8080` (all interfaces), not `127.0.0.1:8080`. The image's HEALTHCHECK runs `skylight-mcp -healthcheck http://127.0.0.1:8080/healthz`.

## Auth

`auth_method: auto` first tries the current app's OAuth + PKCE login: it fetches the login form, posts your credentials, catches the redirect code, and exchanges it for access and refresh tokens. If that fails it falls back to the legacy `POST /api/sessions` login with Basic auth. Tokens are cached in `token_cache` (mode 0600), refreshed when they expire, and renewed automatically on a 401.

## Tools

Start with `skylight_overview`, which returns the frame, profiles, lists and calendars along with their ids. In the API, "categories" are family profiles, so `profile_id(s)` arguments take category ids.

| Area | Tools |
|---|---|
| Calendar | list_events, search_events, list_countdowns, create_event, update_event, delete_event, list_calendars, set_default_calendar, add_webcal_subscription, remove_source_calendar, link_profiles_to_calendar, get/update_event_notification_settings |
| Profiles | list_profiles, get_profile, create_profile, update_profile, delete_profile, update_family_member, link_calendars_to_profile |
| Chores | list_chores, search_chores, create_chore, update_chore, complete_chore, delete_chore, list/create/update/delete task box items |
| Lists | list_lists, get_list, create_list, update_list, delete_list, add_list_items, update_list_item, move_list_item, set_list_items_section, delete_list_items |
| Rewards | list_rewards, get_reward, create_reward, update_reward, delete_reward, redeem_reward, unredeem_reward, get_reward_points, award_points |
| Meals | list_meal_categories, update_meal_category, list/get/create/update/delete recipe, add_recipe_to_grocery_list, list_meal_plan, add_meal_to_plan, update_planned_meal, delete_planned_meal |
| Other | nudges CRUD, devices + alarms CRUD, photos (list/get/caption/delete), albums, household config, frame users, activity feed, sidekick intents, user, frame, plus access, colors, avatars |
| Escape hatch | `skylight_api_request`: any method/path (with a `{frame_id}` placeholder) |

Most create/update tools also accept an `extra` object that is merged into the request body, for fields that aren't modelled.

## Verified vs. inferred

Request bodies for these tools come from captured traffic: events (list/create/update/delete), profiles (get/update/family member), chores (list/create/update status), lists and list items (create/update/bulk section), rewards (all), reward points, recipes, and meal sittings (list/create).

Search and nudge listing were confirmed against a live account. The rest are built from endpoint names only and may need adjusting: chore `completions`, task box create, list item move/bulk delete, nudges, alarms, set_default_calendar, webcal, calendar linking, planned-meal update/delete, and the `start_time` encoding. You can fix them by capturing a HAR in the web app and running it through `../skylight-api`'s converter.
