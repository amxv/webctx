---
title: Search the web
description: Search multiple providers and automatically retrieve relevant source documentation with the results.
order: 10
category: Guides
summary: Find candidate pages and get actual source context in the same call.
---

## Search normally

```bash
webctx search "golang http client retries"
```

webctx asks the enabled Brave, Tavily, and Exa providers, removes duplicate URLs, and then automatically reads up to three strong results in parallel. It also follows up to two highly relevant same-host documentation links and selects concise, source-grounded excerpts. The original ranked URL list remains available for deeper navigation. A single configured and enabled search provider is sufficient.

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

`--keyword` uses Exa's include-text search when available, if you care more about a phrase appearing on the page than broad provider agreement. If Exa is disabled, unconfigured, or fails, it falls back to Brave/Tavily with a quoted-phrase query, **not** a guaranteed page-text filter. The output labels this difference.

## Manage search providers

```bash
webctx status
webctx providers disable exa
webctx providers enable exa
webctx providers disable tavily
```

Settings persist across CLI invocations and override key presence: a disabled provider is never contacted even if its API key is configured. `status` shows enabled/disabled state and whether keys are present, without exposing secrets or querying API credit balances. The three toggles govern **search** only; Firecrawl remains a separate page-reading and structured-data integration. Provider settings are stored in `webctx/config.json` under the OS user config directory; optionally set `WEBCTX_CONFIG_FILE` for a custom location.

## Add fallback API keys

Each search provider supports multiple keys, using its plural environment variable (`BRAVE_API_KEYS`, `TAVILY_API_KEYS`, or `EXA_API_KEYS`) as a comma/newline-separated list or JSON array. The plural keys are tried in order, followed by the existing singular `*_API_KEY` as a last fallback. Identical keys are deduplicated.

On macOS, WebCTX can securely manage the plural key list in your Keychain:

```bash
webctx keys list
pbpaste | webctx keys add exa --stdin
webctx keys remove exa 1
webctx providers enable exa
```

The `keys add` command reads the secret from standard input; it never requires it in shell arguments. Newly added Keychain keys take priority. `keys list` reveals only counts, including the number of Keychain-managed keys, and `keys remove` indexes only those managed keys (1 = newest).

When the first key fails with an authentication, exhausted-credit/quota, rate-limit, or server error, WebCTX tries the next configured key automatically. Transport failures can also advance to the next key. Invalid request arguments and malformed successful JSON are **not** retried with other credentials. This applies to Brave, Tavily, and Exa searches, not the separately billed Firecrawl/Alexandria API; respect each provider's plan and account limits.

## Dig deeper with `read-link`

A good research loop is:

```bash
webctx search "OpenAI Apps SDK MCP annotations"
webctx read-link https://developers.openai.com/apps-sdk/reference
```

Search already returns relevant source material, so agents often need only one call. Use `read-link` if you need the entire original page or a narrower question-focused expansion of one URL.

When Alexandria is available, Webctx discovers matching provider capabilities for free and lists their identity. Discovery does not execute paid provider calls or automatically accept third-party terms.

If you want the exact scoring model behind the merged list, see [How search ranking works](/docs/ranking).

## Output

```markdown
Total Results: 12

## Retrieved source context

### https://docs.example.com/reference
<relevant Markdown, exact API examples, and source-backed excerpts>

## Search results

- [Result title](https://example.com/page)
    - Short summary of the page
```

Search uses bounded source counts and excerpt budgets. It returns supporting material rather than generating an answer or claiming to have semantically verified every source. Failed secondary fetches do not prevent the original result list from being returned.
