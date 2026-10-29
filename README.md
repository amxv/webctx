# webctx

**Web research for coding agents, from the terminal.**

webctx finds useful pages, reads the part of a URL you actually need, and discovers structured data sources. It returns Markdown or JSON that an agent, script, or human can use directly, without browser UI noise.

## What can it do?

| Command | What it does |
| --- | --- |
| `search` | Searches Brave, Tavily, and Exa in parallel, removes duplicates, and ranks useful links. |
| `read-link` | Turns a web page or GitHub URL into clean text, including exact source lines, issues, PR threads, and CI jobs. |
| `map-site` | Finds pages on a site so you can choose what to read next. |
| `research` | Finds structured datasets and available operations through Firecrawl Alexandria. |
| `inspect` | Shows the inputs, outputs, and price of a data operation. |
| `execute` | Runs a selected data operation and returns its results. This can use paid credits. |

**Search finds candidates; `read-link` retrieves the source.** Search results include provider-supplied snippets, not automatically scraped page contents. This keeps searches fast and avoids unnecessary crawl charges.

## Install

```bash
npm install -g webctx
webctx --help
```

Prebuilt binaries for macOS, Linux, and Windows are available in [GitHub Releases](https://github.com/amxv/webctx/releases). You can also [build from source](https://webctx.ashray.xyz/docs/install).

## Try it

```bash
# Find relevant documentation
webctx search "Go context cancellation"

# Read the exact GitHub source lines in a copied link
webctx read-link 'https://github.com/amxv/webctx/blob/main/README.md#L1-L20'

# Ask a question about a documentation page
webctx read-link https://docs.example.com/api --question "How does pagination work?"

# Find pages on a documentation site
webctx map-site https://docs.firecrawl.dev

# Discover structured data, inspect its contract, then query it
webctx research "podcast conversations about AI agents"
webctx inspect particle/podcasts/episodes/search
webctx execute particle/podcasts/episodes/search --inputs '{"semantic_search":"AI agents","limit":2}'
```

GitHub URLs are especially useful: `read-link` understands repository trees, files, line ranges, issues, pull requests, review comments, commits, and Actions jobs. You can give it the link you already copied instead of learning a separate command for each GitHub feature.

## Add the keys you need

- **Search:** At least one of `BRAVE_API_KEY`, `TAVILY_API_KEY`, or `EXA_API_KEY`.
- **Page extraction, site maps, and structured data:** `FIRECRAWL_API_KEY`.
- **GitHub:** `GH_TOKEN` is optional for many public URLs, but helps with higher limits and authenticated resources.

Set keys as environment variables, in `.env.local`, or in macOS Keychain. Multiple keys from separate accounts are supported, including `FIRECRAWL_API_KEYS`. WebCTX spreads work across available accounts and coordinates rate limits between agents running on the same machine.

See [Credentials](https://webctx.ashray.xyz/docs/credentials) for setup and [Rate limits and concurrency](https://webctx.ashray.xyz/docs/rate-limits) for the technical details.

## Use it through MCP

**Origo** is the hosted MCP interface built on the same retrieval engine. It offers `read_link`, `map_site`, `research`, `inspect`, and `execute` to clients with an Origo access key. The federated `search` command is CLI-only.

See [Origo MCP setup](https://webctx.ashray.xyz/docs/origo).

## Documentation

Start with the [Quickstart](https://webctx.ashray.xyz/docs/quickstart) or browse the [documentation](https://webctx.ashray.xyz/docs).

The **Guides** cover search, reading URLs, mapping sites, structured data, and agent workflows. The **Reference** section contains command flags, supported GitHub URL formats, credentials, rate limits, ranking, architecture, and advanced data/MCP behavior.

Contributing, building, and releasing: [CONTRIBUTORS.md](CONTRIBUTORS.md).

License: [Apache-2.0](LICENSE).
