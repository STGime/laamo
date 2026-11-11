-- Store encrypted secrets as text (base64 of the internal/crypto envelope)
-- instead of bytea (issue #28). The REST API's JSON encoding of bytea isn't
-- something to depend on; base64 text round-trips as is. No secrets are
-- stored yet, so the columns are simply replaced.
ALTER TABLE places DROP COLUMN secret_ciphertext;
ALTER TABLE places ADD COLUMN secret_ciphertext text; -- base64 envelope (webhook URL, chat id, key)
ALTER TABLE brains DROP COLUMN key_ciphertext;
ALTER TABLE brains ADD COLUMN key_ciphertext text;    -- base64 envelope; null when included
