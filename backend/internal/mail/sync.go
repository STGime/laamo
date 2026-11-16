package mail

import (
	"context"
	"errors"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/store"
)

// SyncAddress makes a helper's email trigger match its stack. Call it
// whenever a helper version is saved:
//
//   - stack starts with Email this helper: exactly one email trigger with an
//     address; an existing address is kept, so it never changes under the
//     user's forwarding rules.
//   - anything else: no email trigger (the address stops working).
func SyncAddress(ctx context.Context, st store.TriggerStore, h store.Helper, stack catalog.Stack, domain string) error {
	existing, err := st.HelperTriggers(ctx, h.ID)
	if err != nil {
		return err
	}
	var mails []store.Trigger
	for _, t := range existing {
		if t.Type == store.TriggerEmail {
			mails = append(mails, t)
		}
	}
	wants := len(stack.Bricks) > 0 && stack.Bricks[0].Type == "when.email"
	keep := 0
	if wants {
		keep = 1
	}
	for _, t := range mails[min(keep, len(mails)):] {
		if err := st.DeleteTrigger(ctx, t.ID); err != nil {
			return err
		}
	}
	if !wants || len(mails) > 0 {
		return nil
	}
	for range 5 { // a clash with another helper's random part: try another
		addr := NewAddress(h.Name, domain)
		t := store.Trigger{WorkspaceID: h.WorkspaceID, UserID: h.UserID, HelperID: h.ID, Type: store.TriggerEmail, InboundAddress: &addr}
		err := st.CreateTrigger(ctx, &t)
		if !errors.Is(err, store.ErrConflict) {
			return err
		}
	}
	return errors.New("mail: couldn't find a free address")
}

// HelperAddress is the helper's email address, or "" if it has none.
func HelperAddress(ctx context.Context, st store.TriggerStore, helperID string) (string, error) {
	ts, err := st.HelperTriggers(ctx, helperID)
	if err != nil {
		return "", err
	}
	for _, t := range ts {
		if t.Type == store.TriggerEmail && t.InboundAddress != nil {
			return *t.InboundAddress, nil
		}
	}
	return "", nil
}
