# Testing the API locally

The API runs on your machine against the **laamo dev project** on Eurobase. Everything you create there is dev data.

## 1. Start the API

```sh
task dev:api          # API on http://localhost:8787 (needs EUROBASE_PRIVATE_KEY in .env)
curl localhost:8787/api/healthz
# {"env":"dev","eurobase":"reachable","status":"ok"}
```

`task dev` starts the API, the worker and the web app together.

## 2. Get an access token

Sign-in is Eurobase auth. `scripts/dev-token.sh` signs in and prints an access token (valid for 1 hour). It asks for the password, or reads it from `LAAMO_TEST_PASSWORD`.

```sh
TOKEN=$(scripts/dev-token.sh --signup you+laamo@example.com)   # first time: creates the user
TOKEN=$(scripts/dev-token.sh you+laamo@example.com)            # afterwards
```

If the project requires email confirmation, `--signup` says so: click the link, then run it again without `--signup`.

## 3. Call it

```sh
A="Authorization: Bearer $TOKEN"
curl -H "$A" localhost:8787/api/me
curl -H "$A" -H 'Content-Type: application/json' \
  -d '{"name":"Bäckerei Krause","timezone":"Europe/Berlin","locale":"de"}' localhost:8787/api/workspaces
curl -X PATCH -H "$A" -H 'Content-Type: application/json' \
  -d '{"profile":{"city":"Leipzig","business":"bakery"}}' localhost:8787/api/workspaces/<id>
```

**API tools:** import `http://localhost:8787/api/openapi.yaml` into Postman, Bruno or Insomnia, set the auth to Bearer Token, and paste `$TOKEN`. Or use `docs/api.http` in VS Code (REST Client) or JetBrains.

## What exists so far

| Endpoint | What it does |
|---|---|
| `GET /api/healthz` | Service health (no token needed) |
| `GET /api/openapi.yaml` | The contract (no token needed) |
| `GET /api/me` | You and your workspaces (empty for a new user) |
| `POST /api/workspaces` | Create a workspace. You become the owner; the included brain is set up too. At most 5 per user. |
| `GET /api/workspaces/{id}` | One of your workspaces. Someone else's gives 404. |
| `PATCH /api/workspaces/{id}` | Name, timezone, language, About me facts (owner only). An empty fact value forgets that fact. |

Errors are always `{"error": "...", "code": "..."}`, with code `bad_request`, `unauthorized`, `forbidden`, `not_found`, `auth_unavailable` or `internal`.

## Things worth trying

- Call any endpoint without a token, with a made-up token, or with an expired one (after 1 hour) → 401.
- Create a workspace with `"timezone":"Mars/Olympus"` or `"locale":"fr"` → 400 with the reason.
- Create a 6th workspace → 400.
- Sign up a second user and open the first user's workspace id → 404.
- Set About me facts: allowed keys are `name`, `business`, `offer`, `city`, `website`, `tone` and `signature`. Anything else → 400.

## The Feed in the app, with real runs

1. Start everything: `task dev` (api on :8787, worker, web on :5173).
2. Sign in: open http://localhost:5173, tap "Make my first helper", enter your email and click the link in the email (see "Sign-in setup" below). Or, without email, paste a token from `scripts/dev-token.sh you@example.com` on the dev screen (`/signin`).
3. Run a helper into your workspace (the id comes from `GET /api/me`):

   ```sh
   cd backend
   go run ./cmd/runstack -workspace <workspace-id> ../examples/stacks/news-deep-read.json   # a Result card
   go run ./cmd/runstack -workspace <workspace-id> ../examples/stacks/news-check-first.json # a question, then Check first
   ```

   Cards appear in the Feed live. Answer an Ask me card in the app: the API queues the run, and the worker carries on. With `-workspace`, runstack uses real search and AI, the workspace's language, About me facts and brain allowance. A stack that posts to chat also needs `-webhook` and `LAAMO_MASTER_KEYS`.

Live updates come from `GET /api/workspaces/{id}/events` (Server-Sent Events). The api holds one Eurobase realtime connection and forwards each member only their workspace's events.

## Sign-in setup (Eurobase auth settings)

The app signs in with magic links, straight from the browser to Eurobase auth using the project's **public** key, read from `EUROBASE_URL` and `EUROBASE_PUBLIC_KEY` in the repo's `.env` at build time.

1. A new email gets an account with a random password nobody sees (an existing email just fails).
2. The app requests a magic link to `<origin>/auth/callback`.
3. Clicking the link signs in.

New and returning people see the same screens.

The Eurobase project's auth settings need:

- **Magic link enabled** and **email confirmation off**. The link itself proves the email; with confirmation on, people would get two emails.
- **redirect_urls**, exact matches: `http://localhost:5173/auth/callback`, and later `https://laamo.app/auth/callback`.
- **cors_origins**: `http://localhost:5173`, and later `https://laamo.app`.

The magic link email uses Eurobase's default template (English). A German Laamo template can be set per project.
