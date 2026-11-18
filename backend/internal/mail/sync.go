package mail

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"maps"
	"strings"

	"github.com/STGime/laamo/backend/internal/catalog"
	"github.com/STGime/laamo/backend/internal/store"
)

// A helper has two addresses:
//
//   - its address, meetings.x7k2qa@in.laamo.app, for mail the user sends or
//     forwards by hand. Only from these senders applies.
//   - its forwarding address, meetings.x7k2qa+<key>@in.laamo.app, for the
//     user's automatic forwarding rule. Auto-forwarded mail keeps the
//     original sender, so a sender list can't tell it from a stranger's;
//     knowing the key is what counts instead. Mail to it is taken from any
//     sender. The key lives in the email trigger's config and is never
//     shown to anyone but the helper's workspace; a new one can be made
//     when it leaks (NewForwardKey).
const forwardKeyConfig = "forward_key"

// SyncAddress makes a helper's email trigger match its stack. Call it
// whenever a helper version is saved:
//
//   - stack starts with Email this helper: exactly one email trigger with an
//     address and a forwarding key; an existing address and key are kept,
//     so they never change under the user's forwarding rules.
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
	if !wants {
		return nil
	}
	if len(mails) > 0 {
		if ForwardKey(mails[0]) != "" {
			return nil
		}
		// Made before forwarding addresses existed: add a key.
		_, err := setForwardKey(ctx, st, mails[0])
		return err
	}
	for range 5 { // a clash with another helper's random part: try another
		addr := NewAddress(h.Name, domain)
		t := store.Trigger{WorkspaceID: h.WorkspaceID, UserID: h.UserID, HelperID: h.ID, Type: store.TriggerEmail,
			InboundAddress: &addr, Config: map[string]any{forwardKeyConfig: newKey(12)}}
		err := st.CreateTrigger(ctx, &t)
		if !errors.Is(err, store.ErrConflict) {
			return err
		}
	}
	return errors.New("mail: couldn't find a free address")
}

// Addresses are a helper's two addresses; empty when it has none.
type Addresses struct {
	Address string
	Forward string
}

// HelperAddresses are the helper's address and forwarding address.
func HelperAddresses(ctx context.Context, st store.TriggerStore, helperID string) (Addresses, error) {
	t, err := emailTrigger(ctx, st, helperID)
	if err != nil || t == nil {
		return Addresses{}, err
	}
	return addresses(*t), nil
}

// NewForwardKey gives the helper a new forwarding address; the old one stops
// being special (mail to it counts as mail to the plain address).
func NewForwardKey(ctx context.Context, st store.TriggerStore, helperID string) (Addresses, error) {
	t, err := emailTrigger(ctx, st, helperID)
	if err != nil || t == nil {
		return Addresses{}, err
	}
	nt, err := setForwardKey(ctx, st, *t)
	if err != nil {
		return Addresses{}, err
	}
	return addresses(nt), nil
}

// ForwardKey is the key of an email trigger's forwarding address.
func ForwardKey(t store.Trigger) string {
	k, _ := t.Config[forwardKeyConfig].(string)
	return k
}

// IsForwardKey reports whether a "+tag" of an address is the trigger's
// forwarding key.
func IsForwardKey(t store.Trigger, tag string) bool {
	k := ForwardKey(t)
	return k != "" && subtle.ConstantTimeCompare([]byte(k), []byte(strings.ToLower(tag))) == 1
}

func addresses(t store.Trigger) Addresses {
	if t.InboundAddress == nil {
		return Addresses{}
	}
	a := Addresses{Address: *t.InboundAddress}
	if k := ForwardKey(t); k != "" {
		local, domain, _ := strings.Cut(a.Address, "@")
		a.Forward = local + "+" + k + "@" + domain
	}
	return a
}

func emailTrigger(ctx context.Context, st store.TriggerStore, helperID string) (*store.Trigger, error) {
	ts, err := st.HelperTriggers(ctx, helperID)
	if err != nil {
		return nil, err
	}
	for _, t := range ts {
		if t.Type == store.TriggerEmail && t.InboundAddress != nil {
			return &t, nil
		}
	}
	return nil, nil
}

func setForwardKey(ctx context.Context, st store.TriggerStore, t store.Trigger) (store.Trigger, error) {
	cfg := maps.Clone(t.Config)
	if cfg == nil {
		cfg = map[string]any{}
	}
	cfg[forwardKeyConfig] = newKey(12)
	return st.UpdateTrigger(ctx, t.ID, store.TriggerPatch{Config: cfg})
}

// newKey is n random letters and digits that can't be misread: 12 of them
// are 60 bits.
func newKey(n int) string {
	const letters = "abcdefghijkmnpqrstuvwxyz23456789" // no l, o, 0, 1
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	for i := range buf {
		buf[i] = letters[int(buf[i])%len(letters)]
	}
	return string(buf)
}
