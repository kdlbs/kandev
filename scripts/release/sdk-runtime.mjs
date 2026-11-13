import { join } from "node:path";

/** Contains the required setting and value, never the supplied value. */
export class ReleaseContextError extends Error {
  constructor(setting, expected) {
    super(`SDK release requires ${setting}=${expected}`);
    this.name = "ReleaseContextError";
    this.setting = setting;
    this.expected = expected;
  }
}

export function assertReleaseContext(env = process.env) {
  const required = {
    GITHUB_ACTIONS: "true",
    GITHUB_REPOSITORY: "kdlbs/kandev",
    GITHUB_EVENT_NAME: "workflow_dispatch",
    GITHUB_REF: "refs/heads/main",
    GITHUB_WORKFLOW_REF:
      "kdlbs/kandev/.github/workflows/release.yml@refs/heads/main",
    RUNNER_ENVIRONMENT: "github-hosted",
    SDK_SCOPE_OWNERSHIP_CONFIRMED: "true",
  };
  for (const [key, value] of Object.entries(required))
    if (env[key] !== value) throw new ReleaseContextError(key, value);
}
export function publicationEnvironment(root, inherited = process.env) {
  const env = Object.fromEntries(
    Object.entries(inherited).filter(
      ([key]) =>
        !/^npm_config_/i.test(key) &&
        !["NPM_TOKEN", "NODE_AUTH_TOKEN"].includes(key),
    ),
  );
  return {
    ...env,
    NPM_CONFIG_CACHE: join(root, "npm-cache"),
    NPM_CONFIG_USERCONFIG: join(root, "user.npmrc"),
    NPM_CONFIG_GLOBALCONFIG: join(root, "global.npmrc"),
    NPM_CONFIG_REGISTRY: "https://registry.npmjs.org",
    NPM_CONFIG_IGNORE_SCRIPTS: "true",
  };
}
