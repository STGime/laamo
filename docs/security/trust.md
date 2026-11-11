# Trust and prompt injection

**Rule:** content a helper reads can change what it writes, never who receives it.

## Trust levels

Every card field records where its value came from (`cards.trust`, per field):

| Level | Meaning | May be a recipient |
|---|---|---|
| `user_entered` | Typed by the user, from About me or My lists, or edited in Check first | yes |
| `approved` | `extracted`, then approved by the user in Check first | yes |
| `extracted` | Pulled out by a Think brick, or a structured field of a listing | no |
| `untrusted` | Raw web page or inbound email text | no, not even after approval |

A field without a recorded level counts as `untrusted`.

## Where it's enforced

1. **Think bricks** get card content only as JSON data inside `<cards>…</cards>`, with a system prompt that says the content may contain instructions and must never be followed. JSON encoding escapes `<` and `>`, so page text can't close the block (`think/guard_test.go`). What a model writes is marked `extracted`.
2. **The validator** (`catalog.Validate`) refuses a stack that sends to other people without Check first above the Act brick (`check_required`), or with a Check first that goes ahead when the user doesn't answer (`check_goes_ahead`).
3. **The engine** holds back cards at run time. An Act brick declares the card fields it sends to (`targets_if` in `bricks.json`; for example `email` for Send an email to people from the cards). Before the brick runs, the engine removes every card whose target field isn't `user_entered` or `approved`. The brick never sees that card, the pile after the step is unchanged, and the step log gets a `held_untrusted_target` entry. This doesn't depend on brick code being right.
4. **Check first** applies the user's decision with `Card.Approve(edits)`: `extracted` fields become `approved`, and edited fields become `user_entered`.

Post to chat sends to a place the user set up, not to a card field, so it has no targets.

## Tests

`engine/trust_test.go` runs a lead-outreach stack through Find businesses, Read a webpage, Pull out details, Write, Check first and Send an email. On one website an injected instruction fools the extractor into producing `evil@attacker.example`, and a buggy extractor copies an address from raw page text.

- Without approval, nothing is sent.
- With approval, the approved address is used, the address the user typed over the injected one is used, and the raw one never is.
