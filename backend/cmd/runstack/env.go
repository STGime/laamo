package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/STGime/laamo/backend/internal/config"
	"github.com/STGime/laamo/backend/internal/crypto"
	"github.com/STGime/laamo/backend/internal/engine"
	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/store"
	sebs "github.com/STGime/laamo/backend/internal/store/eurobase"
	"github.com/STGime/laamo/backend/internal/store/memory"
)

// runEnv is where a run lives: in memory (default) or in a real Eurobase
// workspace (-workspace).
type runEnv struct {
	st    store.Store
	ws    store.Workspace
	keys  *crypto.Keyring
	blobs engine.Blobs
	// helper creates the helper and its version.
	helper func(ctx context.Context, name, avatar string, stack []byte) (store.Helper, error)
}

func setup(ctx context.Context, o options, raw []byte, path string) (runEnv, error) {
	if o.workspace != "" {
		return workspaceEnv(ctx, o)
	}
	st := memory.New()
	profile := map[string]any{}
	for _, kv := range strings.Split(o.profile, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.TrimSpace(k) != "" {
			profile[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	user := store.NewID()
	ws := st.PutWorkspace(store.Workspace{UserID: user, Name: "runstack", Timezone: o.tz, Locale: o.lang, Profile: profile})
	st.PutBrain(store.Brain{WorkspaceID: ws.ID, UserID: user, AllowanceCredits: 500})
	keyBytes := make([]byte, 32)
	_, _ = rand.Read(keyBytes)
	keys, _ := crypto.ParseKeyring("dev:"+base64.StdEncoding.EncodeToString(keyBytes), "")
	return runEnv{st: st, ws: ws, keys: keys, blobs: &engine.MemoryBlobs{},
		helper: func(_ context.Context, name, avatar string, stack []byte) (store.Helper, error) {
			h := st.PutHelper(store.Helper{WorkspaceID: ws.ID, UserID: user, Name: name, Avatar: nonEmpty(avatar), Status: "active"})
			v := st.PutHelperVersion(store.HelperVersion{WorkspaceID: ws.ID, UserID: user, HelperID: h.ID, Version: 1, Stack: stack})
			h.CurrentVersionID = &v.ID
			return st.PutHelper(h), nil
		}}, nil
}

// workspaceEnv writes to a real workspace: its language, About me facts and
// brain apply, and the run's cards show up in the app.
func workspaceEnv(ctx context.Context, o options) (runEnv, error) {
	cfg, err := config.Load()
	if err != nil {
		return runEnv{}, err
	}
	if cfg.EurobaseSecretKey == "" {
		return runEnv{}, errors.New("-workspace needs EUROBASE_PRIVATE_KEY in the environment")
	}
	// Post to chat needs the real master keys (the worker must decrypt
	// what runstack stores); other stacks don't.
	keys, _ := crypto.ParseKeyring(cfg.MasterKeys, cfg.MasterKeyActive)
	c := eb.New(cfg.EurobaseURL, cfg.EurobaseSecretKey)
	st := sebs.New(c)
	ws, err := st.GetWorkspace(ctx, o.workspace)
	if err != nil {
		return runEnv{}, fmt.Errorf("workspace %s: %w", o.workspace, err)
	}
	return runEnv{st: st, ws: ws, keys: keys, blobs: eb.Blobs{Client: c, Prefix: "laamo/"},
		helper: func(ctx context.Context, name, avatar string, stack []byte) (store.Helper, error) {
			h := store.Helper{ID: store.NewID(), WorkspaceID: ws.ID, UserID: ws.UserID, Name: name, Avatar: nonEmpty(avatar),
				Status: "active", CreatedVia: "scratch"}
			if err := c.Insert(ctx, "helpers", h.ID, h, nil); err != nil {
				return h, err
			}
			v := store.HelperVersion{ID: store.NewID(), WorkspaceID: ws.ID, UserID: ws.UserID, HelperID: h.ID, Version: 1, Stack: stack}
			if err := c.Insert(ctx, "helper_versions", v.ID, v, nil); err != nil {
				return h, err
			}
			h.CurrentVersionID = &v.ID
			return h, c.Update(ctx, "helpers", h.ID, map[string]string{"current_version_id": v.ID}, nil)
		}}, nil
}

func helperAvatar(raw []byte) string {
	var t struct {
		Avatar string `json:"avatar"`
	}
	_ = json.Unmarshal(raw, &t)
	return t.Avatar
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
