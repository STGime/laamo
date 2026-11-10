# Secrets at rest

Secrets that users give Laamo are encrypted before they are stored in Eurobase: chat webhook URLs, Telegram chat ids, Posta keys and (post-MVP) their own AI keys. Code: `backend/internal/crypto`.

## How it works

- **Envelope encryption.** Each secret gets a fresh random 256-bit data key and is sealed with AES-256-GCM. The data key is sealed with a master key (also AES-256-GCM). The stored envelope holds the master key id, the wrapped data key and the ciphertext.
- **Bound to its row.** Each envelope is sealed with associated data `"<table>/<row id>/<column>"` (`crypto.AAD`). An envelope copied into another row or column doesn't decrypt.
- **Who decrypts.** Only workers decrypt, and only when they need the secret (posting to a chat, calling a model). The API stores new secrets and returns only `crypto.Last4` ("…oken"); it never returns plaintext.
- **Plaintext never goes into logs, job payloads, run logs or cards.**

## Master keys

`LAAMO_MASTER_KEYS` holds versioned keys: `v1:<base64>,v2:<base64>`, each 32 random bytes. The last key listed, or `LAAMO_MASTER_KEY_ACTIVE`, encrypts new secrets; every listed key can decrypt.

```sh
echo "v1:$(openssl rand -base64 32)"
```

- **Dev:** put a key in `.env`. Dev keys never go to staging or prod.
- **Staging and prod (MVP):** a Kubernetes secret in the Kapsule namespace, mounted as the env var into the api and worker only (#3). The web image never sees it; `scripts/check-web-secrets.sh` fails if `LAAMO_MASTER_KEY…` appears in `apps/web`.
- **Later:** Scaleway Key Manager holds the master key and unwraps data keys on request. The envelope format already carries the key id, so this is a new key provider behind `Keyring`, not a data migration.

Losing every master key means losing the stored secrets. Users would then re-enter their webhooks and keys; nothing else depends on them. Keep a copy of each key in the team password manager.

## Rotation

Rotation re-wraps only the small data keys, never the secrets themselves.

1. **Add** a new key at the end: `LAAMO_MASTER_KEYS=v1:…,v2:…`, and deploy the api and worker. New secrets use v2; v1 secrets still decrypt.
2. **Re-wrap** stored secrets: for each row with an envelope whose key id is v1 (`crypto.KeyID`), call `Keyring.Rewrap(env, crypto.AAD(table, id, column))` and save the result. Rewrap is idempotent, so the job can be re-run safely. (A small `cmd` tool for this comes with the first table that stores secrets, #34.)
3. **Retire** v1: once no envelope uses v1, remove it from `LAAMO_MASTER_KEYS` and deploy.

If a key may have leaked, do all three steps at once, then ask users to replace their secrets in the external services (webhook URLs can be regenerated in Discord and Slack).

## Open point for #34

The `places.secret_ciphertext` and `brains.key_ciphertext` columns are `bytea`. Eurobase REST sends `bytea` as JSON, so the encoding round trip (a base64 string in, base64 out) has to be checked live before the first secret is stored. If it isn't clean, store the envelope base64-encoded in a `text` column instead (a small migration).
