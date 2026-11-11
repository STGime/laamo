# Reading web pages

The Read a webpage brick (`look.read_page`) fetches pages that users and other bricks name. The URLs come from users, search results and business listings, so the fetcher (`backend/internal/web`) treats them as hostile.

- **Public addresses only.** Every connection is checked after DNS resolution, including each redirect. Loopback, private networks, link-local (cloud metadata at 169.254.169.254), CGNAT, NAT64 and unspecified addresses are refused. Environment proxies aren't used, because they would bypass the check.
- **http and https only**, with no credentials in the URL and at most 5 redirects.
- **robots.txt** is respected for `LaamoBot` (or `*`) and cached for an hour per host. A missing robots.txt allows everything; a 5xx disallows everything for now, as search engines do.
- **Limits:** 15 s per page, 2 MiB read (the rest is cut off), and 20,000 characters of text per page (30,000 with contact pages). Up to 4 pages are read at once per run.
- **User-Agent:** `LaamoBot/0.1 (+https://laamo.app/bot)`. The bot page still needs writing before launch.
- **Untrusted output:** `page_title` and `page_text` are marked `untrusted`. Think bricks get them only as delimited data (#22).
- **Failures don't stop a run.** A page that can't be read (robots, 403 paywall, 404, not HTML) gets a short note in `page_text` and the run carries on. It only retries when every page failed for a reason that may pass (timeouts, 5xx).

Large text (over 32 KB) goes to project storage under `laamo/runs/<run>/<step>/<card>/<field>.txt`, and the card keeps a reference. Cleaning these up with run retention is still to do.
