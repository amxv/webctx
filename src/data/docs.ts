export const siteConfig = {
  name: "webctx",
  strapline: "Agent-friendly web context from the terminal",
  description:
    "Use webctx to search the web, turn useful URLs into clean agent-ready text, and map sites before deciding what to read.",
  repoUrl: "https://github.com/amxv/webctx",
  accentColor: "#4f66e8",
  accentColorDark: "#c8f43d",
  footerSections: [
    {
      title: "webctx",
      text:
        "Search, read, and map the web from a small terminal tool built for agent workflows."
    },
    {
      title: "Start here",
      text:
        "Start with examples, then read the short How it works guides when you want the technical model behind URL fast paths and search ranking."
    },
    {
      title: "Repository",
      linkPrefix: "Source: ",
      linkHref: "https://github.com/amxv/webctx",
      linkLabel: "github.com/amxv/webctx"
    }
  ]
} as const;

export const docCategories = ["Start", "Guides", "How it works", "Reference"] as const;

export const primaryNav = [
  { href: "/docs", label: "Docs" },
  { href: siteConfig.repoUrl, label: "GitHub", external: true }
];
