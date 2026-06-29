---
title: Search command
description: Use Brave, Tavily, and Exa together, filter noisy domains, run Exa keyword mode, and understand result formatting.
order: 4
category: Commands
summary: The behavior of `webctx search`.
---

## Basic usage

```bash
webctx search "golang http client retries"
```

The normal search path queries Brave, Tavily, and Exa concurrently. Each provider has a 40-second context timeout. Results are deduplicated, scored, ranked, and returned as markdown links with short summaries.

## Output shape

Search output starts with a count, then a list of markdown links:

```markdown
Total Results: 12

- [Result title](https://go.dev/doc/effective_go)
    - Result overview text
```

The final output is capped at 35 ranked results.

## Default excluded domains

webctx excludes common video and social domains by default:

```text
youtube.com
vimeo.com
dailymotion.com
twitch.tv
tiktok.com
instagram.com
facebook.com
```

This keeps agent-facing search results focused on pages that are more likely to contain readable documentation, articles, issues, or reference content.

## Custom exclusions

Add more domains with `--exclude`:

```bash
webctx search "react useEffect cleanup" --exclude medium.com,dev.to
```

Domain matching normalizes `www.` and compares hostnames.

## Keyword mode

Use `--keyword` when you want Exa include-text filtering:

```bash
webctx search "drizzle orm" --keyword "migration guide"
```

In keyword mode, webctx queries Exa only. The keyword phrase is truncated to five words before it is sent as `includeText`.

## Provider request details

Brave request behavior:

```text
endpoint: https://api.search.brave.com/res/v1/web/search
limit: 20
result_filter: web
text_decorations: false
```

Tavily request behavior:

```text
endpoint: https://api.tavily.com/search
max_results: 20
```

Exa request behavior:

```text
endpoint: https://api.exa.ai/search
numResults: 25
type: auto
contents.livecrawl: preferred
```
