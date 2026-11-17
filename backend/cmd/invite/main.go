// Command invite lets people from the waiting list into the app: it emails
// them an invite and marks them invited (only invited people can use the
// app while it is invite-only).
//
//	invite 20                    # who'd be next: the 20 who confirmed first
//	invite -send 20              # invite them
//	invite -send anna@example.com [-lang de]   # invite someone by address
//
// Without -send nothing is sent or changed.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/STGime/laamo/backend/internal/config"
	eb "github.com/STGime/laamo/backend/internal/eurobase"
	"github.com/STGime/laamo/backend/internal/mail"
	sebs "github.com/STGime/laamo/backend/internal/store/eurobase"
	"github.com/STGime/laamo/backend/internal/waitlist"
)

func main() {
	send := flag.Bool("send", false, "send the invites (without it: only show who'd get one)")
	lang := flag.String("lang", "en", "language for people invited by address who aren't on the list (en, de)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: invite [-send] <how many> | invite [-send] [-lang de] <email>...")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*send, *lang, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "invite:", err)
		os.Exit(1)
	}
}

func run(send bool, lang string, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.EurobaseSecretKey == "" {
		return errors.New("EUROBASE_PRIVATE_KEY isn't set")
	}
	if send && cfg.Env != "dev" {
		if cfg.ResendAPIKey == "" {
			return errors.New("RESEND_API_KEY isn't set: nobody would get the email")
		}
		if strings.Contains(cfg.SiteURL, "localhost") {
			return fmt.Errorf("SITE_URL is %s: invite links would point there", cfg.SiteURL)
		}
	}
	var sender mail.Sender = mail.LogSender{Log: slog.Default()}
	if cfg.ResendAPIKey != "" {
		sender = &mail.Resend{APIKey: cfg.ResendAPIKey, From: cfg.MailFrom}
	}
	s := &waitlist.Service{Store: sebs.New(eb.New(cfg.EurobaseURL, cfg.EurobaseSecretKey)), Mail: sender, SiteURL: cfg.SiteURL}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Printf("%s · %s · links to %s\n", cfg.Env, cfg.EurobaseURL, cfg.SiteURL)

	// By address.
	if _, err := strconv.Atoi(args[0]); err != nil {
		for _, email := range args {
			if !send {
				ok, err := s.Invited(ctx, email)
				if err != nil {
					return err
				}
				fmt.Printf("  %s%s\n", email, map[bool]string{true: " (already invited)"}[ok])
				continue
			}
			if _, err := s.InviteEmail(ctx, email, lang); err != nil {
				return err
			}
			fmt.Println("  invited", email)
		}
		if !send {
			fmt.Println("Nothing sent. Add -send to invite.")
		}
		return nil
	}

	// The next n.
	n, _ := strconv.Atoi(args[0])
	if n < 1 || n > 500 {
		return errors.New("how many: 1 to 500")
	}
	next, err := s.Next(ctx, n)
	if err != nil {
		return err
	}
	if len(next) == 0 {
		fmt.Println("Nobody is waiting: everyone who confirmed is invited.")
		return nil
	}
	for _, e := range next {
		line := fmt.Sprintf("%-40s %s  confirmed %s  %s", e.Email, e.Locale, e.ConfirmedAt.Format("2006-01-02"), strings.Join(e.Interests, ","))
		if e.Wish != nil {
			line += fmt.Sprintf("  %q", *e.Wish)
		}
		if send {
			if _, err := s.Invite(ctx, e); err != nil {
				return err
			}
			line = "invited " + line
		}
		fmt.Println("  " + line)
	}
	if !send {
		fmt.Printf("%d would be invited. Nothing sent. Add -send to invite.\n", len(next))
	} else {
		fmt.Printf("%d invited.\n", len(next))
	}
	return nil
}
