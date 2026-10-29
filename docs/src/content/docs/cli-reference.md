---
title: CLI reference
description: Every webctx command, common flags, and output format.
order: 20
category: Reference
summary: Copyable syntax for web and structured-data commands.
---

## Commands

```bash
webctx --help
webctx --version

# Web research
webctx search <query> [--exclude domain1,domain2] [--keyword "phrase"]
webctx read-link <url> [--question "what to find"]
webctx map-site <url> [--query topic] [--limit N] [--offset N] [--path-prefix /api] [--language en|all]

# Structured data
webctx research [question] [--mode ranked|catalogue] [--view sources|groups|operations] [--sources IDs] [--include inputs,output,examples] [--limit N] [--offset N]
webctx inspect <operation-or-source-id> [--raw]
webctx execute <operation-id> --inputs '{"field":"value"}' [--request-id ID]
webctx execute --calls '[{"id":"source/operation","inputs":{}}]'

# Configuration
webctx status
webctx providers [status|enable <brave|tavily|exa>|disable <brave|tavily|exa>]
webctx keys list
webctx keys add <brave|tavily|exa|firecrawl> --stdin
webctx keys remove <brave|tavily|exa|firecrawl> <managed-key-index>
```

Use quotes around URLs containing `#`, `?`, or shell-special characters.

## Search

| Flag | Behavior |
| --- | --- |
| `--exclude domains` | Removes specified domains, separated by commas, in addition to built-in exclusions. |
| `--keyword phrase` | Prefers Exa for a quoted phrase, with Brave/Tavily fallback. It is not a verified full-text filter. |

Normal search uses available enabled providers in parallel, removes duplicate URLs, and returns up to **35** ranked links with provider snippets. It does **not** automatically open result pages.

Enable or disable providers with `webctx providers enable exa` and `webctx providers disable exa`. This preference is separate from whether you have configured keys.

## `read-link`

| Flag | Behavior |
| --- | --- |
| `--question "..."` | Selects relevant sections and can explore a bounded set of related documentation links. |

Without `--question`, the response follows the specified URL. Native GitHub URLs keep source ranges, comments, review threads, jobs, and other selectors when possible. See [GitHub URL reference](/docs/github-urls).

## `map-site`

| Flag | Default | Behavior |
| --- | --- | --- |
| `--query` | None | Prioritizes pages related to the topic. |
| `--limit` | 30 | Number of returned pages (1–100). |
| `--offset` | 0 | Zero-based result offset. |
| `--path-prefix` | None | Includes only paths under the given prefix. |
| `--language` | `en` | Locale filter; use `all` to include translated pages. |

Mapping can discover at most 5,000 URLs from the configured provider request. Results include total counts and a copyable continuation when more pages are available. Explicit `--limit 0` is invalid; omit the flag for the default.

## Structured-data commands

`research` can search by natural-language question, or use `--mode catalogue` to browse a paginated list. Views are `sources`, `groups`, and `operations`. Additional filters include `--urls`, `--sources`, `--categories`, `--groups`, and `--operations`. Use `--include inputs,output,examples` to expand specific contract fields.

`inspect` accepts an operation or source ID. Use `--raw` for the full upstream contract.

`execute` accepts an operation ID and a JSON object of inputs, or `--calls` for a batch of up to ten operations. Prefix a JSON filename with `@` when arguments are too long:

```bash
webctx execute --calls @requests.json
webctx execute particle/podcasts/episodes/search --inputs @inputs.json
```

`--request-id` supplies an idempotency ID for retries of the **same** paid execution. Do not reuse it for a new query or page. For prices, credit limits, return fields, and retries, see [Structured data reference](/docs/data-reference).

## Credentials and settings

`webctx status` displays search provider enablement and locally configured key counts, not live credit balances. `keys list` also reports managed macOS Keychain slots. `keys add` reads one key from standard input and `keys remove` removes a numbered *managed* key; they never print key values.

Search provider settings are stored in your OS user config directory under `webctx/config.json`, or at `WEBCTX_CONFIG_FILE` if set.

See [Credentials](/docs/credentials) for environment variables and [Rate limits and concurrency](/docs/rate-limits) for scheduling overrides and state locations.

## Output formats

| Command | Output |
| --- | --- |
| `search` | Markdown with titles, URLs, and short provider snippets. |
| `read-link` | Markdown/text with source attribution and caveats. |
| `map-site` | Site URL inventory with counts and pagination. |
| `research` | JSON with sources/operations and next-step arguments. |
| `inspect` | JSON with contract, price, and suggested execution. |
| `execute` | Provider data and per-operation results, credits, and continuations. |

Raw source files, diffs, and patches remain close to their upstream representation. Errors include provider-specific limitations when known.
