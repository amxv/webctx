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
        "Paste the URL you already have. webctx keeps useful URL intent—source lines, Issues, PR threads, Actions jobs, and more—while removing browser noise."
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
  { href: siteConfig.repoUrl, label: "GitHub", external: true }
];
