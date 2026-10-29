import assert from 'node:assert/strict';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { preparePackage } from './prepare-package.mjs';

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');

test('repository root stays Go-only and docs have independent Node dependencies', () => {
  assert.equal(existsSync(path.join(repo, 'package.json')), false);
  const docs = JSON.parse(readFileSync(path.join(repo, 'docs/package.json'), 'utf8'));
  const npm = JSON.parse(readFileSync(path.join(repo, 'npm/package.json'), 'utf8'));
  assert.equal(docs.private, true);
  assert.ok(docs.devDependencies.astro);
  assert.equal(npm.devDependencies, undefined);
  assert.equal(npm.name, 'webctx');
});

test('npm distribution contains the CLI and Go source fallback, but no Astro site', (t) => {
  const root = mkdtempSync(path.join(tmpdir(), 'webctx-npm-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  preparePackage(root, '9.8.7');

  const manifest = JSON.parse(readFileSync(path.join(root, 'package.json'), 'utf8'));
  assert.equal(manifest.name, 'webctx');
  assert.equal(manifest.version, '9.8.7');
  assert.deepEqual(manifest.scripts, { postinstall: 'node scripts/postinstall.js' });
  assert.equal(manifest.bin.webctx, 'bin/webctx.js');

  for (const file of [
    'bin/webctx.js', 'scripts/postinstall.js', 'cmd/webctx/main.go',
    'internal/app/app.go', 'internal/buildinfo/buildinfo.go',
    'pkg/retrieval/retrieval.go', 'go.mod', 'go.sum', 'README.md', 'LICENSE'
  ]) {
    assert.ok(existsSync(path.join(root, file)), `${file} is missing from npm package`);
  }
  for (const file of ['docs', 'src', 'astro.config.mjs', 'bun.lock']) {
    assert.equal(existsSync(path.join(root, file)), false, `${file} should not be published`);
  }
});
