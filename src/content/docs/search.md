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

webctx asks Brave, Tavily, and Exa, removes duplicate URLs, and then automatically reads up to three strong results in parallel. It also follows up to two highly relevant same-host documentation links and selects concise, source-grounded excerpts. The original ranked URL list remains available for deeper navigation.

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

`--keyword` uses Exa's include-text search when you care more about a phrase appearing on the page than broad provider agreement.

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
