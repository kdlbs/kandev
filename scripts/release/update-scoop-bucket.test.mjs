import assert from "node:assert/strict";
import {
  chmod,
  mkdir,
  mkdtemp,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

const sourceRoot = path.resolve(import.meta.dirname, "../..");
const publisherPath = path.join(
  sourceRoot,
  "scripts/release/update-scoop-bucket.sh",
);
const validHash = "0123456789abcdef".repeat(4);

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    ...options,
    encoding: "utf8",
  });
  assert.equal(
    result.status,
    0,
    `${command} ${args.join(" ")} failed:\n${result.stdout}\n${result.stderr}`,
  );
  return result.stdout.trim();
}

async function writeExecutable(file, content) {
  await writeFile(file, content);
  await chmod(file, 0o755);
}

async function createFixture() {
  const root = await mkdtemp(path.join(tmpdir(), "kandev-scoop-"));
  const remote = path.join(root, "bucket.git");
  const seed = path.join(root, "seed");
  const bin = path.join(root, "bin");
  await mkdir(bin, { recursive: true });

  run("git", ["init", "--bare", remote]);
  await mkdir(path.join(seed, "bucket"), { recursive: true });
  run("git", ["init", "--initial-branch=main", seed]);
  run("git", ["-C", seed, "config", "user.email", "fixture@example.test"]);
  run("git", ["-C", seed, "config", "user.name", "Fixture"]);

  const originalManifest = {
    version: "0.85.0",
    description: "Manage tasks and ship value",
    homepage: "https://github.com/kdlbs/kandev",
    license: "AGPL-3.0-only",
    architecture: {
      "64bit": {
        url: "https://github.com/kdlbs/kandev/releases/download/v0.85.0/kandev-windows-x64.tar.gz",
        hash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
        extract_dir: "kandev",
      },
    },
    bin: "bin\\kandev.exe",
    env_set: {
      KANDEV_BUNDLE_DIR: "$dir",
      KANDEV_VERSION: "$version",
    },
    checkver: {
      github: "https://github.com/kdlbs/kandev",
    },
    autoupdate: {
      architecture: {
        "64bit": {
          url: "https://github.com/kdlbs/kandev/releases/download/v$version/kandev-windows-x64.tar.gz",
        },
      },
      hash: {
        url: "$url.sha256",
      },
    },
  };
  await writeFile(
    path.join(seed, "bucket/kandev.json"),
    `${JSON.stringify(originalManifest, null, 4)}\n`,
  );
  run("git", ["-C", seed, "add", "bucket/kandev.json"]);
  run("git", ["-C", seed, "commit", "-m", "fixture"]);
  run("git", ["-C", seed, "remote", "add", "origin", remote]);
  run("git", ["-C", seed, "push", "-u", "origin", "main"]);
  run("git", ["--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/main"]);

  await writeExecutable(
    path.join(bin, "gh"),
    `#!/usr/bin/env bash
set -euo pipefail

if [[ "$1" == "release" && "$2" == "download" ]]; then
  shift 2
  directory=""
  pattern=""
  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      --dir) directory="$2"; shift 2 ;;
      --pattern) pattern="$2"; shift 2 ;;
      --repo) shift 2 ;;
      *) shift ;;
    esac
  done
  mkdir -p "$directory"
  case "$MOCK_CHECKSUM" in
    valid) printf '%s  kandev-windows-x64.tar.gz\\n' "$MOCK_HASH" > "$directory/$pattern" ;;
    malformed) printf '%s  wrong-name.tar.gz\\n' "$MOCK_HASH" > "$directory/$pattern" ;;
    absent) exit 0 ;;
    *) printf 'unexpected checksum mode\\n' >&2; exit 2 ;;
  esac
  exit 0
fi

if [[ "$1" == "repo" && "$2" == "clone" ]]; then
  git clone "$MOCK_BUCKET_REMOTE" "$4"
  exit 0
fi

printf 'unexpected gh command: %s\\n' "$*" >&2
exit 2
`,
  );

  return {
    root,
    remote,
    bin,
    originalManifest,
  };
}

function runPublisher(
  fixture,
  checksumMode = "valid",
  version = "1.2.3",
  tag = "v1.2.3",
) {
  return spawnSync("bash", [publisherPath, version, tag], {
    cwd: sourceRoot,
    encoding: "utf8",
    env: {
      ...process.env,
      GH_TOKEN: "fixture-token",
      MOCK_BUCKET_REMOTE: fixture.remote,
      MOCK_CHECKSUM: checksumMode,
      MOCK_HASH: validHash,
      PATH: `${fixture.bin}:${process.env.PATH}`,
      SCOOP_BUCKET_REPO: "fixture/scoop-kandev",
    },
  });
}

function remoteHead(fixture) {
  return run("git", ["--git-dir", fixture.remote, "rev-parse", "refs/heads/main"]);
}

function remoteManifest(fixture) {
  const manifest = run("git", [
    "--git-dir",
    fixture.remote,
    "show",
    "refs/heads/main:bucket/kandev.json",
  ]);
  return JSON.parse(manifest);
}

test("updates the Scoop manifest and is idempotent", async (t) => {
  const fixture = await createFixture();
  t.after(async () => rm(fixture.root, { recursive: true, force: true }));

  const first = runPublisher(fixture);
  assert.equal(
    first.status,
    0,
    `publisher failed:\n${first.stdout}\n${first.stderr}`,
  );
  const manifest = remoteManifest(fixture);
  assert.equal(manifest.version, "1.2.3");
  assert.equal(
    manifest.architecture["64bit"].url,
    "https://github.com/kdlbs/kandev/releases/download/v1.2.3/kandev-windows-x64.tar.gz",
  );
  assert.equal(manifest.architecture["64bit"].hash, validHash);
  assert.deepEqual(
    {
      description: manifest.description,
      homepage: manifest.homepage,
      license: manifest.license,
      extractDir: manifest.architecture["64bit"].extract_dir,
      bin: manifest.bin,
      envSet: manifest.env_set,
      checkver: manifest.checkver,
      autoupdate: manifest.autoupdate,
    },
    {
      description: fixture.originalManifest.description,
      homepage: fixture.originalManifest.homepage,
      license: fixture.originalManifest.license,
      extractDir: fixture.originalManifest.architecture["64bit"].extract_dir,
      bin: fixture.originalManifest.bin,
      envSet: fixture.originalManifest.env_set,
      checkver: fixture.originalManifest.checkver,
      autoupdate: fixture.originalManifest.autoupdate,
    },
  );
  assert.equal(
    run("git", ["--git-dir", fixture.remote, "log", "-1", "--format=%s", "main"]),
    "kandev 1.2.3",
  );

  const headAfterUpdate = remoteHead(fixture);
  const second = runPublisher(fixture);
  assert.equal(
    second.status,
    0,
    `idempotent rerun failed:\n${second.stdout}\n${second.stderr}`,
  );
  assert.equal(remoteHead(fixture), headAfterUpdate);
  assert.match(second.stdout, /unchanged|nothing to commit/i);
});

test("rejects an absent or malformed checksum before changing the bucket", async (t) => {
  const fixture = await createFixture();
  t.after(async () => rm(fixture.root, { recursive: true, force: true }));

  const originalHead = remoteHead(fixture);
  for (const checksumMode of ["malformed", "absent"]) {
    const result = runPublisher(fixture, checksumMode);
    assert.notEqual(
      result.status,
      0,
      `${checksumMode} checksum unexpectedly passed`,
    );
    assert.match(result.stderr, /checksum/i);
    assert.equal(remoteHead(fixture), originalHead);
  }
});

test("rejects mismatched version and tag before changing the bucket", async (t) => {
  const fixture = await createFixture();
  t.after(async () => rm(fixture.root, { recursive: true, force: true }));

  const originalHead = remoteHead(fixture);
  const result = runPublisher(fixture, "valid", "1.2.3", "v1.2.4");

  assert.notEqual(
    result.status,
    0,
    "mismatched version and tag unexpectedly passed",
  );
  assert.match(result.stderr, /tag must match version/i);
  assert.equal(remoteHead(fixture), originalHead);
});
