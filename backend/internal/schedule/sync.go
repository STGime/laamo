package schedule

import (
	"context"
	"errors"
	"time"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/store"
)

// Sync makes a helper's schedule trigger match its stack. Call it whenever a
// helper version is saved or the helper is switched on or off:
//
//   - stack starts with Every…: exactly one schedule trigger with the
//     brick's settings; next_run_at is the next slot while the helper is
//     active, NULL while it isn't (the scheduler skips NULL).
//   - anything else: no schedule trigger.
func Sync(ctx context.Context, st store.TriggerStore, cat *catalog.Catalog, h store.Helper, ws store.Workspace,
	stack catalog.Stack, now time.Time) error {
	existing, err := st.HelperTriggers(ctx, h.ID)
	if err != nil {
		return err
	}
	var schedules []store.Trigger
	for _, t := range existing {
		if t.Type == store.TriggerSchedule {
			schedules = append(schedules, t)
		}
	}

	var every *catalog.StackBrick
	if len(stack.Bricks) > 0 && stack.Bricks[0].Type == "when.every" {
		every = &stack.Bricks[0]
	}
	if every == nil {
		for _, t := range schedules {
			if err := st.DeleteTrigger(ctx, t.ID); err != nil {
				return err
			}
		}
		return nil
	}

	b, _ := cat.Brick("when.every")
	full := b.Config(every.Config)
	cfg := ConfigFrom(full)
	config := map[string]any{"freq": cfg.Freq, "time": cfg.Time}
	if cfg.Freq == "weekly" {
		config["days"] = cfg.Days
	}

	next := store.Null[time.Time]()
	if h.Status == "active" {
		at, err := Next(cfg, ws.Timezone, now)
		switch {
		case err == nil:
			next = store.Set(at)
		case !errors.Is(err, ErrNever):
			return err
		}
	}

	if len(schedules) == 0 {
		t := store.Trigger{WorkspaceID: h.WorkspaceID, UserID: h.UserID, HelperID: h.ID, Type: store.TriggerSchedule, Config: config}
		if next.Valid {
			t.NextRunAt = &next.Value
		}
		return st.CreateTrigger(ctx, &t)
	}
	if _, err := st.UpdateTrigger(ctx, schedules[0].ID, store.TriggerPatch{Config: config, NextRunAt: next}); err != nil {
		return err
	}
	for _, t := range schedules[1:] {
		if err := st.DeleteTrigger(ctx, t.ID); err != nil {
			return err
		}
	}
	return nil
}

// ConfigFrom reads an Every… brick config (defaults applied).
func ConfigFrom(cfg map[string]any) Config {
	c := Config{}
	c.Freq, _ = cfg["freq"].(string)
	c.Time, _ = cfg["time"].(string)
	switch days := cfg["days"].(type) {
	case []any:
		for _, d := range days {
			if s, ok := d.(string); ok {
				c.Days = append(c.Days, s)
			}
		}
	case []string:
		c.Days = days
	}
	return c
}
