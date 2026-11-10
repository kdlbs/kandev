import {test} from 'node:test';
import assert from 'node:assert/strict';
import {assertUpgradeCI} from './ci-only.mjs';

test('upgrade workloads require GitHub Actions on main with no local bypass', () => {
  const allowed = {CI:'true', GITHUB_ACTIONS:'true', GITHUB_REF:'refs/heads/main'};
  assert.doesNotThrow(() => assertUpgradeCI(allowed));
  for (const env of [{}, {...allowed,GITHUB_REF:'refs/heads/feature/test-release-upgrade-7ee'}, {CI:'true'}, {...allowed,GITHUB_ACTIONS:'false'}, {...allowed,CI:'false'}, {...allowed,GITHUB_REF:'refs/heads/feature'}, {...allowed,GITHUB_REF:'refs/pull/1/merge'}]) {
    assert.throws(() => assertUpgradeCI(env), /only in GitHub Actions on main/);
  }
});
