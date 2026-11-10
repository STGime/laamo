# Brick catalog

`bricks.json` is the single source of truth for bricks. The Go engine validates stacks against it and the web builder renders brick sheets from it. Adding a brick = catalog entry + Go executor.

`templates/*.json` holds the Shelf templates (stack, setup questions, required places).

The v1 schema and all 20 MVP bricks come in issue #10.
