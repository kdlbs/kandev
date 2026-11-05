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
    const previous = value.assets.find((asset) => asset.name === name);
    const asset = {
      id: previous?.id ?? value.assets.length + 1,
      name,
      size: bytes.length,
      digest: `sha256:${hash(bytes)}`,
      browser_download_url: `https://github.com/kdlbs/kandev/releases/download/${tag}/${name}`,
    };
    if (previous) Object.assign(previous, asset);
    else value.assets.push(asset);
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
async function runtimeFixture(
  t,
  {
    zip = false,
    compact = true,
    full = false,
    missing = [],
    wrongVersion = false,
  } = {},
) {
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
  const suffix = zip ? ".exe" : "";
  for (const binary of ["kandev", "agentctl"])
    if (!missing.includes(binary))
      await writeFile(join(bundle, `bin/${binary}${suffix}`), binary);
  if (compact) {
    const manifest = JSON.parse(
      await readFile(
        join(
          artifact,
          `manifests/${full ? "full" : "standard"}/remote-helpers.json`,
        ),
      ),
    );
    if (wrongVersion) manifest.version = "v0.98.0";
    await writeFile(
      join(bundle, "remote-helpers.json"),
      JSON.stringify(manifest),
    );
  }
  if (full || !compact) {
    for (const helper of REMOTE_HELPERS)
      await writeFile(join(bundle, "bin", helper.name), helper.name, {
        mode: 0o755,
      });
  }
  const base = zip ? "kandev-windows-x64" : "kandev-linux-x64";
  const name = `${base}${compact && full ? "-full" : ""}.${zip ? "zip" : "tar.gz"}`;
  const archive = join(f.root, "runtime." + (zip ? "zip" : "tar.gz"));
  if (zip)
    await run("python3", ["-m", "zipfile", "-c", archive, "kandev"], {
      cwd: f.root,
    });
  else await run("tar", ["-czf", archive, "-C", f.root, "kandev"]);
  const bytes = await readFile(archive);
  await f.add(name, bytes);
  await f.add(`${name}.sha256`, `${hash(bytes)}  ${name}\n`);
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
  f.compact = compact;
  f.root = process.cwd();
  f.archive = archive;
  f.runtimeName = name;
  await sizeReport(f);
  return f;
}
async function sizeReport(f) {
  const names = [
    f.runtimeName,
    ...REMOTE_HELPERS.map((helper) => helper.asset),
  ].filter(Boolean);
  const rows = f.release.assets
    .filter((asset) => names.includes(asset.name))
    .map((asset) => {
      const helper = asset.name.startsWith("agentctl-");
      const variant = helper
        ? "helper"
        : f.compact && !asset.name.includes("-full.")
          ? "standard"
          : "full";
      const label = helper
        ? `\`${asset.name}\``
        : `${asset.name.match(/^kandev-(.+?)(?:-full)?\.(?:zip|tar.gz)$/)[1]} (\`${asset.name}\`)`;
      return `| ${variant} | ${label} | ${asset.size} | ${(asset.size / 1048576).toFixed(2)} |`;
    });
  await f.add(
    "runtime-size-report.md",
    "# Runtime asset sizes\n\n" + rows.join("\n") + "\n",
  );
}
test("real archive, helper, desktop and unsigned updater validators accept a published compact release", async (t) => {
  const f = await runtimeFixture(t);
  assert.equal(
    (await verifyPublishedAssets({ ...f, root: process.cwd() })).tag,
    tag,
  );
});
test("Windows ZIP verification requires runtime binaries for legacy and compact releases", async (t) => {
  for (const options of [
    { compact: false },
    { compact: true },
    { compact: true, full: true },
  ]) {
    const good = await runtimeFixture(t, { zip: true, ...options });
    await verifyPublishedAssets(good);
    for (const binary of ["kandev", "agentctl"]) {
      const bad = await runtimeFixture(t, {
        zip: true,
        ...options,
        missing: [binary],
      });
      await assert.rejects(verifyPublishedAssets(bad), /missing|runtime/);
    }
  }
});
test("compact Windows ZIP must carry the selected release identity", async (t) => {
  const f = await runtimeFixture(t, { zip: true, wrongVersion: true });
  await assert.rejects(verifyPublishedAssets(f), /manifest|identity|source/);
});
test("Windows ZIP rejects traversal paths and symlinks before extraction", async (t) => {
  for (const link of [false, true]) {
    const f = await runtimeFixture(t, { zip: true });
    await f.run("python3", [
      "-c",
      "import sys,zipfile; z=zipfile.ZipFile(sys.argv[1],'a'); i=zipfile.ZipInfo('kandev/link' if sys.argv[2]=='true' else '../escape'); i.create_system=3; i.external_attr=(0o120777 if sys.argv[2]=='true' else 0o100644)<<16; z.writestr(i,b'outside'); z.close()",
      f.archive,
      String(link),
    ]);
    const bytes = await readFile(f.archive);
    await f.add(f.runtimeName, bytes);
    await f.add(
      `${f.runtimeName}.sha256`,
      `${hash(bytes)}  ${f.runtimeName}\n`,
    );
    await sizeReport(f);
    await assert.rejects(verifyPublishedAssets(f), /ZIP|archive|path|entry/);
  }
});
test("published size report must match the original runtime assets", async (t) => {
  for (const change of [
    (text) => text.replace(/\| \d+ \|/, "| 1 |"),
    (text) =>
      text
        .split("\n")
        .filter((line) => !line.includes("kandev-linux-x64.tar.gz"))
        .join("\n"),
  ]) {
    const f = await runtimeFixture(t);
    await f.add(
      "runtime-size-report.md",
      change(f.payloads.get("runtime-size-report.md").toString()),
    );
    await assert.rejects(verifyPublishedAssets(f), /size report/);
  }
});
test("size report accepts the publisher's rounding at an exact half-cent MiB", async (t) => {
  const f = await fixture(t);
  await f.add("agentctl-linux-amd64.gz", Buffer.alloc(131072, 1));
  await f.add(
    "runtime-size-report.md",
    "| helper | `agentctl-linux-amd64.gz` | 131072 | 0.12 |\n",
  );
  await verifyPublishedAssets(f);
});
test("JSON assets download with verified bytes while binary assets reject JSON responses", async (t) => {
  const f = await fixture(t);
  const bytes = Buffer.from('{"version":"0.99.0"}');
  for (const name of ["latest.json", "plugin-sdk-0.1.0.receipt.json"]) {
    const asset = {
      ...f.release.assets[0],
      name,
      size: bytes.length,
      digest: `sha256:${hash(bytes)}`,
    };
    const fetcher = async () => {
      const response = new Response(bytes, {
        headers: { "content-type": "application/json" },
      });
      Object.defineProperty(response, "url", {
        value: "https://release-assets.githubusercontent.com/asset",
      });
      return response;
    };
    await downloadReleaseAsset(asset, join(f.root, name), fetcher);
    await assert.rejects(
      downloadReleaseAsset(
        { ...asset, name: "runtime.tar.gz" },
        join(f.root, "binary"),
        fetcher,
      ),
      /response/,
    );
  }
});
test("failed downloads remove only their own partial file", async (t) => {
  const f = await fixture(t);
  const destination = join(f.root, "partial");
  for (const asset of [
    { ...f.release.assets[0], digest: `sha256:${"0".repeat(64)}` },
    { ...f.release.assets[0], size: 1 },
  ]) {
    await assert.rejects(
      downloadReleaseAsset(asset, destination, f.fetcher),
      /digest|size/,
    );
    assert.equal((await readdir(f.root)).includes("partial"), false);
  }
  await writeFile(destination, "existing bytes");
  await assert.rejects(
    downloadReleaseAsset(f.release.assets[0], destination, f.fetcher),
    /EEXIST/,
  );
  assert.equal(await readFile(destination, "utf8"), "existing bytes");
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
