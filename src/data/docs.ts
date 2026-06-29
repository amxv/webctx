export const siteConfig = {
  name: "webctx",
  strapline: "Agent-friendly web context from the terminal",
  description:
    "Documentation for webctx, a pure Go CLI that combines Brave, Tavily, and Exa search results, extracts clean markdown from links, and maps site URLs for agent workflows.",
  repoUrl: "https://github.com/amxv/webctx"
} as const;

export const docCategories = [
  "Start",
  "Commands",
  "Credentials",
  "Internals",
  "Reference"
] as const;

export const primaryNav = [
  { href: "/", label: "Overview" },
  { href: "/docs", label: "Docs" },
  { href: siteConfig.repoUrl, label: "GitHub", external: true }
];
