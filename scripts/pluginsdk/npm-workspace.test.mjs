import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import {
  cp,
  mkdtemp,
  mkdir,
  readFile,
  rm,
  symlink,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import test from "node:test";
import { packCandidate } from "./pack-candidate.mjs";
import { npmEnvironment, startRegistry } from "./npm-consumer.mjs";

// @covers AC-PLUGINS-SDK-DISTRIBUTION-002.2
test("checker follows an npm workspace's installed resolution and binds its exact lock entry", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "sdk-npm-workspace-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const run = promisify(execFile);
  const source = join(root, "source");
  await cp(
    fileURLToPath(new URL("../../apps/packages/plugin-sdk", import.meta.url)),
    source,
    {
      recursive: true,
      filter: (path) => !path.split("/").includes("node_modules"),
    },
  );
  const record = JSON.parse(
    await readFile(join(source, "sdk-release.json"), "utf8"),
  );
  record.host_release = "0.100.0";
  record.min_kandev_version = "0.100.0";
  await writeFile(join(source, "sdk-release.json"), JSON.stringify(record));
  const candidate = await packCandidate({
    directory: source,
    output: join(root, "packed"),
  });
  const registry = await startRegistry(candidate);
  t.after(() => registry.close());
  const workspace = join(root, "workspace");
  const cwd = join(workspace, "packages/plugin");
  await mkdir(cwd, { recursive: true });
  await writeFile(
    join(workspace, "package.json"),
    JSON.stringify({ private: true, workspaces: ["packages/*"] }),
  );
  await writeFile(
    join(cwd, "package.json"),
    JSON.stringify({
      name: "workspace-plugin",
      private: true,
      dependencies: { "@kdlbs/plugin-sdk": record.sdk_version },
    }),
  );
  const env = npmEnvironment(root);
  await run("npm", ["install", "--registry", registry.url], {
    cwd: workspace,
    env,
    timeout: 180_000,
  });
  const binary = join(root, "checker");
  await run("go", ["build", "-o", binary, "./cmd/kandev-sdk-check"], {
    cwd: fileURLToPath(new URL("../../pluginsdk", import.meta.url)),
    env: { ...process.env, GOWORK: "off" },
  });
  await writeFile(
    join(cwd, "manifest.yaml"),
    "api_version: 2\nmin_kandev_version: 0.100.0\n",
  );
  const lockPath = join(workspace, "package-lock.json");
  const before = await readFile(lockPath, "utf8");
  assert.ok(JSON.parse(before).packages["node_modules/@kdlbs/plugin-sdk"]);
  const check = (path = "node_modules/@kdlbs/plugin-sdk") =>
    run(binary, ["--frontend-only", "--frontend-sdk", path], { cwd, env });
  assert.match((await check()).stdout, /compatible/);
  assert.match(
    (await check("../../node_modules/@kdlbs/plugin-sdk")).stdout,
    /compatible/,
  );
  const alias = join(root, "plugin-link");
  await symlink(cwd, alias, "dir");
  for (const path of [
    "node_modules/@kdlbs/plugin-sdk",
    join(alias, "node_modules/@kdlbs/plugin-sdk"),
    join(workspace, "node_modules/@kdlbs/plugin-sdk"),
  ]) {
    const result = await run(
      binary,
      ["--frontend-only", "--frontend-sdk", path],
      { cwd: alias, env: { ...env, PWD: alias } },
    );
    assert.match(result.stdout, /compatible/);
  }
  assert.equal(await readFile(lockPath, "utf8"), before);

  const installed = join(workspace, "node_modules/@kdlbs/plugin-sdk");
  const shadow = join(cwd, "node_modules/@kdlbs/plugin-sdk");
  await cp(installed, shadow, { recursive: true });
  await assert.rejects(check(), (error) =>
    /no registry SDK resolution/.test(error.stderr),
  );
  await assert.rejects(
    check("../../node_modules/@kdlbs/plugin-sdk"),
    (error) => /resolved installed/.test(error.stderr),
  );
  await rm(shadow, { recursive: true });
  await symlink(installed, shadow);
  await assert.rejects(check(), (error) =>
    /local substitution/.test(error.stderr),
  );
  await rm(shadow);

  const lock = JSON.parse(before);
  lock.packages["node_modules/@kdlbs/plugin-sdk"].version = "0.2.0";
  await writeFile(lockPath, JSON.stringify(lock));
  await assert.rejects(check(), (error) => /lock version/.test(error.stderr));
  // A nearer lockfile cannot authorize a package installed outside its tree.
  await writeFile(join(cwd, "package-lock.json"), before);
  await assert.rejects(check(), (error) =>
    /installed frontend SDK/.test(error.stderr),
  );
});
