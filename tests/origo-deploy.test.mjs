import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const origoWorkflow = readFileSync(path.join(repo, '.github/workflows/origo-deploy.yml'), 'utf8');
const docsWorkflow = readFileSync(path.join(repo, '.github/workflows/docs-deploy.yml'), 'utf8');

const origoPaths = [
  'api/**', 'pkg/**', 'internal/app/**', 'internal/buildinfo/**',
  'go.mod', 'go.sum', 'vercel.mjs', '.github/workflows/origo-deploy.yml'
];

function eventPaths(workflow, event) {
  const block = workflow.match(new RegExp(`^  ${event}:\\n([\\s\\S]*?)(?=^  (?:push|pull_request|workflow_dispatch):|^permissions:)`, 'm'))?.[1];
  assert.ok(block, `${event} trigger missing`);
  assert.match(block, /branches: \[main\]/);
  return [...block.matchAll(/^      - '([^']+)'$/gm)].map(m => m[1]);
}

test('Origo deploys only for Go function inputs, never for docs-only changes', () => {
  for (const event of ['push', 'pull_request']) {
    assert.deepEqual(eventPaths(origoWorkflow, event), origoPaths);
  }
  assert.match(origoWorkflow, /^  workflow_dispatch:$/m);
  assert.match(origoWorkflow, /github\.ref == 'refs\/heads\/main'/);
  assert.doesNotMatch(origoWorkflow, /deploy --prebuilt/);
  assert.match(origoWorkflow, /vercel@latest deploy --prod --yes/);
});

test('Origo has independent credentials and leaves WebCTX Docs untouched', () => {
  assert.match(origoWorkflow, /VERCEL_TOKEN: \$\{\{ secrets\.VERCEL_ORIGO_TOKEN \}\}/);
  assert.match(origoWorkflow, /VERCEL_PROJECT_ID: \$\{\{ vars\.VERCEL_ORIGO_PROJECT_ID \}\}/);
  assert.match(origoWorkflow, /ORIGO_DEPLOYMENT: '1'/);
  assert.match(docsWorkflow, /VERCEL_TOKEN: \$\{\{ secrets\.VERCEL_TOKEN \}\}/);
  assert.match(docsWorkflow, /VERCEL_PROJECT_ID: \$\{\{ vars\.VERCEL_PROJECT_ID \}\}/);
  for (const source of ['api/**', 'pkg/**', 'internal/app/**', 'go.mod', 'go.sum']) {
    assert.ok(!docsWorkflow.includes(`- '${source}'`) && !docsWorkflow.includes(`- "${source}"`), `Docs mistakenly watches ${source}`);
  }
});

test('both Vercel project configurations disable Git auto-deploy and ignored builds', async () => {
  const previous = process.env.ORIGO_DEPLOYMENT;
  try {
    delete process.env.ORIGO_DEPLOYMENT;
    const docs = await import('../vercel.mjs?test=docs');
    assert.equal(docs.config.git.deploymentEnabled, false);
    assert.equal(Object.hasOwn(docs.config, 'ignoreCommand'), false);
    process.env.ORIGO_DEPLOYMENT = '1';
    const origo = await import('../vercel.mjs?test=origo');
    assert.equal(origo.config.git.deploymentEnabled, false);
    assert.equal(Object.hasOwn(origo.config, 'ignoreCommand'), false);
    assert.equal(origo.config.installCommand, '', 'Go-only Origo must skip the old Bun install override');
    assert.equal(origo.config.framework, null);
    assert.deepEqual(origo.config.rewrites, [{ source: '/mcp', destination: '/api/mcp' }]);
    assert.equal(origo.config.functions['api/*.go'].maxDuration, 300);
  } finally {
    if (previous === undefined) delete process.env.ORIGO_DEPLOYMENT;
    else process.env.ORIGO_DEPLOYMENT = previous;
  }
});
