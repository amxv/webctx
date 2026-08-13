import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

// These are the files that can change the generated Astro documentation site.
// The shared package manifest and lockfile are included because they provide
// the docs build scripts and dependencies.
export const relevantPaths = [
  "astro.config.mjs",
  "bun.lock",
  "package.json",
  "public",
  "src",
  "tsconfig.json"
];

const runGit = (args, cwd) =>
  spawnSync("git", args, {
    cwd,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"]
  });

const resolveCommit = (ref, cwd) => {
  if (!ref) return null;

  const result = runGit(["rev-parse", "--verify", `${ref}^{commit}`], cwd);
  return result.status === 0 ? result.stdout.trim() : null;
};

/**
 * Return true when Vercel should build the docs site.
 *
 * Vercel's ignore-command contract is inverted: exit 0 skips a build and
 * exit 1 continues it. Unknown Git state fails open and builds the site.
 */
export const shouldBuild = ({ cwd = process.cwd(), env = process.env } = {}) => {
  const rootResult = runGit(["rev-parse", "--show-toplevel"], cwd);
  if (rootResult.status !== 0) return true;

  const root = rootResult.stdout.trim();
  const head = resolveCommit(env.VERCEL_GIT_COMMIT_SHA ?? "HEAD", root);
  if (!head) return true;

  const baseRef = [
    env.VERCEL_GIT_PREVIOUS_SHA,
    env.VERCEL_GIT_PULL_REQUEST_BASE_SHA,
    env.SITE_DEPLOY_DIFF_BASE
  ].find(Boolean);
  const base = resolveCommit(baseRef, root);
  if (!base) return true;

  const mergeBaseResult = runGit(["merge-base", base, head], root);
  if (mergeBaseResult.status !== 0) return true;

  const comparisonBase = mergeBaseResult.stdout.trim();
  const diffResult = runGit(
    ["diff", "--quiet", comparisonBase, head, "--", ...relevantPaths],
    root
  );

  if (diffResult.status === 0) return false;
  if (diffResult.status === 1) return true;
  return true;
};

const isMainModule =
  process.argv[1] && fileURLToPath(import.meta.url) === resolve(process.argv[1]);

if (isMainModule) {
  process.exit(shouldBuild() ? 1 : 0);
}
