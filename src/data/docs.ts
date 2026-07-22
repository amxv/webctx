export const siteConfig = {
  name: "webctx",
  strapline: "Agent-friendly web context from the terminal",
  description:
    "Documentation for webctx, a pure Go CLI that combines Brave, Tavily, and Exa search results, extracts clean markdown from links, and maps site URLs for agent workflows.",
  repoUrl: "https://github.com/amxv/webctx",
  accentColor: "#1d4ed8",
  accentColorDark: "#60a5fa",
  footerSections: [
    {
      title: "webctx",
      text:
        "A Go CLI for search, clean markdown extraction, and site mapping in agent workflows."
    },
    {
      title: "What this site covers",
      text:
        "Command usage, provider credentials, ranking behavior, architecture notes, and release workflow details."
    },
    {
      title: "Repository",
      linkPrefix: "Source: ",
      linkHref: "https://github.com/amxv/webctx",
      linkLabel: "github.com/amxv/webctx"
    }
  ]
} as const;

export const docCategories = [
  "Start",
  "Commands",
  "Credentials",
  "Internals",
  "Reference"
] as const;

export const primaryNav = [
  { href: "/docs", label: "Docs" },
  { href: siteConfig.repoUrl, label: "GitHub", external: true }
];
