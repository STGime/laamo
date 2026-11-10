# Brick catalog

`bricks.json` is the single source of truth for bricks. The Go engine validates stacks against it, and the web builder renders brick sheets and live hints from it.

- Go: `backend/internal/catalog`. The JSON is embedded via the tiny Go module in this folder (`embed.go`, wired in with a `replace` in `backend/go.mod`).
- Web: `import { catalog, brick } from '@laamo/catalog'` (types in `index.ts`) and `import { validate } from '@laamo/catalog/validate'`.
- Checks: `pnpm --filter @laamo/catalog test` validates the files against `bricks.schema.json` and `template.schema.json`. `go test ./internal/catalog` checks the meaning: references, defaults, at most 3 visible settings, fixes, and that templates validate as stacks.

Adding a brick takes three steps: add a catalog entry, add a Go executor in `backend/internal/bricks`, and add an `en`/`de` text for every label.

## Bricks

| Key | Meaning |
|---|---|
| `id` | `<group>.<name>`, e.g. `look.search`. Stored in stacks, so never rename one. |
| `group` | `when` 🟡, `look` 🔵, `think` 🟣, `do` 🟢 or `ask` 🔴. A When brick is always first, and there is only one. |
| `shape` | How the pile changes (see below). `shape_if` overrides it per config, e.g. Write it with "one in total" is `combine`. |
| `status` | `mvp`, or `later` (shown as coming soon, can't be used yet). |
| `sentence` | The brick as one sentence. The first entry whose `if` matches wins. `[key]` is replaced by that setting's display value. |
| `settings` | The brick sheet. At most 3 are visible at once; the rest are `advanced` (behind "More"). `show_if` hides a setting unless its condition matches. |
| `needs` / `needs_if` | Card fields (or `context` keys) that must exist above this brick. |
| `adds` / `adds_if` / `adds_from` | Fields this brick adds to the cards. `adds_from` names a setting whose value is the list of added fields (Pull out details). `open: true` means the fields are unknown, for example a List's columns. |
| `provides` / `provides_if` | Run values for later bricks: `answer`, `trigger.from`, `trigger.subject`. |
| `requires` / `requires_if` | Needs a brain, one of these place types, or a paid plan. |
| `check` | Check first: `default` (the builder adds it, the user may remove it) and/or `required_if` (it can't be removed). |

**Shapes**

| Shape | What it does to the pile | Fields after the brick |
|---|---|---|
| `start` | Starts with nothing → 0 or 1 card | only the brick's `adds` |
| `expand` | 1 card → many new cards | only the brick's `adds` |
| `enrich` | Each card keeps its id and gains more fields | previous fields + `adds` |
| `filter` | Many cards → fewer | previous fields + `adds` |
| `combine` | Many cards → 1 new card | only the brick's `adds` |
| `act` | Same pile, marked done | previous fields + `adds` |

**Conditions** (`if`, `show_if`, `required_if`): every key must match, after defaults are filled in. A value is either a list of allowed values (`{"to": ["cards", "fixed"]}`) or `"set"`, which means not empty.

**Pills** in settings marked `pills: true`:

- `{email}`, `{name}`, …: a card field. It must be added by a brick above.
- `{my city}`, `{my offer}`, …: an About me fact from `profile`. If it isn't known yet, the user is asked once (S15).
- `{answer}`: needs Ask me something, or a question on Tap Go.
- `{trigger.subject}`, `{trigger.from}`: only when the helper starts with Email this helper.

**Fixes**: `fixes` maps a missing field to the brick that adds it. The validator returns it as a one-tap fix: "insert this brick before brick b5". `fixes.check` is the Check first fix.

## Stack validation (`catalog.Validate`)

The validator walks the stack top to bottom and tracks which card fields and run values exist at each point. Each problem it finds is returned with a `level`:

- `error`: the stack can't be saved or run. Causes:
  - an unknown brick, or one that isn't available yet
  - the When brick isn't on top
  - a setting that is unknown, missing or has a bad value
  - a bad or unknown pill
  - a missing field or run value
  - an email to other people without a Check first above it, or under a Check first set to go ahead when you don't answer
  - bad stack limits
- `hint`: a gentle suggestion with a fix, e.g. "Check with you first?".

The same rules run in TypeScript for the builder's live hints: `import { validate } from '@laamo/catalog/validate'`. Both versions run the cases in `fixtures/validate.json` (Go in `internal/catalog`, TypeScript in `validate.test.ts`), so they can't drift apart. Add a case there whenever a rule changes.

## Templates (`templates/*.json`)

Each template is a Shelf helper: name, promise, `category` (one of the S3 need chips), avatar, a stack, at most 3 `setup_questions`, and `required`.

A setup question either:

- asks for an About me fact (`{"profile": "business"}`). It's skipped when the profile already has the fact, and the stack uses `{my business}`.
- fills brick settings for this helper (`fills: [{"brick": "b5", "setting": "place"}]`). Settings filled this way don't count as missing when the template is validated.

`required` must match what the stack needs; the Go tests check that.

Note for #38: as written in the product plan, Weekend Scout (Search → Keep → Pick → Put in my calendar) doesn't validate. Search results have no date or time, so it needs a Pull out details (date, time, place) step before the calendar.
