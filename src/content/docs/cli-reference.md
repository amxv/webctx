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
webctx status
webctx providers [status|enable <brave|tavily|exa>|disable <brave|tavily|exa>]
webctx keys list
webctx keys add <brave|tavily|exa> --stdin
webctx keys remove <brave|tavily|exa> <managed-key-index>
webctx read-link <url> [--question "what to find"]
webctx map-site <url> [--query topic] [--limit 30] [--offset 0] [--path-prefix /api] [--language en|all]
webctx research [question] [--mode ranked|catalogue] [--view sources|groups|operations] [--sources IDs] [--include inputs,output,examples] [--limit N] [--offset N]
webctx inspect <operation-or-source-id> [--raw]
webctx execute <operation-id> --inputs '{"field":"value"}' [--request-id ID]
webctx execute --calls '[{"id":"source/operation","inputs":{}}]'
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

If Exa is disabled, missing a key, or unavailable, Brave/Tavily search with the quoted phrase instead (not a strict page-text filter).

## Provider settings

```bash
webctx status                # see enabled state and credential presence
webctx providers disable exa # never send Exa search requests
webctx providers enable exa  # re-enable later
```

These are persistent, per-user search-provider settings, independent of credentials. The config file is `webctx/config.json` inside the OS user config directory, or the path given by `WEBCTX_CONFIG_FILE`. `status` does not check provider quotas or API health. Firecrawl is separate from federated search.

## Multiple credentials per provider

For additional API keys, use `BRAVE_API_KEYS`, `TAVILY_API_KEYS`, or `EXA_API_KEYS` (CSV, newlines, or JSON arrays). Plural keys are tried in order before the legacy singular key. A rejected key triggers failover for auth, quota/credits, rate limits, server, or transport failures; malformed successful responses and invalid API requests do not.

On macOS, `pbpaste | webctx keys add exa --stdin` stores a newly copied key in Keychain, not on disk. `webctx keys list` reports configured counts and Keychain-managed slots without printing secrets; `webctx keys remove exa 1` deletes the newest managed Exa key, without touching the singular legacy key. These commands support Brave, Tavily, and Exa only; Firecrawl/Alexandria has separate request and billing semantics.

## Question-focused reading

`--question` asks Webctx to select relevant API/code sections and follow up to two matching same-host documentation links. Without it, `read-link` retains the full original source read.

```bash
webctx read-link https://docs.example.com/api --question "How does pagination work?"
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

- `search` → retrieved source excerpts, supporting documentation, and ranked markdown links
- `read-link` → full source Markdown/text, or question-focused excerpts and related sources
- `map-site` → discovered URLs with titles/descriptions when available
- `research` → JSON with ranked or pageable operations and next inspect calls
- `inspect` → JSON with the exact source contract, price, and next execute call
- `execute` → provider-native JSON, charged credits, request ID, and pagination hints

Raw source, diffs, patches, and explicit job logs stay close to the original provider text. Structured GitHub pages use compact metadata plus the content that matters.

## More detail

- [Search the web](/docs/search)
- [Read a URL](/docs/read-link)
- [Map a site](/docs/map-site)
- [Research structured data](/docs/data-sources)
- [Credentials](/docs/credentials)

If you are developing or releasing webctx itself, see `CONTRIBUTORS.md` in the repository.
