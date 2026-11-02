import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { validateRelease, verifyDigest, validateArchive } from './prepare-release.mjs';

test('only published Stable SemVer releases can seed an upgrade', () => {
  assert.equal(validateRelease({ tag_name: 'v0.97.0', draft: false, prerelease: false }), 'v0.97.0');
  for (const release of [
    {tag_name:'v0.97.0-nightly.sha123'}, {tag_name:'main'},
    {tag_name:'v0.97.0',draft:true}, {tag_name:'v0.97.0',prerelease:true},
  ]) assert.throws(() => validateRelease(release), /Stable/);
});
test('archive bytes must match the checksum for that exact filename', () => {
  const bytes = Buffer.from('release bytes');
  const digest = createHash('sha256').update(bytes).digest('hex');
  assert.equal(verifyDigest(bytes, `${digest}  runtime.tar.gz\n`, 'runtime.tar.gz'), digest);
  for (const value of [`${digest}  other.tar.gz`, `${'0'.repeat(64)}  runtime.tar.gz`, '']) {
    assert.throws(() => verifyDigest(bytes, value, 'runtime.tar.gz'), /checksum/);
  }
});
test('archive traversal, absolute paths and links cannot escape extraction', () => {
  validateArchive('runtime/\nruntime/kandev\nruntime/agentctl\n', '-rwxr-xr-x file\ndrwxr-xr-x dir\n');
  for (const name of ['../escape','runtime/../../escape','/absolute','runtime/../escape']) {
    assert.throws(() => validateArchive(`${name}\n`, '-rwxr-xr-x file\n'), /unsafe/);
  }
  for (const kind of ['l','h']) assert.throws(() => validateArchive('runtime/link\n', `${kind}rwx file\n`), /link/);
});

test('acquisition failure exits nonzero without publishing a manifest', async () => {
  const fs=await import('node:fs');
  const os=await import('node:os');
  const path=await import('node:path');
  const {spawnSync}=await import('node:child_process');
  const dir=fs.mkdtempSync(path.join(os.tmpdir(),'upgrade-download-test-'));
  try {
    fs.writeFileSync(path.join(dir,'gh'),'#!/bin/sh\necho "release download unavailable" >&2\nexit 42\n',{mode:0o755});
    const output=path.join(dir,'baseline');
    const result=spawnSync(process.execPath,['scripts/upgrade/prepare-release.mjs','--tag','latest','--out',output],{encoding:'utf8',env:{...process.env,CI:"true",GITHUB_ACTIONS:"true",GITHUB_REF:"refs/heads/main",PATH:dir}});
    assert.equal(result.status,1);
    assert.match(result.stderr,/acquisition:.*gh api/);
    assert.equal(fs.existsSync(path.join(output,'manifest.json')),false);
  } finally {fs.rmSync(dir,{recursive:true,force:true});}
});
