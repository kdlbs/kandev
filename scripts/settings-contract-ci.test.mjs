import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import test from "node:test";

const root = resolve(new URL("..", import.meta.url).pathname);
const backend = resolve(root, "apps/backend");
const snapshotPath = resolve(root, "apps/web/lib/settings-discovery/contract.generated.json");
const profileSnapshotPath = resolve(
  root,
  "apps/web/lib/settings-discovery/profile-contract.generated.json",
);

function readJSON(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}

test("settings catalog snapshots are fresh", () => {
  execFileSync("go", ["run", "./cmd/settings-catalog", "--check"], {
    cwd: backend,
    stdio: "inherit",
  });
});

test("settings catalog has complete deterministic identities", () => {
  const snapshot = readJSON(snapshotPath);
  const profileSnapshot = readJSON(profileSnapshotPath);
  assert.equal(snapshot.schema_version, "settings-catalog.v1");
  assert.equal(snapshot.generated_by, "cmd/settings-catalog");
  assert.equal(profileSnapshot.schema_version, "settings-catalog.v1");

  const resourceTypes = new Set();
  for (const domain of snapshot.domains) {
    assert.ok(domain.resource_type);
    assert.ok(!resourceTypes.has(domain.resource_type), `duplicate ${domain.resource_type}`);
    resourceTypes.add(domain.resource_type);

    const keys = new Set();
    const paths = new Set();
    for (const field of domain.fields) {
      assert.ok(!keys.has(field.key), `duplicate field key ${field.key}`);
      assert.ok(!paths.has(field.field_path), `duplicate field path ${field.field_path}`);
      keys.add(field.key);
      paths.add(field.field_path);
      if (field.support === "supported" && field.writable) {
        assert.equal(field.classification, "writable", field.key);
        assert.ok(field.validator, field.key);
        assert.ok(field.authority, field.key);
      }
      assert.notEqual(field.support, "pending", field.key);
    }
  }

  for (const resourceType of [
    "agent_profile",
    "agent_profile_mcp",
    "user_settings",
    "workflow",
    "workflow_step",
    "workspace",
    "repository",
    "repository_script",
    "repository_set",
    "executor",
    "executor_profile",
    "environment",
    "task",
    "prompt",
    "utility_agent",
    "editor",
    "notification_provider",
    "automation",
    "automation_trigger",
    "runtime_flag",
    "storage_maintenance",
  ]) {
    assert.ok(resourceTypes.has(resourceType), `missing ${resourceType}`);
  }
});

test("compact settings tools keep closed schemas", () => {
  const source = readFileSync(
    resolve(root, "apps/backend/internal/mcp/server/settings_tools.go"),
    "utf8",
  );
  for (const tool of [
    "search_settings_kandev",
    "describe_setting_kandev",
    "get_settings_kandev",
    "update_settings_kandev",
    "list_settings_resources_kandev",
  ]) {
    assert.match(source, new RegExp(`NewToolWithRawSchema\\(\\n\\s*\\"${tool}\\"`));
  }
  assert.doesNotMatch(source, /WithEnum\(/);
});
