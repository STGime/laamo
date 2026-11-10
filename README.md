# Laamo

Little AI helpers, built from bricks. Sign up, pick or describe a helper, snap 4–6 bricks together, and results land in your Feed. EU-hosted on Eurobase.

- Product plan: https://app.notion.com/p/4127383ff7d1819b9227ff7ea68f435e
- MVP implementation plan: https://app.notion.com/p/4137383ff7d1810b882deed096ba811b
- Use cases: https://app.notion.com/p/4137383ff7d1818a9732dafafeead668

## Layout

```
apps/web/            SvelteKit SPA (one centered column on every screen size)
backend/             one Go module
  cmd/api            HTTP API
  cmd/worker         run engine + outbound (job queue on Eurobase)
  cmd/mailin         SMTP receiver for helper addresses
  cmd/runstack       run a stack JSON locally
  cmd/overpassmock   local Overpass stand-in (dev only)
  internal/          catalog, engine, bricks/{when,look,think,do,ask}, llm, search, fetch,
                     osm, mail, chat, billing, auth, crypto, store, queue, config
packages/catalog/    bricks.json + templates (single source of truth for bricks)
packages/openapi/    API contract
migrations/          SQL migrations for Eurobase (+ local RLS tests)
deploy/              Dockerfiles, Kapsule manifests, DNS notes
assets/generated/    dev-only generated media (tracked in its README)
```

## Getting started

Needs Go 1.26+, Node 22+, pnpm 10, Docker and [Task](https://taskfile.dev) (`brew install go-task`).

```sh
cp .env.example .env     # fill in the Eurobase dev project keys
task setup
task up                  # Mailpit (http://localhost:8025) + Overpass mock (:8081)
task dev                 # web on http://localhost:5173, api on :8787
```

Other tasks: `task test`, `task lint`, `task build`, `task --list`.

## Rules

- All data lives in Eurobase (EU). The Eurobase secret key is used only by Go services, never in the web app.
- Schema changes go in `migrations/` (see its README), never by hand on a live project.
- UI follows the Look & feel rules in the product plan: tokens only (`apps/web/src/lib/styles/tokens.css`), no gradients, glows, soft shadows or scroll animations.
