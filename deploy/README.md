# Deploy

Laamo runs on **Scaleway Serverless Containers in Paris (fr-par)**, the same
region as Eurobase, where all its data lives. One namespace per environment
(`laamo-staging`, `laamo-prod`), three containers each:

| Container | What | Size | Scale |
|---|---|---|---|
| `api` | HTTP API (`backend/cmd/api`) | 512 MB / 280 mvCPU | staging 0–3, prod 1–5 |
| `worker` | runs, schedules, outbox (`backend/cmd/worker`) | 1 GB / 560 mvCPU | always 1 |
| `web` | the app (static) + nginx forwarding `/api` to `api` | 128 MB / 70 mvCPU | staging 0–3, prod 1–5 |

The browser only talks to `web` (one domain, no CORS); `/api/...` goes on to
the `api` container. The worker takes no requests: it answers `/healthz` on
`HEALTH_ADDR` so Scaleway can see it's alive.

Incoming email arrives through Cloudflare (`infra/cloudflare/inbound-mail`):
its `API_URL` is `https://<web domain>/api/inbound/email`.

## First time

1. `scw init` (Scaleway account, project, credentials), Docker with buildx, `jq`.
2. `cp deploy/env.example deploy/staging.env` and fill it in (gitignored).
   Use a separate Eurobase project for staging, with its migrations applied
   (`task migrate`).
3. `task deploy:staging`
4. Laamo is invite-only, and the app has no sign-up. Invite yourself first:
   `ENV=staging task invite -- -send <your email>`, then use the email's link.
   Add the web domain to the Eurobase project's redirect URLs
   (`https://<domain>/auth/callback`) and CORS origins, or sign-in links fail.

The script creates the registry namespace, the container namespace and the
containers the first time, and updates them after that. It prints the web
and api URLs at the end and checks `/api/healthz` through the web container.

## Every deploy

```sh
task deploy:staging          # build, push and roll out the current commit
SKIP_BUILD=1 TAG=abc1234 deploy/deploy.sh staging   # images already pushed
task deploy:prod             # asks first
```

Images are tagged with the git commit (`api:abc1234`, `web:staging-abc1234`;
the web bundle holds the environment's Eurobase URL and public key, so it's
built per environment). Uncommitted changes are refused unless `ALLOW_DIRTY=1`.

To go back: `SKIP_BUILD=1 TAG=<older commit> deploy/deploy.sh staging`.

## Domains

Set `WEB_DOMAIN` (e.g. `staging.laamo.app`) in the env file, point a CNAME
at the web container's host name (printed by the script), and deploy again:
the script attaches the domain and Scaleway issues the certificate.

DNS overview (issue #5): `laamo.app` (site + app), `in.laamo.app` (incoming
mail, Cloudflare Email Routing), `notify.laamo.app` (outgoing, Resend).

## Staging login

Staging is for testing before prod, so nobody else should see it: with
`WEB_LOGIN_USER` and `WEB_LOGIN_PASSWORD` in `deploy/staging.env`, the web
container asks for that login (HTTP Basic auth in nginx) on every page.
The script sends only the password's hash. Left out:

- `/api/`: the API has its own sign-in and uses the same `Authorization`
  header for it; it stays invite-only.
- `/healthz`: Scaleway's health check.

- `/manifest.webmanifest`, the home-screen icons and `/sw.js`: a phone
  fetches these itself, without the login. Behind it, an iPhone couldn't put
  Laamo on the Home Screen, which notifications need there.

Prod and `docker run` locally leave the two lines empty, so there's no login.
The flow is: local (no login) → `task deploy:staging` (login) →
`task deploy:prod`.

## Notifications

People can get a notification on their phone or desktop when a helper puts
something in the Feed (Web Push, no native app). It needs a key pair,
`VAPID_PUBLIC_KEY` and `VAPID_PRIVATE_KEY` in the env file:

```sh
cd backend && go run ./cmd/vapid    # prints the two lines
```

- **One pair for every environment that shares a database** (today: local,
  staging and prod), like `LAAMO_MASTER_KEYS`. A browser's subscription only
  works with the key it was made for, and any worker may run any
  environment's jobs.
- **Don't change it.** A new pair cuts off every device; each person has to
  turn notifications on again.
- Without the keys notifications are off and the app doesn't offer them.
- A push goes to the browser maker's push service (Google for Chrome and
  Android, Apple for Safari and iPhones, Mozilla, Microsoft), which hands it
  to the device. These are US companies. The text is encrypted for the
  browser, so they can't read it; they do see that Laamo sent this device
  something, and when. Native apps would use the same services.
- iPhone and iPad: only once Laamo is on the Home Screen, iOS 16.4 or later.

## Social posting (Posta)

Helpers post to social media through Posta (getposta.app). `POSTA_PARTNER_KEY`
in the env file lets the api create a Posta account for a user who switches
social posting on (Me → Social media), and delete it when they switch it off.

- **No sandbox.** Staging and local dev talk to the real Posta: accounts are
  real, and published posts really go out. Use throwaway emails; each email
  and each Laamo user gets one 5-day trial, ever.
- **After the trial** posting stops until the account has a plan. Laamo has
  no billing yet, so set it by hand:

  ```sh
  cd backend && set -a && . ../.env && set +a
  go run ./cmd/posta list                       # who has it on, and their plan
  go run ./cmd/posta plan <laamo user id> starter 30
  ```

- **Results come back by webhook.** Before an account's first post the
  worker registers `SITE_URL/api/posta/webhook/<place id>` at Posta, and
  Posta calls it when a post went out or failed (a Feed card and a
  notification). `SITE_URL` must be https, so local dev registers nothing.
  All environments share the database: whichever posts first for an
  account registers its own address, and that one keeps getting the calls.
- **People with their own Posta account** paste its API key instead. Their
  plan and channels stay with Posta, and the partner key can't touch them.
- The key is all-powerful for the accounts Laamo created. It never goes into
  the web app (`scripts/check-web-secrets.sh` checks) or the repo; a lost key
  is replaced by Posta, not shown again.

## To check on staging

- **Client IP.** `LAAMO_TRUST_PROXY` is off: behind web's nginx and Scaleway's
  own proxies the API can't yet tell the user's IP, so the public demo's
  per-IP limit counts everyone together. Look at the `X-Forwarded-For` the
  api receives, then pick the right hop.
- **Worker always on.** `min-scale=1` should keep one worker running without
  requests. Watch that scheduled helpers start on time.
- **Live updates.** The Feed's event stream goes through two proxies; the
  container timeout is 15 minutes, after which the browser reconnects.
