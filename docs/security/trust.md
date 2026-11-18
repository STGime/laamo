# Trust and prompt injection

**Rule:** content a helper reads can change what it writes, never who receives it.

## Trust levels

Every card field records where its value came from (`cards.trust`, per field):

| Level          | Meaning                                                                | May be a recipient          |
| -------------- | ---------------------------------------------------------------------- | --------------------------- |
| `user_entered` | Typed by the user, from About me or My lists, or edited in Check first | yes                         |
| `approved`     | `extracted`, then approved by the user in Check first                  | yes                         |
| `extracted`    | Pulled out by a Think brick, or a structured field of a listing        | no                          |
| `untrusted`    | Raw web page or inbound email text                                     | no, not even after approval |

A field without a recorded level counts as `untrusted`.

## Where it's enforced

1. **Think bricks** get card content only as JSON data inside `<cards>…</cards>`, with a system prompt that says the content may contain instructions and must never be followed. JSON encoding escapes `<` and `>`, so page text can't close the block (`think/guard_test.go`). What a model writes is marked `extracted`.
2. **The validator** (`catalog.Validate`) refuses a stack that sends to other people without Check first above the Act brick (`check_required`), or with a Check first that goes ahead when the user doesn't answer (`check_goes_ahead`).
3. **The engine** holds back cards at run time. An Act brick declares the card fields it sends to (`targets_if` in `bricks.json`; for example `email` for Send an email to people from the cards). Before the brick runs, the engine removes every card whose target field isn't `user_entered` or `approved`. The brick never sees that card, the pile after the step is unchanged, and the step log gets a `held_untrusted_target` entry. This doesn't depend on brick code being right.
4. **Check first** applies the user's decision with `Card.Approve(edits)`: `extracted` fields become `approved`, and edited fields become `user_entered`.

Post to chat sends to a place the user set up, not to a card field, so it has no targets.

## What the user reads

A fooled model can't pick a recipient, but it can still try to fool the user: a summary in Laamo's voice that says "log in at …", or a link whose text hides where it goes. So:

5. **Post to chat** posts card text (titles, summaries, email text, drafts) as plain text (`do/chat.go`, `plain`). A URL inside it becomes `(link: host)`, and `[text](…)`, `<url|text>` and `<!channel>` can't form. Links come only from a card's link fields (`url`, `website`, its source), only `http(s)`, with the host always shown next to the title. Hosts appear in ASCII form, so a look-alike domain reads as `xn--…`.
6. **Email cards say who sent them.** The inbound handler always checks whether the From domain signed the mail (DKIM) and passes `from_signed` (`yes`/`no`), which the email card holds as `user_entered` (Laamo's own check, not the sender's words). Chat messages and the Feed show "✉️ From … · sender checked / not checked"; a `from_signed` with any other trust counts as not checked.
7. **Sum it up** reports what a card asks ("the email from … asks you to …") instead of asking it itself, and writes no links (`think/summarize.go`, `relay`).
8. **Only from me by default.** A helper's email address is public. A new Email this helper brick gets the account's own address in "Only from these senders" (the builder when the brick is added, the API when a stack without a sender list is saved). Only a signed mail from a listed sender starts a run. An empty list (`senders: []`) means anyone, and only the user sets it, on purpose. gmail.com and googlemail.com count as one mailbox. An ignored email becomes a Feed note for the user (`api/ignored.go`), never a reply to the sender: at most one per sender and day, and five per helper and day. "Allow this sender" is offered only when the domain signed the mail, since an unsigned one wouldn't get in anyway.

## Tests

`engine/trust_test.go` runs a lead-outreach stack through Find businesses, Read a webpage, Pull out details, Write, Check first and Send an email. On one website an injected instruction fools the extractor into producing `evil@attacker.example`, and a buggy extractor copies an address from raw page text.

- Without approval, nothing is sent.
- With approval, the approved address is used, the address the user typed over the injected one is used, and the raw one never is.
- `do/chat_test.go` (`TestEmailCantHideLinks`, `TestLinksShowTheirHost`): masked, Slack and raw links in an email never become links in Discord or Slack; a look-alike host shows as `xn--…`; `javascript:` links are dropped; a forged `from_signed` reads as not checked.
- `api/inbound_test.go`: a saved email helper defaults to the owner's address, an unsigned mail claiming to be the owner is ignored, and a cleared list lets anyone in.
