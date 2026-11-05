#!/usr/bin/env node
import { execFile } from "node:child_process";
import { promisify, parseArgs } from "node:util";
import { createHash } from "node:crypto";
import { createReadStream, createWriteStream } from "node:fs";
import {
  mkdir,
  readdir,
  readFile,
  writeFile,
  appendFile,
  mkdtemp,
  rm,
} from "node:fs/promises";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { Readable, Transform } from "node:stream";
import { pipeline } from "node:stream/promises";
import { pathToFileURL } from "node:url";
import {
  parseManifest,
  verifyRemoteHelperBundle,
} from "./remote-helper-assets.mjs";
import { gunzipSync } from "node:zlib";
import { verifiedFingerprint } from "./sdk-artifact.mjs";

const execute = promisify(execFile);
const repository = "kdlbs/kandev";
const optionalUpdater = (name) =>
  name === "latest.json" ||
  (name.startsWith("kandev-desktop-") &&
    /(?:\.sig(?:\.sha256)?|\.(?:app\.tar\.gz|AppImage\.tar\.gz|nsis\.zip)(?:\.sha256)?)$/.test(
      name,
    ));

function validateRelease(release, tag) {
  if (
    !release ||
    !Number.isSafeInteger(release.id) ||
    release.id <= 0 ||
    release.tag_name !== tag ||
    typeof release.draft !== "boolean" ||
    typeof release.immutable !== "boolean" ||
    release.prerelease !== false ||
    !Array.isArray(release.assets)
  ) {
    throw new Error("invalid existing release identity or metadata");
  }
}
async function verifySource({ tag, commit, root, run }) {
  const options = { cwd: root, encoding: "utf8" };
  const peeled = (
    await run("git", ["rev-parse", `${tag}^{commit}`], options)
  ).stdout.trim();
  if (peeled !== commit)
    throw new Error("existing release signed source mismatch");
  const key = join(root, ".github/release-signing-key.asc");
  const info = (
    await run("gpg", ["--batch", "--with-colons", "--show-keys", key], options)
  ).stdout;
  if (info.split("\n").filter((line) => line.startsWith("pub:")).length !== 1)
    throw new Error("invalid release public key");
  const fingerprint = info
    .split("\n")
    .find((line) => line.startsWith("fpr:"))
    ?.split(":")[9];
  if (!/^[A-F0-9]{40}$/.test(fingerprint ?? ""))
    throw new Error("invalid release key fingerprint");
  await run("gpg", ["--batch", "--import", key], options);
  const signature = await run("git", ["verify-tag", "--raw", tag], options);
  verifiedFingerprint(signature.stderr, fingerprint);
}
export async function inspectPublishedRelease({
  tag,
  commit,
  root,
  run = execute,
}) {
  if (!/^v\d+\.\d+\.\d+$/.test(tag) || !/^[a-f0-9]{40}$/.test(commit))
    throw new Error("invalid release source");
  let response;
  try {
    response = await run(
      "gh",
      ["api", `repos/${repository}/releases/tags/${encodeURIComponent(tag)}`],
      { cwd: root, encoding: "utf8" },
    );
  } catch (error) {
    if (/\bHTTP 404\b/.test(error.stderr ?? error.message))
      return { reuse: false, release: null, tag, commit };
    throw error;
  }
  const release = JSON.parse(response.stdout);
  validateRelease(release, tag);
  const reuse = release.immutable && !release.draft;
  if (reuse) await verifySource({ tag, commit, root, run });
  return { reuse, release, tag, commit };
}
export async function downloadReleaseAsset(
  asset,
  destination,
  fetcher = fetch,
) {
  const initial = new URL(asset.browser_download_url);
  if (
    initial.origin !== "https://github.com" ||
    !initial.pathname.startsWith(`/${repository}/releases/download/`)
  )
    throw new Error("invalid release download URL");
  const response = await fetcher(initial, {
    signal: AbortSignal.timeout(120_000),
  });
  const final = new URL(response.url);
  if (!response.ok || !response.body)
    throw new Error(
      `asset download failed: ${asset.name} (HTTP ${response.status})`,
    );
  if (
    final.protocol !== "https:" ||
    ![
      "github.com",
      "release-assets.githubusercontent.com",
      "objects.githubusercontent.com",
    ].includes(final.hostname)
  )
    throw new Error("unsafe asset download redirect");
  if (
    /text\/html|application\/(?:[^;]+\+)?json/i.test(
      response.headers.get("content-type") ?? "",
    )
  )
    throw new Error("invalid asset download response type");
  const digest = createHash("sha256");
  let size = 0;
  const meter = new Transform({
    transform(chunk, encoding, callback) {
      size += chunk.length;
      if (size > asset.size)
        return callback(new Error(`asset size mismatch: ${asset.name}`));
      digest.update(chunk);
      callback(null, chunk);
    },
  });
  await pipeline(
    Readable.fromWeb(response.body),
    meter,
    createWriteStream(destination, { flags: "wx" }),
  );
  if (size !== asset.size || `sha256:${digest.digest("hex")}` !== asset.digest)
    throw new Error(`asset digest or size mismatch: ${asset.name}`);
}
function assetIndex(release) {
  const assets = new Map();
  for (const asset of release.assets) {
    if (
      !asset ||
      !/^[A-Za-z0-9][A-Za-z0-9._-]*$/.test(asset.name ?? "") ||
      !Number.isSafeInteger(asset.id) ||
      asset.id <= 0 ||
      !Number.isSafeInteger(asset.size) ||
      asset.size <= 0 ||
      !/^sha256:[a-f0-9]{64}$/.test(asset.digest ?? "") ||
      assets.has(asset.name)
    )
      throw new Error("invalid or duplicate release asset");
    assets.set(asset.name, asset);
  }
  return assets;
}
async function fileDigest(file) {
  const digest = createHash("sha256");
  for await (const chunk of createReadStream(file)) digest.update(chunk);
  return digest.digest("hex");
}
async function verifyChecksums(directory, assets) {
  for (const name of assets.keys()) {
    if (!name.endsWith(".sha256")) continue;
    const target = name.slice(0, -7);
    const checksum = await readFile(join(directory, name), "utf8");
    const match = /^([a-f0-9]{64}) [ *]([^\r\n]+)\r?\n?$/.exec(checksum);
    if (
      !assets.has(target) ||
      !match ||
      match[2] !== target ||
      (await fileDigest(join(directory, target))) !== match[1]
    )
      throw new Error(`invalid asset checksum: ${name}`);
  }
}
async function verifyRuntimeManifest(manifest, name, directory, tag, commit) {
  const records = parseManifest(
    manifest,
    /-full\.(?:tar\.gz|zip)$/.test(name) ? "full" : "standard",
    { version: tag, commit },
  );
  for (const record of records.values()) {
    const bytes = gunzipSync(await readFile(join(directory, record.asset)), {
      maxOutputLength: 256 * 1024 * 1024,
    });
    if (
      bytes.length !== record.size_bytes ||
      createHash("sha256").update(bytes).digest("hex") !== record.sha256
    )
      throw new Error(`canonical helper identity mismatch: ${record.asset}`);
  }
}
async function verifyRuntimeArchives({
  directory,
  assets,
  compact,
  tag,
  commit,
  run,
  root,
}) {
  for (const name of assets.keys()) {
    if (
      !name.startsWith("kandev-") ||
      name.startsWith("kandev-desktop-") ||
      name.startsWith("kandev-browser-demo") ||
      !(name.endsWith(".tar.gz") || name.endsWith(".zip"))
    )
      continue;
    const archive = join(directory, name);
    const options = { cwd: root, encoding: "utf8", maxBuffer: 4 * 1024 * 1024 };
    if (name.endsWith(".zip")) {
      if (compact) {
        const manifest = JSON.parse(
          (
            await run(
              "unzip",
              ["-p", archive, "kandev/remote-helpers.json"],
              options,
            )
          ).stdout,
        );
        await verifyRuntimeManifest(manifest, name, directory, tag, commit);
      }
      continue;
    }
    const list = (await run("tar", ["-tzf", archive], options)).stdout
      .trim()
      .split("\n");
    if (
      list.some(
        (entry) =>
          !entry.startsWith("kandev/") || entry.split("/").includes(".."),
      )
    )
      throw new Error(`invalid runtime archive paths: ${name}`);
    const detail = (await run("tar", ["-tvzf", archive], options)).stdout;
    if (
      detail
        .split("\n")
        .filter(Boolean)
        .some((line) => !/^[d-]/.test(line))
    )
      throw new Error(`unsupported runtime archive entry: ${name}`);
    const temporary = await mkdtemp(join(tmpdir(), "verify-runtime-"));
    try {
      await run(
        "tar",
        [
          "-xzf",
          archive,
          "-C",
          temporary,
          "--no-same-owner",
          "--no-same-permissions",
        ],
        options,
      );
      const bundleDir = join(temporary, "kandev");
      verifyRemoteHelperBundle({
        bundleDir,
        variant:
          compact && !name.endsWith("-full.tar.gz") ? "standard" : "full",
      });
      if (compact) {
        const manifest = JSON.parse(
          await readFile(join(bundleDir, "remote-helpers.json"), "utf8"),
        );
        await verifyRuntimeManifest(manifest, name, directory, tag, commit);
      }
    } finally {
      await rm(temporary, { recursive: true, force: true });
    }
  }
}
export async function verifyPublishedAssets({
  release,
  expectedDir,
  outputDir,
  tag,
  commit,
  compact,
  root = process.cwd(),
  run = execute,
  fetcher = fetch,
}) {
  validateRelease(release, tag);
  if (release.draft || !release.immutable)
    throw new Error("release is not published immutable");
  const assets = assetIndex(release);
  const expected = (await readdir(expectedDir, { withFileTypes: true }))
    .filter((entry) => entry.isFile() && !optionalUpdater(entry.name))
    .map((entry) => entry.name);
  if (!expected.length)
    throw new Error("required release asset inventory is empty");
  for (const name of expected)
    if (!assets.has(name))
      throw new Error(`missing required release asset: ${name}`);
  await mkdir(outputDir, { recursive: true });
  if ((await readdir(outputDir)).length)
    throw new Error("existing asset download directory is not empty");
  for (const asset of assets.values())
    await downloadReleaseAsset(asset, join(outputDir, asset.name), fetcher);
  await verifyChecksums(outputDir, assets);
  await verifyRuntimeArchives({
    directory: outputDir,
    assets,
    compact,
    tag,
    commit,
    run,
    root,
  });
  const options = { cwd: root, encoding: "utf8", maxBuffer: 4 * 1024 * 1024 };
  await run(
    "bash",
    [join(root, "scripts/release/verify-desktop-assets.sh"), outputDir],
    options,
  );
  const manifest = assets.has("latest.json");
  const args = [
    join(root, "scripts/release/updater-manifest.mjs"),
    manifest ? "verify" : "generate",
    "--assets-dir",
    outputDir,
    manifest ? "--manifest" : "--output",
    join(outputDir, manifest ? "latest.json" : "unsigned-check.json"),
    "--version",
    tag.slice(1),
    "--tag",
    tag,
    "--repository",
    repository,
  ];
  if (!manifest)
    args.push(
      "--allow-unsigned",
      "--notes-file",
      "/dev/null",
      "--pub-date",
      "2000-01-01T00:00:00Z",
    );
  await run("node", args, options);
  if (!manifest && (await readdir(outputDir)).includes("unsigned-check.json"))
    throw new Error("published signed updater assets are missing latest.json");
  return {
    release_id: release.id,
    tag,
    commit,
    assets: [...assets.values()].map(({ name, digest }) => ({ name, digest })),
  };
}

async function main() {
  const { positionals, values } = parseArgs({
    allowPositionals: true,
    options: Object.fromEntries(
      [
        "tag",
        "commit",
        "output-json",
        "release-json",
        "assets-dir",
        "output-dir",
        "compact",
      ].map((name) => [name, { type: "string" }]),
    ),
  });
  if (positionals[0] === "inspect") {
    const result = await inspectPublishedRelease({
      tag: values.tag,
      commit: values.commit,
      root: process.cwd(),
    });
    await writeFile(values["output-json"], JSON.stringify(result));
    if (process.env.GITHUB_OUTPUT)
      await appendFile(process.env.GITHUB_OUTPUT, `reuse=${result.reuse}\n`);
    console.log(
      JSON.stringify({ reuse: result.reuse, release_id: result.release?.id }),
    );
  } else if (positionals[0] === "verify") {
    const { release, tag, commit, reuse } = JSON.parse(
      await readFile(values["release-json"], "utf8"),
    );
    if (!reuse || !["true", "false"].includes(values.compact))
      throw new Error("invalid immutable release verification context");
    console.log(
      JSON.stringify(
        await verifyPublishedAssets({
          release,
          tag,
          commit,
          compact: values.compact === "true",
          expectedDir: values["assets-dir"],
          outputDir: values["output-dir"],
        }),
      ),
    );
  } else throw new Error("expected inspect or verify");
}
if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
)
  await main();
