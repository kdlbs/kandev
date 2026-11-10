import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { assertUpgradeCI } from './ci-only.mjs';

const repo = 'kdlbs/kandev';
export function validateRelease(release) {
  if (release.draft || release.prerelease || !/^v\d+\.\d+\.\d+$/.test(release.tag_name)) {
    throw new Error('baseline must be a published Stable SemVer release');
  }
  return release.tag_name;
}
export function verifyDigest(bytes, checksum, filename) {
  const lines = checksum.trim().split('\n');
  const entry = lines.find(line => {
    const parts = line.trim().split(/\s+/);
    return parts.length === 1 || parts[1].replace(/^\*/, '') === filename;
  });
  const expected = entry?.trim().split(/\s+/)[0];
  const actual = createHash('sha256').update(bytes).digest('hex');
  if (!/^[a-f0-9]{64}$/.test(expected ?? '') || actual !== expected) {
    throw new Error(`checksum mismatch or missing checksum: ${filename}`);
  }
  return actual;
}
export function validateArchive(names, listing) {
  for (const name of names.trim().split('\n')) {
    if (!name || name.startsWith('/') || name.includes('\\') || name.split('/').includes('..')) {
      throw new Error(`unsafe archive path: ${name}`);
    }
  }
  for (const line of listing.trim().split('\n')) {
    if (!['-', 'd'].includes(line[0])) throw new Error('archive link or special file is forbidden');
  }
}
const gh = args => execFileSync('gh', args, { encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
export function prepareRelease(tag, output) {
  assertUpgradeCI();
  const metadata = JSON.parse(gh(['api', `repos/${repo}/releases/${tag === 'latest' ? 'latest' : `tags/${encodeURIComponent(tag)}`}`]));
  const resolvedTag = validateRelease(metadata);
  const source = JSON.parse(gh(['api', `repos/${repo}/commits/${resolvedTag}`])).sha;
  if (!/^[a-f0-9]{40}$/.test(source)) throw new Error('missing baseline source commit');
  const platform = { linux: 'linux', darwin: 'macos' }[process.platform];
  const arch = { x64: 'x64', arm64: 'arm64' }[process.arch];
  if (!platform || !arch) throw new Error('upgrade rehearsal requires Linux or macOS x64/arm64');
  const archive = `kandev-${platform}-${arch}.tar.gz`;
  for (const name of [archive, `${archive}.sha256`]) {
    if (!metadata.assets.some(asset => asset.name === name)) throw new Error(`release artifact missing: ${name}`);
  }
  const dir = path.resolve(output);
  if (fs.existsSync(path.join(dir, 'manifest.json'))) throw new Error(`baseline already prepared: ${dir}; use a new output directory`);
  fs.mkdirSync(dir, { recursive: true });
  gh(['release', 'download', resolvedTag, '--repo', repo, '--dir', dir, '--pattern', archive, '--pattern', `${archive}.sha256`, '--clobber']);
  const digest = verifyDigest(fs.readFileSync(path.join(dir, archive)), fs.readFileSync(path.join(dir, `${archive}.sha256`), 'utf8'), archive);
  const tar = args => execFileSync('tar', args, { encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
  validateArchive(tar(['-tzf', path.join(dir, archive)]), tar(['-tvzf', path.join(dir, archive)]));
  const runtime = path.join(dir, 'runtime');
  fs.mkdirSync(runtime, {recursive:true});
  tar(['-xzf', path.join(dir, archive), '-C', runtime, '--no-same-owner']);
  function find(name, dir) {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const p = path.join(dir, entry.name);
      if (entry.isDirectory()) { const nested = find(name, p); if (nested) return nested; }
      else if (entry.name === name) return p;
    }
  }
  const binary = find('kandev', runtime);
  const agentctl = find('agentctl', runtime);
  if (!binary || !agentctl) throw new Error('release runtime lacks kandev/agentctl');
  // The released backend remains untouched; only its deterministic test provider
  // is built from the matching source revision.
  try { execFileSync('git', ['cat-file', '-e', `${source}^{commit}`], {stdio:'pipe'}); }
  catch { execFileSync('git', ['fetch', '--no-tags', '--depth=1', `https://github.com/${repo}.git`, source], {stdio:'inherit'}); }
  const sourceDir = path.join(dir, 'source');
  fs.mkdirSync(sourceDir, {recursive:true});
  execFileSync('bash', ['-c', 'set -o pipefail; git archive "$1" apps/backend profiles.yaml | tar -x -C "$2"', 'upgrade-source', source, sourceDir], {stdio:'inherit'});
  const mock = path.join(path.dirname(binary), 'mock-agent');
  execFileSync('go', ['build', '-o', mock, './cmd/mock-agent'], {cwd: path.join(sourceDir, 'apps/backend'), stdio:'inherit'});
  fs.rmSync(sourceDir, {recursive:true});
  const hash = p => createHash('sha256').update(fs.readFileSync(p)).digest('hex');
  const manifest = { version: 1, tag: resolvedTag, source_commit: source, archive, archive_sha256: digest,
    binary: path.relative(dir, binary), binary_sha256: hash(binary), mock_agent: path.relative(dir, mock), mock_agent_sha256: hash(mock), agentctl: path.relative(dir, agentctl), agentctl_sha256: hash(agentctl) };
  fs.writeFileSync(path.join(dir, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n', { flag: 'wx' });
  return manifest;
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const option = (key, fallback) => args.includes(key) ? args[args.indexOf(key) + 1] : fallback;
  try { console.log(JSON.stringify(prepareRelease(option('--tag', 'latest'), option('--out', '.cache/upgrade/baseline')), null, 2)); }
  catch (error) { console.error(`acquisition: ${error.message}`); process.exitCode = 1; }
}
