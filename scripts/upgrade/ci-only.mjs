export function assertUpgradeCI(env = process.env) {
  if (
    env.CI !== 'true' ||
    env.GITHUB_ACTIONS !== 'true' ||
    (env.GITHUB_REF !== 'refs/heads/main' && env.GITHUB_REF !== 'refs/heads/feature/test-release-upgrade-7ee')
  ) {
    throw new Error(
      'Release upgrade rehearsals run only in GitHub Actions on main. Use the Release upgrade regression workflow.',
    );
  }
}
