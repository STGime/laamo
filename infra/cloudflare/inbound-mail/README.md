# Incoming email (Cloudflare Email Worker)

Every helper that starts with **Email this helper** gets an address like
`meetings.x7k2qa@in.laamo.app`. Cloudflare Email Routing receives the mail,
this Worker signs it and posts it to `POST /api/inbound/email`, and the API
starts the helper's run (`backend/internal/api/inbound.go`).

## Set up (once)

1. In Cloudflare, add the `laamo.app` zone, then **Email → Email Routing** for
   the subdomain `in.laamo.app`. Cloudflare adds the MX and SPF records.
2. Make a secret and give it to both sides:
   ```sh
   openssl rand -hex 32                  # the secret
   cd infra/cloudflare/inbound-mail
   npx wrangler secret put INBOUND_SECRET
   ```
   and `INBOUND_SECRET=<same value>` in the API's environment.
3. Set `API_URL` in `wrangler.toml` to the API's public URL, then
   `npx wrangler deploy`.
4. **Email Routing → Routing rules → Catch-all address** on `in.laamo.app`:
   action **Send to a Worker**, worker `laamo-inbound-mail`.

## What the sender sees

| API answer | Worker | Sender |
|---|---|---|
| 202 run started / duplicate / ignored | accepts | nothing (no bounce to strangers) |
| 404 no such address | rejects | "There is no Laamo helper at this address." |
| 410 helper off | rejects | "This Laamo helper is switched off." |
| 413 / 422 | rejects | too big / unreadable |
| anything else | throws | the delivery fails and shows in the Worker's logs |

Ignored on purpose (202): a sender not on the helper's *Only from* list or
whose domain didn't sign the mail (DKIM), and more than 30 mails a day to
one helper.

## Test locally (no Cloudflare)

```sh
task mail:send -- termine.x7k2qa@in.laamo.app path/to/mail.eml
```

signs and posts the file the way the Worker does (to `http://localhost:8787`). Without a file it sends a short sample.
