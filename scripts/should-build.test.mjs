import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, it } from "node:test";
import { relevantPaths, shouldBuild } from "./should-build.mjs";

const temporaryRepositories = [];

const git = (cwd, ...args) => {
  const result = spawnSync("git", args, {
    cwd,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"]
  });
  assert.equal(result.status, 0, result.stderr);
  return result.stdout.trim();
};

const createRepository = () => {
  const cwd = mkdtempSync(join(tmpdir(), "webctx-vercel-ignore-"));
  temporaryRepositories.push(cwd);
  git(cwd, "init", "--quiet");
  git(cwd, "config", "user.email", "tests@example.com");
  git(cwd, "config", "user.name", "webctx tests");
  mkdirSync(join(cwd, "src"), { recursive: true });
  writeFileSync(join(cwd, "src", "index.astro"), "<h1>Docs</h1>\n");
  writeFileSync(join(cwd, "internal.go"), "package main\n");
  git(cwd, "add", ".");
  git(cwd, "commit", "--quiet", "-m", "initial");
  return cwd;
};

const commit = (cwd, message) => {
  git(cwd, "add", ".");
  git(cwd, "commit", "--quiet", "-m", message);
  return git(cwd, "rev-parse", "HEAD");
};

const decision = (cwd, base, head) =>
  shouldBuild({
    cwd,
    env: {
      VERCEL_GIT_PREVIOUS_SHA: base,
      VERCEL_GIT_COMMIT_SHA: head
    }
  });

afterEach(() => {
  for (const cwd of temporaryRepositories.splice(0)) {
    rmSync(cwd, { recursive: true, force: true });
  }
});

describe("Vercel docs ignore command", () => {
  it("builds when a docs source file changes", () => {
    const cwd = createRepository();
    const base = git(cwd, "rev-parse", "HEAD");
    writeFileSync(join(cwd, "src", "index.astro"), "<h1>Updated docs</h1>\n");
    const head = commit(cwd, "update docs");

    assert.equal(decision(cwd, base, head), true);
  });

  it("skips when only unrelated code changes", () => {
    const cwd = createRepository();
    const base = git(cwd, "rev-parse", "HEAD");
    writeFileSync(join(cwd, "internal.go"), "package main\n\nvar changed = true\n");
    const head = commit(cwd, "update cli");

    assert.equal(decision(cwd, base, head), false);
  });

  it("builds when docs files are renamed or deleted", () => {
    const cwd = createRepository();
    writeFileSync(join(cwd, "src", "old.astro"), "<p>Old</p>\n");
    const renameBase = commit(cwd, "add page");
    git(cwd, "mv", "src/old.astro", "src/new.astro");
    const renameHead = commit(cwd, "rename page");
    assert.equal(decision(cwd, renameBase, renameHead), true);

    const deleteBase = renameHead;
    git(cwd, "rm", "--quiet", "src/new.astro");
    const deleteHead = commit(cwd, "delete page");
    assert.equal(decision(cwd, deleteBase, deleteHead), true);
  });

  it("finds a docs change earlier in a multi-commit range", () => {
    const cwd = createRepository();
    const base = git(cwd, "rev-parse", "HEAD");
    writeFileSync(join(cwd, "src", "index.astro"), "<h1>Updated docs</h1>\n");
    commit(cwd, "update docs");
    writeFileSync(join(cwd, "internal.go"), "package main\n\nvar changed = true\n");
    const head = commit(cwd, "update cli");

    assert.equal(decision(cwd, base, head), true);
  });

  it("fails open when Git cannot resolve the comparison", () => {
    const cwd = createRepository();
    const head = git(cwd, "rev-parse", "HEAD");

    assert.equal(decision(cwd, "missing-base", head), true);
    assert.equal(decision(cwd, git(cwd, "rev-parse", "HEAD"), "missing-head"), true);
    assert.equal(
      shouldBuild({ cwd, env: { VERCEL_GIT_COMMIT_SHA: head } }),
      true
    );
  });

  it("keeps the intended docs paths in the deployment filter", () => {
    assert.deepEqual(relevantPaths, [
      "astro.config.mjs",
      "bun.lock",
      "package.json",
      "public",
      "src",
      "tsconfig.json"
    ]);
    const config = JSON.parse(
      readFileSync(new URL("../vercel.json", import.meta.url), "utf8")
    );
    assert.equal(config.ignoreCommand, "node scripts/should-build.mjs");
  });
});
