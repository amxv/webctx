---
title: CLI reference
description: The small command and flag reference for everyday webctx use.
order: 20
category: Reference
summary: Commands, flags, keys, and output in one page.
---

## Commands

```bash
webctx --help
webctx --version
webctx search <query> [--exclude domains] [--keyword phrase]
webctx read-link <url>
webctx map-site <url>
```

## Search flags

### `--exclude`

Add comma-separated domains to the normal exclusion list:

```bash
webctx search "react hooks" --exclude youtube.com,medium.com
```

### `--keyword`

Use Exa include-text search for a short phrase:

```bash
webctx search "drizzle orm" --keyword "migration guide"
```

## Environment variables

```text
BRAVE_API_KEY
TAVILY_API_KEY
EXA_API_KEY
FIRECRAWL_API_KEY
GH_TOKEN
GITHUB_TOKEN
```

`GH_TOKEN` is preferred when both GitHub variables are set.

## Output

- `search` → markdown links with short summaries
- `read-link` → focused markdown/text for the selected URL
- `map-site` → discovered URLs with titles/descriptions when available

Raw source, diffs, patches, and explicit job logs stay close to the original provider text. Structured GitHub pages use compact metadata plus the content that matters.

## More detail

- [Search the web](/docs/search)
- [Read a URL](/docs/read-link)
- [Map a site](/docs/map-site)
- [Credentials](/docs/credentials)

If you are developing or releasing webctx itself, see `CONTRIBUTORS.md` in the repository.
