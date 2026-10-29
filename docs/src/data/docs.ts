export const siteConfig = {
  name: "webctx",
  strapline: "Web context and structured data for agents",
  description:
    "Search the web, read useful URLs, map sites, and query structured data from one agent-friendly CLI.",
  repoUrl: "https://github.com/amxv/webctx",
  accentColor: "#4f66e8",
  accentColorDark: "#c8f43d",
  footerSections: [
    {
      title: "webctx",
      text:
        "Find sources, read useful URLs, map sites, and query structured data from the terminal."
    },
    {
      title: "Start here",
      text:
        "Start with task-based guides. Find flags, rate limits, GitHub selectors, and internals under Reference."
    },
    {
      title: "Repository",
      linkPrefix: "Source: ",
      linkHref: "https://github.com/amxv/webctx",
      linkLabel: "github.com/amxv/webctx"
    }
  ]
} as const;

export const docCategories = ["Start", "Guides", "Reference"] as const;

export const primaryNav = [
  { href: "/docs", label: "Docs" },
  { href: "/docs/origo", label: "Origo MCP" },
  { href: siteConfig.repoUrl, label: "GitHub", external: true }
];
