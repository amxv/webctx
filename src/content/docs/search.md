---
title: Search the web
description: Search Brave, Tavily, and Exa with coding-oriented reranking and no automatic page scrapes.
order: 10
category: Guides
summary: Rank technical documentation and first-party sources, then read only the links you need.
---

## Search normally

```bash
webctx search "golang http client retries"
```

webctx asks the enabled Brave, Tavily, and Exa providers, removes duplicate URLs, and reranks up to 35 results. The response contains each result's title, URL, and provider-supplied snippets. It **does not automatically open any webpages**; this keeps searches fast and avoids unnecessary Firecrawl credits. A single configured and enabled provider is sufficient.

For programming queries, the reranker favors matching title keywords, documentation paths, API and SDK references, official documentation sites, and verified first-party GitHub organizations. Independent agreement between providers remains an important ranking signal. Tavily uses its more comprehensive `advanced` search depth; Exa uses query-relevant highlights without forcing a fresh extraction of every page.

When several providers independently surface the same page, that agreement helps the page rise in the final list.

## Remove noisy sites

```bash
webctx search "react useEffect cleanup" --exclude medium.com,dev.to
```

webctx already excludes common video and social sites such as YouTube, TikTok, Instagram, and Facebook. `--exclude` adds your own domains for one search.

## Look for a specific phrase

```bash
webctx search "drizzle orm" --keyword "migration guide"
```

`--keyword` uses Exa `auto` search with the phrase quoted in the query. Exa's deprecated `includeText` filter had approximate matching and has no equivalent modern strict page-text filter. If Exa is disabled, unconfigured, or fails, WebCTX tries a quoted phrase with Brave and enables Tavily's `exact_match` to restrict its results. The output labels the distinction, and **does not promise that Exa/Brave results have been checked against the full page text**.

## Manage search providers

```bash
webctx status
webctx providers disable exa
webctx providers enable exa
webctx providers disable tavily
```

Settings persist across CLI invocations and override key presence: a disabled provider is never contacted even if its API key is configured. `status` shows enabled/disabled state and whether keys are present, without exposing secrets or querying API credit balances. The three toggles govern **search** only; Firecrawl remains a separate page-reading and structured-data integration. Provider settings are stored in `webctx/config.json` under the OS user config directory; optionally set `WEBCTX_CONFIG_FILE` for a custom location.

## Load-balance multiple API keys

Each search provider supports multiple keys, using its plural environment variable (`BRAVE_API_KEYS`, `TAVILY_API_KEYS`, or `EXA_API_KEYS`) as a comma/newline-separated list or JSON array. The legacy singular `*_API_KEY` joins the same round-robin pool, and identical keys are deduplicated. **Subsequent searches use different keys**, including when separate agents invoke the CLI at the same time.

On macOS, WebCTX can securely manage the plural key list in your Keychain:

```bash
webctx keys list
pbpaste | webctx keys add exa --stdin
webctx keys remove exa 1
webctx providers enable exa
```

The `keys add` command reads the secret from standard input; it never requires it in shell arguments. Newly added Keychain keys appear first in management lists, but **no key has priority for requests**. `keys list` reveals only counts, including the number of Keychain-managed keys, and `keys remove` indexes only those managed keys (1 = newest).

When a selected key fails with an authentication, exhausted-credit/quota, rate-limit, or server error, WebCTX tries the next available key automatically. Transport failures can also advance to another key. Unhealthy keys receive a **shared temporary cooldown** so other agents do not keep retrying them; `Retry-After` is honored if provided. Invalid request arguments and malformed successful JSON are **not** retried with other credentials. For multi-key providers, a shared cursor and health record live in the OS user cache under `webctx/search-keys`, or an override supplied through `WEBCTX_SEARCH_KEY_BALANCER_DIR`. No keys are stored there, only one-way hashes and cooldown timestamps; the actual API keys stay in their existing environment/Keychain locations. This applies to Brave, Tavily, and Exa searches, not the separately billed Firecrawl/Alexandria API. If all keys share one provider account, its account-wide limits still apply.

## Dig deeper with `read-link`

A good research loop is:

```bash
webctx search "OpenAI Apps SDK MCP annotations"
webctx read-link https://developers.openai.com/apps-sdk/reference
```

Search gives agents useful provider snippets and direct links. Use `read-link` for full webpage content, code examples, or question-focused exploration after choosing a result worth investigating. This deliberately avoids automatically spending Firecrawl credits on speculative pages.

Structured-data discovery is separate from regular search. Use `webctx research "what you need"` to discover Firecrawl Alexandria provider capabilities; this does not run during `webctx search`. Discovery does not execute paid provider calls or automatically accept third-party terms.

If you want the exact scoring model behind the merged list, see [How search ranking works](/docs/ranking).

## Output

```markdown
Total Results: 12

## Search results

- [Result title](https://example.com/page)
    - Short summary of the page
```

Search uses provider-native snippets and Exa highlights (bounded to 1,100 characters per result) rather than opening webpages; it does not independently verify each snippet against live page content. An explicit `read-link` does that deeper retrieval when the agent chooses to follow a URL.

## Shared Firecrawl queue

`read-link`, `map-site`, and `research` share file-backed Firecrawl admission controls across all WebCTX processes running as the same OS user, rather than resetting their request budgets on each CLI invocation. Firecrawl Free's current scrape, map, and search limits are **10 requests per rolling minute per endpoint**, with **two concurrent browser slots** and **two browser-session creations per minute**. Expensive HTTP operations run outside file locks; requests wait for permits as needed. State defaults to your OS user cache directory and can be moved with `WEBCTX_FIRECRAWL_QUEUE_DIR` when sharing a working directory between agents.

This local queue cannot coordinate users running on separate machines or independent serverless workers. Firecrawl's team-wide quotas remain authoritative; requests from other clients using the same Firecrawl team can still cause `429` responses. Your existing Firecrawl 30-minute cache window and proxy escalation are unchanged.
