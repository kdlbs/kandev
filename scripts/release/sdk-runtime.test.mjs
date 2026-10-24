import test from "node:test";
import assert from "node:assert/strict";
import {
  assertReleaseContext,
  publicationEnvironment,
} from "./sdk-runtime.mjs";
const approved = {
  GITHUB_ACTIONS: "true",
  GITHUB_REPOSITORY: "kdlbs/kandev",
  GITHUB_EVENT_NAME: "workflow_dispatch",
  GITHUB_REF: "refs/heads/main",
  GITHUB_WORKFLOW_REF:
    "kdlbs/kandev/.github/workflows/release.yml@refs/heads/main",
  RUNNER_ENVIRONMENT: "github-hosted",
  SDK_SCOPE_OWNERSHIP_CONFIRMED: "true",
};
test("release mutations require the canonical protected workflow and confirmed scope ownership", () => {
  assertReleaseContext(approved);
  for (const key of Object.keys(approved))
    assert.throws(
      () => assertReleaseContext({ ...approved, [key]: "other" }),
      /release|scope|workflow|runner/,
    );
});
test("SDK npm publication has fresh config and no token fallback", () => {
  const env = publicationEnvironment("/tmp/fresh", {
    PATH: "/bin",
    NPM_TOKEN: "secret",
    NODE_AUTH_TOKEN: "secret",
    npm_config_registry: "http://local",
    ACTIONS_ID_TOKEN_REQUEST_URL: "https://actions",
  });
  assert.equal(env.NPM_TOKEN, undefined);
  assert.equal(env.NODE_AUTH_TOKEN, undefined);
  assert.equal(env.npm_config_registry, undefined);
  assert.equal(env.NPM_CONFIG_USERCONFIG, "/tmp/fresh/user.npmrc");
  assert.equal(env.NPM_CONFIG_GLOBALCONFIG, "/tmp/fresh/global.npmrc");
  assert.equal(env.ACTIONS_ID_TOKEN_REQUEST_URL, "https://actions");
});
