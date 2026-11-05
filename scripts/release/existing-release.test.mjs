import assert from "node:assert/strict";
import test from "node:test";
import { createHash } from "node:crypto";
import {
  mkdtemp,
  mkdir,
  readFile,
  writeFile,
  readdir,
  rm,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import {
  buildRemoteHelperArtifact,
  REMOTE_HELPERS,
} from "./remote-helper-assets.mjs";
import {
  inspectPublishedRelease,
  verifyPublishedAssets,
  downloadReleaseAsset,
} from "./existing-release.mjs";

const tag = "v0.99.0";
const commit = "a".repeat(40);
const fingerprint = "B".repeat(40);
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
const release = () => ({
  id: 1,
  tag_name: tag,
  draft: false,
  prerelease: false,
  immutable: true,
  assets: [],
});
function commands(value = release()) {
  const calls = [];
  const run = async (binary, args) => {
    calls.push([binary, ...args]);
    if (binary === "gh") return { stdout: JSON.stringify(value) };
    if (binary === "git" && args[0] === "rev-parse") return { stdout: commit };
    if (binary === "git") return { stderr: `[GNUPG:] VALIDSIG ${fingerprint}` };
    if (args.includes("--show-keys"))
      return { stdout: `pub:::::::::\nfpr:::::::::${fingerprint}:\n` };
    return { stdout: "" };
  };
  return { run, calls };
}
test("immutable release reuse verifies its signed host source without GitHub writes", async () => {
  const f = commands();
  const result = await inspectPublishedRelease({
    tag,
    commit,
    root: ".",
    run: f.run,
  });
  assert.equal(result.reuse, true);
  assert.ok(
    f.calls.some(
      ([binary, command]) => binary === "git" && command === "verify-tag",
    ),
  );
  assert.equal(f.calls.filter(([binary]) => binary === "gh").length, 1);
});
test("only a confirmed HTTP 404 selects absence; mutable publication stays enabled", async () => {
  const absent = await inspectPublishedRelease({
    tag,
    commit,
    root: ".",
    run: async () => {
      throw new Error("gh: Not Found (HTTP 404)");
    },
  });
  assert.equal(absent.reuse, false);
  for (const message of ["HTTP 403", "timeout", "Not Found", "HTTP 500"]) {
    await assert.rejects(
      inspectPublishedRelease({
        tag,
        commit,
        root: ".",
        run: async () => {
          throw new Error(message);
        },
      }),
      new RegExp(message),
    );
  }
  const f = commands({ ...release(), immutable: false });
  assert.equal(
    (await inspectPublishedRelease({ tag, commit, root: ".", run: f.run }))
      .reuse,
    false,
  );
});
test("malformed JSON mentioning HTTP 404 is not release absence", async () => {
  await assert.rejects(
    inspectPublishedRelease({
      tag,
      commit,
      root: ".",
      run: async () => ({ stdout: "HTTP 404" }),
    }),
    SyntaxError,
  );
});
test("invalid release metadata and mismatched source fail closed", async () => {
  for (const value of [
    {},
    { ...release(), tag_name: "v1.0.0" },
    { ...release(), prerelease: true },
    { ...release(), immutable: undefined },
  ]) {
    await assert.rejects(
      inspectPublishedRelease({
        tag,
        commit,
        root: ".",
        run: commands(value).run,
      }),
      /release/,
    );
  }
  const f = commands();
  await assert.rejects(
    inspectPublishedRelease({
      tag,
      commit: "c".repeat(40),
      root: ".",
      run: f.run,
    }),
    /source/,
  );
});

async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), "existing-release-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const expectedDir = join(root, "expected");
  await mkdir(expectedDir);
  const payloads = new Map();
  const value = release();
  async function add(name, data, staged = true) {
    const bytes = Buffer.from(data);
    if (staged) await writeFile(join(expectedDir, name), bytes);
    payloads.set(name, bytes);
    value.assets.push({
      id: value.assets.length + 1,
      name,
      size: bytes.length,
      digest: `sha256:${hash(bytes)}`,
      browser_download_url: `https://github.com/kdlbs/kandev/releases/download/${tag}/${name}`,
    });
  }
  await add("runtime-size-report.md", "original report\n");
  await add("kandev-browser-demo.tar.gz", "original demo");
  await add(
    "kandev-browser-demo.tar.gz.sha256",
    `${hash("original demo")}  kandev-browser-demo.tar.gz\n`,
  );
  const calls = [];
  const run = async (binary, args) => {
    calls.push([binary, ...args]);
    return { stdout: "" };
  };
  const fetcher = async (url) => {
    const bytes = payloads.get(
      decodeURIComponent(new URL(url).pathname.split("/").at(-1)),
    );
    const response = new Response(bytes, {
      headers: { "content-type": "application/octet-stream" },
    });
    Object.defineProperty(response, "url", {
      value: "https://release-assets.githubusercontent.com/asset",
    });
    return response;
  };
  return {
    root,
    expectedDir,
    outputDir: join(root, "downloaded"),
    release: value,
    payloads,
    add,
    run,
    calls,
    fetcher,
    tag,
    commit,
    compact: false,
  };
}
test("reuse verifies original published bytes rather than nondeterministic rebuilt bytes", async (t) => {
  const f = await fixture(t);
  await writeFile(
    join(f.expectedDir, "kandev-browser-demo.tar.gz"),
    "new build",
  );
  const result = await verifyPublishedAssets(f);
  assert.equal(result.release_id, 1);
  assert.equal(
    await readFile(join(f.outputDir, "kandev-browser-demo.tar.gz"), "utf8"),
    "original demo",
  );
  assert.equal(
    f.calls.some(([binary]) => binary === "gh"),
    false,
  );
});
test("required missing, duplicate, unsafe, empty, and corrupt assets stop reuse", async (t) => {
  for (const mutation of [
    (f) => {
      f.release.assets.pop();
    },
    (f) => {
      f.release.assets.push(f.release.assets[0]);
    },
    (f) => {
      f.release.assets[0].name = "../escape";
    },
    (f) => {
      f.release.assets[0].size = 0;
    },
    (f) => {
      f.payloads.set("kandev-browser-demo.tar.gz", Buffer.from("corrupt"));
    },
    (f) => {
      f.payloads.set(
        "kandev-browser-demo.tar.gz.sha256",
        Buffer.from("invalid checksum"),
      );
      f.release.assets[2].size = 16;
      f.release.assets[2].digest = `sha256:${hash("invalid checksum")}`;
    },
  ]) {
    const f = await fixture(t);
    mutation(f);
    await assert.rejects(
      verifyPublishedAssets(f),
      /asset|checksum|digest|size/,
    );
  }
});
test("new signing inputs do not require an updater absent from the published release", async (t) => {
  const f = await fixture(t);
  await writeFile(
    join(f.expectedDir, "kandev-desktop-macos-arm64-Kandev.app.tar.gz"),
    "new updater",
  );
  await writeFile(
    join(f.expectedDir, "kandev-desktop-macos-arm64-Kandev.app.tar.gz.sig"),
    "new signature",
  );
  await verifyPublishedAssets(f);
  assert.ok(f.calls.some(([, ...args]) => args.includes("--allow-unsigned")));
});
test("compact Windows ZIP must carry the selected release identity", async (t) => {
  const f = await fixture(t);
  await f.add("kandev-windows-x64.zip", "zip payload");
  f.compact = true;
  f.run = async (binary) => ({
    stdout:
      binary === "unzip" ? JSON.stringify({ version: "v0.98.0", commit }) : "",
  });
  await assert.rejects(verifyPublishedAssets(f), /manifest|identity|source/);
});
test("real archive, helper, desktop and unsigned updater validators accept a published compact release", async (t) => {
  const f = await fixture(t);
  const run = promisify(execFile);
  const binDir = join(f.root, "helpers");
  await mkdir(binDir);
  for (const helper of REMOTE_HELPERS)
    await writeFile(join(binDir, helper.name), helper.name, { mode: 0o755 });
  const artifact = join(f.root, "canonical");
  buildRemoteHelperArtifact({
    binDir,
    outputDir: artifact,
    version: tag,
    commit,
    stable: true,
  });
  for (const name of await readdir(join(artifact, "assets"))) {
    await f.add(name, await readFile(join(artifact, "assets", name)));
  }
  const bundle = join(f.root, "kandev");
  await mkdir(join(bundle, "bin"), { recursive: true });
  await writeFile(join(bundle, "bin/kandev"), "launcher");
  await writeFile(join(bundle, "bin/agentctl"), "controller");
  await writeFile(
    join(bundle, "remote-helpers.json"),
    await readFile(join(artifact, "manifests/standard/remote-helpers.json")),
  );
  const archive = join(f.root, "runtime.tar.gz");
  await run("tar", ["-czf", archive, "-C", f.root, "kandev"]);
  const bytes = await readFile(archive);
  await f.add("kandev-linux-x64.tar.gz", bytes);
  await f.add(
    "kandev-linux-x64.tar.gz.sha256",
    `${hash(bytes)}  kandev-linux-x64.tar.gz\n`,
  );
  for (const platform of [
    "macos-arm64",
    "macos-x64",
    "linux-x64",
    "linux-arm64",
    "windows-x64",
  ]) {
    const name = `kandev-desktop-${platform}-installer`;
    await f.add(name, "installer");
    await f.add(`${name}.sha256`, `${hash("installer")}  ${name}\n`);
  }
  f.run = run;
  f.compact = true;
  assert.equal(
    (await verifyPublishedAssets({ ...f, root: process.cwd() })).tag,
    tag,
  );
});
test("download rejects unsuccessful responses, HTML, unsafe redirects, and oversized bodies", async (t) => {
  const f = await fixture(t);
  for (const properties of [
    { status: 404 },
    { type: "text/html" },
    { url: "https://example.com/asset" },
    { body: "oversized body" },
  ]) {
    const asset = {
      ...f.release.assets[0],
      size: 1,
      digest: `sha256:${hash("a")}`,
    };
    const fetcher = async () => {
      const response = new Response(properties.body ?? "a", {
        status: properties.status ?? 200,
        headers: {
          "content-type": properties.type ?? "application/octet-stream",
        },
      });
      Object.defineProperty(response, "url", {
        value:
          properties.url ??
          "https://release-assets.githubusercontent.com/asset",
      });
      return response;
    };
    await assert.rejects(
      downloadReleaseAsset(
        asset,
        join(f.root, `bad-${Math.random()}`),
        fetcher,
      ),
      /download|response|size|redirect/,
    );
  }
});
