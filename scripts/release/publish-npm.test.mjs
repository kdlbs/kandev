import assert from "node:assert/strict";
import {
  chmod,
  copyFile,
  mkdir,
  mkdtemp,
  readFile,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

const sourceRoot = path.resolve(import.meta.dirname, "../..");

async function loadRuntimeInventory() {
  const inventoryPath = path.join(
    sourceRoot,
    "scripts/release/npm-packages.sh",
  );
  const source = await readFile(inventoryPath, "utf8");
  const platformBlock = source.match(/RUNTIME_PLATFORMS=\(([\s\S]*?)\n\)/);
  assert.ok(platformBlock, "runtime platform inventory is missing");
  const platforms = [...platformBlock[1].matchAll(/"([^"]+)"/g)].map(
    (match) => match[1],
  );
  const packageByPlatform = new Map(
    [...source.matchAll(/^\s*\["([^"]+)"\]="([^"]+)"$/gm)].map((match) => [
      match[1],
      match[2],
    ]),
  );
  return platforms.map((platform) => {
    const packageName = packageByPlatform.get(platform);
    assert.ok(packageName, `runtime package is missing for ${platform}`);
    return { platform, packageName };
  });
}

const runtimeInventory = await loadRuntimeInventory();
const runtimePackages = runtimeInventory.map(({ packageName }) => packageName);
const assets = runtimeInventory.map(
  ({ platform }) => `kandev-${platform}.tar.gz`,
);
const releases = [
  { name: "Stable", version: "1.2.3", distTag: "latest" },
  {
    name: "Nightly",
    version: "1.2.4-nightly.shaabcdef123456",
    distTag: "nightly",
  },
];

async function createFixture() {
  const root = await mkdtemp(path.join(tmpdir(), "kandev-publish-npm-"));
  const releaseDir = path.join(root, "scripts/release");
  const cliDir = path.join(root, "apps/cli");
  const assetsDir = path.join(root, "assets");
  const binDir = path.join(root, "bin");
  const publishLog = path.join(root, "publish.log");
  const publishedMetadata = path.join(root, "published-package.json");
  await Promise.all([
    mkdir(releaseDir, { recursive: true }),
    mkdir(cliDir, { recursive: true }),
    mkdir(assetsDir, { recursive: true }),
    mkdir(binDir, { recursive: true }),
  ]);
  await Promise.all(
    ["publish-npm.sh", "npm-packages.sh", "npm-view-version.sh"].map((name) =>
      copyFile(
        path.join(sourceRoot, "scripts/release", name),
        path.join(releaseDir, name),
      ),
    ),
  );
  await Promise.all(
    assets.map((name) => writeFile(path.join(assetsDir, name), "fixture")),
  );
  await Promise.all([
    writeFile(publishLog, ""),
    writeFile(publishedMetadata, ""),
  ]);

  const originalPackageJSON = `${JSON.stringify(
    {
      name: "kandev",
      version: "0.0.0-bootstrap",
      optionalDependencies: Object.fromEntries(
        runtimePackages.map((name) => [name, "0.0.0-bootstrap"]),
      ),
    },
    null,
    2,
  )}\n`;
  await writeFile(path.join(cliDir, "package.json"), originalPackageJSON);

  const runtimePackageLines = runtimePackages
    .map((name) => `${name} ${name.slice("@kdlbs/".length)}`)
    .join("\n");
  await writeExecutable(
    path.join(releaseDir, "package-npm-runtime.sh"),
    `#!/usr/bin/env bash
set -euo pipefail
version="$1"
out="$3"
while read -r package name; do
  dir="$out/@kdlbs/$name"
  mkdir -p "$dir"
  printf '{"name":"%s","version":"%s"}\n' "$package" "$version" > "$dir/package.json"
done <<'PACKAGES'
${runtimePackageLines}
PACKAGES
`,
  );
  await writeExecutable(
    path.join(binDir, "npm"),
    `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  view)
    spec="$2"
    if [[ "$MOCK_NPM_MODE" == "registry-failure" ]]; then
      printf '%s\n' 'npm error code EAI_AGAIN' >&2
      exit 1
    fi
    if [[ "$MOCK_NPM_MODE" == "fresh" || "$MOCK_NPM_MODE" == "runtime-publish-failure" ]]; then
      printf '%s\n' 'npm error code E404' >&2
      exit 1
    fi
    if [[ "$MOCK_NPM_MODE" == main-conflict* && "$spec" == "kandev@$MOCK_NPM_VERSION" ]]; then
      printf '%s\n' 'npm error code E404' >&2
      exit 1
    fi
    if [[ "$MOCK_NPM_MODE" == "main-conflict" && "$spec" == "kandev@nightly" ]]; then
      printf '%s\n' '1.2.3-nightly.sha000000000000'
      exit 0
    fi
    printf '%s\n' "$MOCK_NPM_VERSION"
    ;;
  publish)
    package="$(node -p "require('./package.json').name")"
    printf '%s\n' "$package" >> "$MOCK_NPM_PUBLISH_LOG"
    if [[ "$package" == "kandev" ]]; then
      cp package.json "$MOCK_NPM_PUBLISHED_METADATA"
    fi
    if [[ "$MOCK_NPM_MODE" == "runtime-publish-failure" && "$package" == "$MOCK_RUNTIME_FAILURE_PACKAGE" ]]; then
      printf '%s\n' 'npm error code E500' >&2
      exit 1
    fi
    if [[ "$MOCK_NPM_MODE" == main-conflict* && "$package" == "kandev" ]]; then
      printf '%s\n' 'npm error code EPUBLISHCONFLICT' >&2
      exit 1
    fi
    ;;
  *)
    printf 'unexpected npm command: %s\n' "$*" >&2
    exit 2
    ;;
esac
`,
  );
  await writeExecutable(
    path.join(binDir, "sleep"),
    "#!/usr/bin/env bash\nexit 0\n",
  );
  await writeExecutable(
    path.join(binDir, "gh"),
    `#!/usr/bin/env bash
set -euo pipefail
pattern=""
directory=""
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --pattern) pattern="$2"; shift 2 ;;
    --dir) directory="$2"; shift 2 ;;
    *) shift ;;
  esac
done
mkdir -p "$directory"
: > "$directory/$pattern"
`,
  );

  return {
    root,
    releaseDir,
    cliDir,
    assetsDir,
    binDir,
    publishLog,
    publishedMetadata,
    originalPackageJSON,
  };
}

async function writeExecutable(file, content) {
  await writeFile(file, content);
  await chmod(file, 0o755);
}

async function runPublish(fixture, { version, distTag, mode }) {
  const args = [
    path.join(fixture.releaseDir, "publish-npm.sh"),
    "--version",
    version,
    "--dist-tag",
    distTag,
  ];
  if (distTag === "latest") {
    args.push("--release-tag", `v${version}`);
  } else {
    args.push("--assets-dir", fixture.assetsDir);
  }
  const result = spawnSync("bash", args, {
    encoding: "utf8",
    cwd: fixture.root,
    env: {
      ...process.env,
      MOCK_NPM_MODE: mode,
      MOCK_NPM_PUBLISH_LOG: fixture.publishLog,
      MOCK_NPM_PUBLISHED_METADATA: fixture.publishedMetadata,
      MOCK_RUNTIME_FAILURE_PACKAGE: runtimePackages[1],
      MOCK_NPM_VERSION: version,
      PATH: `${fixture.binDir}${path.delimiter}${process.env.PATH ?? ""}`,
    },
  });
  return {
    ...result,
    packageJSON: await readFile(
      path.join(fixture.cliDir, "package.json"),
      "utf8",
    ),
    publishedMetadata: await readFile(fixture.publishedMetadata, "utf8"),
    published: (await readFile(fixture.publishLog, "utf8"))
      .trim()
      .split("\n")
      .filter(Boolean),
  };
}

for (const release of releases) {
  test(`${release.name} publication is idempotent when every exact package already exists`, async () => {
    const fixture = await createFixture();
    try {
      const result = await runPublish(fixture, {
        ...release,
        mode: "all-published",
      });
      assert.equal(result.status, 0, result.stderr);
      assert.deepEqual(result.published, []);
      assert.equal(result.packageJSON, fixture.originalPackageJSON);
    } finally {
      await rm(fixture.root, { recursive: true, force: true });
    }
  });
}

for (const release of releases) {
  test(`a fresh ${release.name} publishes every package with pinned launcher metadata`, async () => {
    const fixture = await createFixture();
    try {
      const result = await runPublish(fixture, { ...release, mode: "fresh" });
      assert.equal(result.status, 0, result.stderr);
      assert.deepEqual(result.published, [...runtimePackages, "kandev"]);
      const metadata = JSON.parse(result.publishedMetadata);
      assert.equal(metadata.version, release.version);
      assert.deepEqual(
        metadata.optionalDependencies,
        Object.fromEntries(
          runtimePackages.map((name) => [name, release.version]),
        ),
      );
      assert.equal(result.packageJSON, fixture.originalPackageJSON);
    } finally {
      await rm(fixture.root, { recursive: true, force: true });
    }
  });
}

test("a conflicting Nightly launcher publish fails closed and restores package metadata", async () => {
  const fixture = await createFixture();
  try {
    const result = await runPublish(fixture, {
      version: "1.2.4-nightly.shaabcdef123456",
      distTag: "nightly",
      mode: "main-conflict",
    });
    assert.notEqual(result.status, 0);
    assert.match(
      result.stderr,
      /kandev@nightly resolves to '1\.2\.3-nightly\.sha000000000000'/,
    );
    assert.deepEqual(result.published, ["kandev"]);
    assert.equal(result.packageJSON, fixture.originalPackageJSON);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("a matching Nightly conflict converges as an idempotent success", async () => {
  const fixture = await createFixture();
  try {
    const result = await runPublish(fixture, {
      version: "1.2.4-nightly.shaabcdef123456",
      distTag: "nightly",
      mode: "main-conflict-matching",
    });
    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(result.published, ["kandev"]);
    assert.equal(result.packageJSON, fixture.originalPackageJSON);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("a transient registry failure aborts before publishing and preserves metadata", async () => {
  const fixture = await createFixture();
  try {
    const result = await runPublish(fixture, {
      version: "1.2.4-nightly.shaabcdef123456",
      distTag: "nightly",
      mode: "registry-failure",
    });
    assert.notEqual(result.status, 0);
    assert.match(
      result.stderr,
      /could not verify whether @kdlbs\/runtime-linux-x64/,
    );
    assert.deepEqual(result.published, []);
    assert.equal(result.packageJSON, fixture.originalPackageJSON);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("a runtime publish failure withholds the launcher", async () => {
  const fixture = await createFixture();
  try {
    const result = await runPublish(fixture, {
      version: "1.2.4-nightly.shaabcdef123456",
      distTag: "nightly",
      mode: "runtime-publish-failure",
    });
    assert.notEqual(result.status, 0);
    assert.deepEqual(result.published, runtimePackages);
    assert.ok(
      result.stderr.includes(`Failed to publish ${runtimePackages[1]}`),
    );
    assert.match(result.stderr, /Refusing to publish main kandev/);
    assert.equal(result.packageJSON, fixture.originalPackageJSON);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});
