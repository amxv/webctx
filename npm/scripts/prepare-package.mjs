#!/usr/bin/env node
// Build the npm distribution without relocating or duplicating the Go module
// in the repository. The installed package retains its Go source fallback.
import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const npmRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repoRoot = path.resolve(npmRoot, '..');

export function preparePackage(outputDir = path.join(npmRoot, '.publish'), version) {
  const manifest = JSON.parse(readFileSync(path.join(npmRoot, 'package.json'), 'utf8'));
  if (version) manifest.version = version;

  // Do not ship development-only scripts that require the repository checkout.
  manifest.scripts = { postinstall: manifest.scripts.postinstall };

  rmSync(outputDir, { recursive: true, force: true });
  mkdirSync(outputDir, { recursive: true });

  for (const dir of ['cmd', 'internal', 'pkg']) {
    cpSync(path.join(repoRoot, dir), path.join(outputDir, dir), { recursive: true });
  }
  for (const file of ['go.mod', 'go.sum', 'README.md', 'LICENSE']) {
    cpSync(path.join(repoRoot, file), path.join(outputDir, file));
  }
  for (const file of ['bin/webctx.js', 'scripts/postinstall.js']) {
    const destination = path.join(outputDir, file);
    mkdirSync(path.dirname(destination), { recursive: true });
    cpSync(path.join(npmRoot, file), destination);
  }

  writeFileSync(path.join(outputDir, 'package.json'), `${JSON.stringify(manifest, null, 2)}\n`);
  return outputDir;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  console.log(`Prepared npm package at ${preparePackage(undefined, process.argv[2])}`);
}
