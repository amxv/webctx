const assert = require("node:assert/strict");
const test = require("node:test");

const { resolvePlatform, releaseAssetName } = require("./postinstall.js");

const releaseTargets = [
  ["darwin", "x64", "darwin", "amd64", "webctx_darwin_amd64"],
  ["darwin", "arm64", "darwin", "arm64", "webctx_darwin_arm64"],
  ["linux", "x64", "linux", "amd64", "webctx_linux_amd64"],
  ["linux", "arm64", "linux", "arm64", "webctx_linux_arm64"],
  ["win32", "ia32", "windows", "386", "webctx_windows_386.exe"],
  ["win32", "x64", "windows", "amd64", "webctx_windows_amd64.exe"],
  ["win32", "arm64", "windows", "arm64", "webctx_windows_arm64.exe"]
];

for (const [platform, arch, goos, goarch, asset] of releaseTargets) {
  test(`resolves ${platform}/${arch} to ${asset}`, () => {
    assert.deepEqual(resolvePlatform(platform, arch), { goos, goarch });
    assert.equal(releaseAssetName("webctx", goos, goarch), asset);
  });
}

test("does not claim 32-bit macOS/Linux release assets", () => {
  assert.equal(resolvePlatform("darwin", "ia32").goarch, undefined);
  assert.equal(resolvePlatform("linux", "ia32").goarch, undefined);
});

test("does not claim unsupported OS or CPU release assets", () => {
  assert.equal(resolvePlatform("freebsd", "x64").goos, undefined);
  assert.equal(resolvePlatform("win32", "mips").goarch, undefined);
});
