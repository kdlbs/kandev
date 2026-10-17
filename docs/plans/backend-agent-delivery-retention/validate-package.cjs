'use strict';

// Read-only structure validation, usable during planning and implementation.
// Lifecycle statuses, Results content and Git staging are intentionally unconstrained.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '../../..');
const { parseFrontmatter, validateCoverage } = require(path.join(root, '.github/scripts/pr-docs.cjs'));
const relative = absolute => path.relative(root, absolute).split(path.sep).join('/');
const read = file => fs.readFileSync(path.join(root, file), 'utf8');
const planPath = relative(path.join(__dirname, 'plan.md'));
const requirementPath = 'docs/specs/platform/requirements/backend-agent-delivery-retention.md';
const designPath = 'docs/specs/platform/system-design/backend-agent-delivery-retention.md';
const adrPath = 'docs/decisions/2026-10-05-backend-delivery-retirement-checkpoints.md';
const orders = fs.readdirSync(__dirname).filter(name => /^task-\d{2}-.*\.md$/.test(name)).sort()
  .map(name => relative(path.join(__dirname, name)));
assert.equal(orders.length, 8);
const documents = [planPath, requirementPath, designPath, adrPath, ...orders];
const fileContents = Object.fromEntries(documents.map(file => [file, read(file)]));
const plan = parseFrontmatter(fileContents[planPath]);
const requirement = parseFrontmatter(fileContents[requirementPath]);
const design = parseFrontmatter(fileContents[designPath]);
assert.ok(Buffer.byteLength(fileContents[designPath]) <= 32768);
assert.ok(Buffer.byteLength(fileContents[requirementPath]) <= 20480);
const reqIds = [...requirement.body.matchAll(/^### (REQ-[A-Z0-9-]+):/gm)].map(match => match[1]);
const acIds = [...requirement.body.matchAll(/\*\*(AC-[A-Z0-9-]+\.\d+):\*\*/g)].map(match => match[1]);
assert.equal(new Set(reqIds).size, reqIds.length);
assert.equal(new Set(acIds).size, acIds.length);
assert.deepEqual([...plan.data.requirements].sort(), [...reqIds].sort());
assert.deepEqual([...design.data.requirements].sort(), [...reqIds].sort());
const covered = new Set();
const ids = new Set();
let links = 0;
let existingFiles = 0;
for (const file of documents) {
  assert.ok(fileContents[file].endsWith('\n'), `${file}: missing final newline`);
  assert.ok(!/[ \t]+$/m.test(fileContents[file]), `${file}: trailing whitespace`);
  for (const match of fileContents[file].matchAll(/\[[^\]]+\]\(([^)]+)\)/g)) {
    const target = match[1].split('#')[0];
    if (!target || /^[a-z]+:\/\//i.test(target)) continue;
    const absolute = path.resolve(root, path.dirname(file), target);
    assert.ok(absolute.startsWith(root + path.sep), `${file}: escaping link ${target}`);
    assert.ok(fs.existsSync(absolute), `${file}: missing link ${target}`);
    links++;
  }
}
for (const [index, file] of orders.entries()) {
  const order = parseFrontmatter(fileContents[file]);
  assert.equal(order.data.wave, index + 1);
  assert.ok(!ids.has(order.data.id));
  for (const dependency of order.data.depends_on) assert.ok(ids.has(dependency), `${file}: invalid dependency ${dependency}`);
  ids.add(order.data.id);
  for (const id of order.data.requirements) assert.ok(reqIds.includes(id), `${file}: missing requirement ${id}`);
  for (const id of order.data.acceptance_criteria) {
    assert.ok(acIds.includes(id), `${file}: missing AC ${id}`);
    assert.ok(order.data.requirements.includes('REQ-' + id.slice(3).split('.')[0]));
    covered.add(id);
  }
  for (const ref of order.data.system_design) {
    assert.equal(relative(path.resolve(root, path.dirname(file), ref)), designPath);
  }
  assert.match(order.body, /## TDD evidence/);
  assert.match(order.body, /## Verification[\s\S]+```bash/);
  assert.match(order.body, /## Results\s+\S/);
  const owned = order.body.split('## Files likely touched\n')[1].split('## Dependencies')[0];
  for (const match of owned.matchAll(/^- `([^`]+)`(.*)$/gm)) {
    if (match[2].includes('(new)')) continue;
    assert.ok(fs.existsSync(path.join(root, match[1])), `${file}: missing owned path ${match[1]}`);
    existingFiles++;
  }
}
assert.deepEqual([...covered].sort(), [...acIds].sort(), 'All ACs must have work-order coverage');
const changedFiles = documents.map(filename => ({ filename, status: 'added' }));
const actual = validateCoverage({ changedFiles, fileContents });
assert.ok(actual.ok, JSON.stringify(actual.errors));
// Exercise reference validation despite the real documentation-only diff being exempt.
const projected = validateCoverage({
  changedFiles: [...changedFiles, { filename: 'apps/backend/internal/task/repository/sqlite/agent_delivery_retention.go', status: 'added' }],
  fileContents,
});
assert.ok(projected.ok, JSON.stringify(projected.errors));
assert.equal(projected.status, 'covered');
const baseline = 'd7a44e96ede93d50c6277773f4174de716272f02';
const mergedDelivery = '349f61bc52770f0140e3b7a2df9cd66aa4231b03';
execFileSync('git', ['merge-base', '--is-ancestor', baseline, 'HEAD'], { cwd: root });
execFileSync('git', ['merge-base', '--is-ancestor', mergedDelivery, 'HEAD'], { cwd: root });
console.log(JSON.stringify({
  documents: documents.length, workOrders: orders.length, requirements: reqIds.length,
  acceptanceCriteria: acIds.length, covered: covered.size, localLinks: links,
  existingOwnedPaths: existingFiles, actualDocsPreflight: actual.status,
  projectedImplementationPreflight: projected.status, ancestry: 'passed', structure: 'passed',
}, null, 2));
