// Command posta manages the Posta accounts Laamo created for its users,
// by hand, until Laamo has billing:
//
//	go run ./cmd/posta list                          who has social posting on
//	go run ./cmd/posta show <laamo user id>          plan and channels
//	go run ./cmd/posta plan <laamo user id> starter [days]
//	go run ./cmd/posta plan <laamo user id> none     ends a paid plan now
//	go run ./cmd/posta usage                         this month's numbers
//
// A plan is starter, professional or none. With days, it ends by itself
// after that many days unless it is set again; without, it runs until
// changed. It reads POSTA_PARTNER_KEY (and the Eurobase keys for list) from
// the environment: `set -a; . ../.env; set +a` first.
//
// There is no Posta sandbox: this changes real accounts.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/STGime/laamo/backend/internal/config"
	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/posta"
	"github.com/STGime/laamo/backend/internal/store"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var errUsage = errors.New("usage: posta list | show <user id> | plan <user id> starter|professional|none [days] | usage")

func run(ctx context.Context, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	svc := posta.New(cfg.PostaPartnerKey, cfg.PostaURL)
	if svc == nil {
		return errors.New("POSTA_PARTNER_KEY is not set")
	}
	if len(args) == 0 {
		return errUsage
	}
	switch args[0] {
	case "list":
		if cfg.EurobaseSecretKey == "" {
			return errors.New("EUROBASE_PRIVATE_KEY is not set")
		}
		c := eb.New(cfg.EurobaseURL, cfg.EurobaseSecretKey)
		places, err := eb.ListAll[store.Place](ctx, c, "places", eb.NewQuery().Eq("type", "posta").Order("created_at", false), 0)
		if err != nil {
			return err
		}
		for _, p := range places {
			source, _ := p.Meta["source"].(string)
			if source == "pasted" {
				fmt.Printf("%s  their own Posta account (workspace %s)\n", p.UserID, p.WorkspaceID)
				continue
			}
			id, _ := p.Meta["external_user_id"].(string)
			a, err := svc.Partner.GetAccount(ctx, id)
			if err != nil {
				fmt.Printf("%s  %v\n", id, err)
				continue
			}
			fmt.Printf("%s  %s\n", id, describe(a))
		}
		if len(places) == 0 {
			fmt.Println("nobody has social posting on")
		}
		return nil
	case "usage":
		u, err := svc.Partner.Usage(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("accounts: %d (%d linked, %d new this month)\ntrials: %d, starter: %d, professional: %d\nposts this month: %d\n",
			u.AccountsTotal, u.AccountsLinked, u.AccountsCreatedInPeriod, u.ActiveTrials, u.ActiveStarter, u.ActiveProfessional, u.PostsInPeriod)
		return nil
	case "show":
		if len(args) != 2 {
			return errUsage
		}
		a, err := svc.Partner.GetAccount(ctx, args[1])
		if err != nil {
			return err
		}
		fmt.Println(describe(a))
		return nil
	case "plan":
		if len(args) < 3 || len(args) > 4 {
			return errUsage
		}
		plan := args[2]
		if plan != "starter" && plan != "professional" && plan != "none" {
			return errUsage
		}
		var until *time.Time
		if len(args) == 4 {
			days, err := strconv.Atoi(args[3])
			if err != nil || days < 1 || days > 400 || plan == "none" {
				return errors.New("days: a number from 1 to 400, and not with none")
			}
			until = new(time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour))
		}
		a, err := svc.Partner.SetPlan(ctx, args[1], plan, until)
		if err != nil {
			return err
		}
		fmt.Println(describe(a))
		return nil
	}
	return errUsage
}

func describe(a posta.Account) string {
	s := "plan " + a.Plan.Type
	if a.Plan.IsActive {
		s += ", active"
	} else {
		s += ", NOT active"
	}
	switch end := a.Plan.ValidUntil; {
	case end != nil:
		s += " until " + end.Format(time.DateOnly)
	case a.Plan.TrialEndsAt != nil:
		s += " until " + a.Plan.TrialEndsAt.Format(time.DateOnly)
	case a.Plan.IsActive:
		s += " until changed"
	}
	return fmt.Sprintf("%s, %d channels", s, a.ConnectedAccounts)
}
