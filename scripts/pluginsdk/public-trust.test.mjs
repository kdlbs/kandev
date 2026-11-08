import test from "node:test";
import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { validatePublicTrust } from "./public-trust.mjs";
const run = promisify(execFile);
const cleanGo = {
  GOPROXY: "https://proxy.golang.org,direct",
  GOSUMDB: "sum.golang.org",
  GONOSUMDB: "",
  GONOPROXY: "",
  GOPRIVATE: "",
  GOINSECURE: "",
  GOFLAGS: "",
};
const read = async (cmd) => ({
  stdout: JSON.stringify(
    cmd === "go" ? cleanGo : { registry: "https://registry.npmjs.org/" },
  ),
});
test("rejects each effective public trust bypass and scoped registry", async () => {
  await validatePublicTrust({ run: read, env: {} });
  for (const [key, value] of Object.entries({
    GOPROXY: "file:///tmp/proxy",
    GOSUMDB: "off",
    GOPRIVATE: "*",
    GONOPROXY: "*",
    GONOSUMDB: "*",
    GOINSECURE: "*",
    GOFLAGS: "-modfile=hidden",
  })) {
    await assert.rejects(
      validatePublicTrust({
        env: {},
        run: async (cmd) => ({
          stdout: JSON.stringify(
            cmd === "go"
              ? { ...cleanGo, [key]: value }
              : { registry: "https://registry.npmjs.org/" },
          ),
        }),
      }),
      /public trust/,
    );
  }
  await assert.rejects(
    validatePublicTrust({
      env: {},
      run: async (cmd) => ({
        stdout: JSON.stringify(
          cmd === "go"
            ? cleanGo
            : {
                registry: "https://registry.npmjs.org/",
                "@kandev:registry": "http://localhost:1234",
              },
        ),
      }),
    }),
    /public trust/,
  );
});
test("reads inherited Go configuration files before isolating caches", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "sdk-goenv-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const config = join(root, "goenv");
  await writeFile(config, "GONOSUMDB=*\n");
  await assert.rejects(
    validatePublicTrust({ cwd: root, env: { ...process.env, GOENV: config } }),
    /public trust.*GONOSUMDB/,
  );
});
test("reads inherited scoped npm configuration before isolating caches", async (t) => {
  const root = await mkdtemp(join(tmpdir(), "sdk-npmrc-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const config = join(root, "npmrc");
  await writeFile(config, "@kandev:registry=http://127.0.0.1:1234\n");
  await assert.rejects(
    validatePublicTrust({
      cwd: root,
      env: {
        ...process.env,
        ...cleanGo,
        GOENV: "off",
        NPM_CONFIG_USERCONFIG: config,
      },
    }),
    /public trust.*registry/,
  );
});
