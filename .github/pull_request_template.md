## What & why

<!-- One or two sentences. Link the issue: Closes #… -->

## How I tested

<!-- task test / task lint, manual steps, screenshots for UI -->

## Look & feel checklist (UI changes only)

- [ ] No scroll animations, gradients, glows, blurs or soft shadows added?
- [ ] Only system font + the one display face, both self-hosted?
- [ ] Color used only from the brick/neutral tokens, and each color means something?
- [ ] One primary action, bottom-anchored, 48px+ targets?
- [ ] Looks identical in structure at 375px and 1440px?
- [ ] Copy free of AI buzzwords and sparkle icons?
- [ ] Motion only on state change and off with reduced motion?

## Safety (backend changes only)

- [ ] Schema changes are migrations in `schema/`, not manual edits
- [ ] No secrets in code, logs or the web app
- [ ] Untrusted content (web pages, inbound email) can't choose recipients
