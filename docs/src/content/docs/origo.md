---
title: Origo MCP server
description: Use webctx's retrieval tools from an MCP client.
order: 15
category: Guides
summary: Connect an MCP client to the hosted webctx retrieval engine.
---

**Origo** is the hosted MCP interface built on the same Go retrieval engine as the webctx CLI. You can use it from a compatible MCP client without installing the CLI locally.

Origo offers five tools:

| Tool | Purpose |
| --- | --- |
| `read_link` | Read a URL as clean text, optionally answering a question about it. |
| `map_site` | Discover and filter pages on a site. |
| `research` | Find structured data sources and operations. |
| `inspect` | View the schema and price of an operation. |
| `execute` | Run a chosen operation. May spend credits. |

The CLI's federated `search` command is **not** available through Origo.

## Connect

Configure a Streamable HTTP MCP connection in your client using the access URL provided for your Origo deployment:

```text
https://api.origo.ashray.xyz/mcp?key=<your-private-key>
```

You need a valid Origo access key. Treat the **entire URL as a secret**, since its query string contains the key. Do not post it publicly or paste it into ordinary browser links.

## Example workflow

Ask your MCP client to read a documentation URL with `read_link`, or to find site pages with `map_site`. For a data task, start with `research`, inspect a returned operation's required inputs, and then explicitly execute it.

Discovery and inspection do not spend Alexandria execution credits. `execute` does. Origo checks the inputs and estimated charge before submitting paid operations and returns provider terms requirements instead of accepting them automatically.

## Next

- [Read a URL](/docs/read-link) for typical source reads.
- [Map a site](/docs/map-site) for URL discovery.
- [Research structured data](/docs/data-sources) for the data workflow.
- [Origo technical reference](/docs/origo-reference) for authentication, request behavior, deployment, and billing details.
