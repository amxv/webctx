---
title: Credentials
description: Configure provider API keys and optional GitHub authentication.
order: 21
category: Reference
summary: Environment variables, Keychain storage, and multiple independent accounts.
---

## Which key does each command use?

| Capability | Keys |
| --- | --- |
| `search` | One or more of `BRAVE_API_KEY`, `TAVILY_API_KEY`, `EXA_API_KEY` |
| `map-site` and normal webpage extraction | `FIRECRAWL_API_KEY` |
| Alexandria `research`, `inspect`, `execute` | `FIRECRAWL_API_KEY` |
| GitHub-native reads | Optional `GH_TOKEN` or `GITHUB_TOKEN` |

You can use the plural form of each provider variable (`BRAVE_API_KEYS`, `TAVILY_API_KEYS`, `EXA_API_KEYS`, and `FIRECRAWL_API_KEYS`) to configure **multiple keys from independent accounts**. Those keys are rotated based on available capacity.

For Firecrawl Alexandria paid executions, WebCTX deliberately uses the singular `FIRECRAWL_API_KEY` if set, or the first plural key otherwise. Paid operations do not automatically switch accounts on retries, to avoid duplicate charges or invalid idempotency replays.

## Environment or `.env.local`

For a simple local setup, add only the keys you need:

```dotenv
BRAVE_API_KEY=your-brave-key
EXA_API_KEY=your-exa-key
TAVILY_API_KEY=your-tavily-key
FIRECRAWL_API_KEY=your-firecrawl-key
GH_TOKEN=your-github-token
```

webctx reads existing environment variables first, then `.env.local` files near the installed binary or current working directory, then macOS Keychain values. Existing variables are not overwritten. Keep `.env.local` out of version control.

Plural variables accept comma-separated keys, lines, or JSON arrays:

```dotenv
TAVILY_API_KEYS=account-one-key,account-two-key
FIRECRAWL_API_KEYS=another-team-key,third-team-key
```

The corresponding singular key joins that provider's pool, with exact duplicates removed. Your provider's credit allowance still applies to each account independently.

## macOS Keychain

You can add or remove supported provider keys without putting them in shell arguments or plaintext files:

```bash
webctx keys list
pbpaste | webctx keys add exa --stdin
pbpaste | webctx keys add firecrawl --stdin
webctx keys remove exa 1
```

`keys add` reads one key from standard input. The `keys remove` number selects a *managed Keychain key*, newest first; it does not remove a singular environment variable. `keys list` and `webctx status` show counts only, never secrets.

On Linux and Windows, use environment variables or `.env.local` for additional credentials. A macOS Keychain entry named after an environment variable, under service `webctx`, can also supply a singular key or `GH_TOKEN`.

## Search providers

```bash
webctx status
webctx providers disable exa
webctx providers enable exa
```

Disabled providers are not queried, even when keys are configured. Provider settings are saved to `webctx/config.json` in the OS user config directory, or the path set by `WEBCTX_CONFIG_FILE`. `status` checks local configuration, not actual provider availability or credit balances.

## GitHub authentication

Most public repository, file, issue, PR, release, and Gist reads work without a token. Set `GH_TOKEN` or `GITHUB_TOKEN` for higher GitHub API capacity or for features such as blame, Discussions, some Actions logs, richer PR review state, and private resources your token can access. `GH_TOKEN` takes precedence when both are set.

Fine-grained tokens may lack permissions for some endpoints. For selected public GitHub reads, webctx may retry anonymously when a narrow token is rejected. It does not bypass GitHub's permission model, and native GitHub rate limits/private-resource errors remain visible.

## Related reference

- [Rate limits and concurrency](/docs/rate-limits) explains per-key admission, fallbacks, and quotas.
- [GitHub URL reference](/docs/github-urls) lists native routes and authentication caveats.
- [Troubleshooting](/docs/troubleshooting) covers missing keys and provider failures.
