import assert from "node:assert/strict";
import test from "node:test";
import { planSDKRelease, isContractInput } from "./plugin-sdk.mjs";

const draft = () => ({
  schema_version: 1,
  sdk_version: "0.1.0",
  host_release: null,
  min_kandev_version: null,
  api_versions: [1, 2],
  change_kind: "initial",
});
const published = () => ({
  ...draft(),
  host_release: "0.100.0",
  min_kandev_version: "0.100.0",
});
// @covers AC-PLUGINS-SDK-DISTRIBUTION-002.1
test("first Stable preparation resolves only draft host fields", () => {
  const current = draft();
  assert.deepEqual(
    planSDKRelease(current, null, ["pluginsdk/serve.go"], "0.100.0"),
    { record: published(), reused: false },
  );
  assert.equal(current.host_release, null);
});
test("unchanged SDK retains its original mapping even when later host README bytes differ", () => {
  assert.deepEqual(
    planSDKRelease(
      published(),
      published(),
      ["apps/packages/plugin-sdk/README.md"],
      "0.101.0",
    ),
    { record: published(), reused: true },
  );
  assert.throws(
    () =>
      planSDKRelease(
        published(),
        published(),
        ["pluginsdk/serve.go"],
        "0.101.0",
      ),
    /contract.*version/,
  );
  assert.throws(
    () =>
      planSDKRelease(
        { ...published(), min_kandev_version: "0.99.0" },
        published(),
        [],
        "0.101.0",
      ),
    /record.*version/,
  );
});
test("shared version progression is intentional and respects pre-1.0 breaking changes", () => {
  const old = published();
  const next = {
    ...draft(),
    sdk_version: "0.1.1",
    change_kind: "patch",
    min_kandev_version: old.min_kandev_version,
  };
  assert.equal(
    planSDKRelease(next, old, ["pluginsdk/serve.go"], "0.101.0").record
      .min_kandev_version,
    "0.100.0",
  );
  assert.throws(
    () =>
      planSDKRelease({ ...next, change_kind: "breaking" }, old, [], "0.101.0"),
    /minor/,
  );
  assert.equal(
    planSDKRelease(
      { ...next, sdk_version: "0.2.0", change_kind: "breaking" },
      old,
      [],
      "0.101.0",
    ).record.sdk_version,
    "0.2.0",
  );
  assert.throws(
    () => planSDKRelease({ ...next, sdk_version: "0.0.9" }, old, [], "0.101.0"),
    /increase/,
  );
  assert.throws(
    () =>
      planSDKRelease({ ...next, change_kind: "initial" }, old, [], "0.101.0"),
    /initial/,
  );
});
test("invalid records and mismatched originating releases stop preparation", () => {
  for (const current of [
    { ...draft(), sdk_version: "0.1" },
    { ...draft(), schema_version: 2 },
    { ...draft(), api_versions: [] },
    { ...draft(), api_versions: [1, 1] },
    { ...draft(), min_kandev_version: "0.102.0" },
    { ...draft(), host_release: "0.99.0" },
  ]) {
    assert.throws(() => planSDKRelease(current, null, [], "0.100.0"));
  }
});
test("one inventory covers public Go, protobuf, frontend and host version contracts", () => {
  for (const path of [
    "pluginsdk/serve.go",
    "pluginsdk/go.mod",
    "pluginsdk/proto/kandev/plugin/v1/plugin.proto",
    "pluginsdk/internal/compatibility/check.go",
    "apps/packages/plugin-sdk/src/index.ts",
    "apps/packages/plugin-sdk/package.json",
    "apps/web/lib/plugins/host-api.ts",
    "apps/backend/internal/plugins/manifest/manifest.go",
  ])
    assert.equal(isContractInput(path), true, path);
  for (const path of [
    "pluginsdk/README.md",
    "pluginsdk/serve_test.go",
    "pluginsdk/sdk-release.json",
    "apps/packages/plugin-sdk/test/example.node.mjs",
    "apps/web/components/example.tsx",
  ])
    assert.equal(isContractInput(path), false, path);
});

test("check mode validates draft intent without guessing a host release", () => {
  assert.deepEqual(planSDKRelease(draft(), null, [], null), {
    record: draft(),
    reused: false,
  });
});

test("release stamping changes only canonical and generated metadata in a disposable checkout", async () => {
  const { mkdtemp, mkdir, readFile, writeFile, rm } =
    await import("node:fs/promises");
  const { tmpdir } = await import("node:os");
  const { join } = await import("node:path");
  const { execFileSync } = await import("node:child_process");
  const { prepareMetadata } = await import("./plugin-sdk.mjs");
  const root = await mkdtemp(join(tmpdir(), "sdk-release-record-"));
  try {
    execFileSync("git", ["init", "--quiet"], { cwd: root });
    await mkdir(join(root, "pluginsdk"));
    await mkdir(join(root, "apps/packages/plugin-sdk"), { recursive: true });
    const original = JSON.stringify(draft(), null, 2) + "\n";
    for (const path of [
      "pluginsdk/sdk-release.json",
      "apps/packages/plugin-sdk/sdk-release.json",
    ])
      await writeFile(join(root, path), original);
    await writeFile(
      join(root, "apps/packages/plugin-sdk/package.json"),
      JSON.stringify({ name: "@kdlbs/plugin-sdk", version: "0.1.0" }),
    );
    await prepareMetadata({ root, hostVersion: "0.100.0", stamp: false });
    assert.equal(
      await readFile(join(root, "pluginsdk/sdk-release.json"), "utf8"),
      original,
    );
    const result = await prepareMetadata({
      root,
      hostVersion: "0.100.0",
      stamp: true,
    });
    assert.equal(result.record.host_release, "0.100.0");
    assert.equal(
      await readFile(join(root, "pluginsdk/sdk-release.json"), "utf8"),
      await readFile(
        join(root, "apps/packages/plugin-sdk/sdk-release.json"),
        "utf8",
      ),
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
