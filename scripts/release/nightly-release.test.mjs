import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
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
import test from "node:test";

const sourceRoot = path.resolve(import.meta.dirname, "../..");
const packages = [
  "kandev",
  "@kdlbs/runtime-linux-x64",
  "@kdlbs/runtime-linux-arm64",
  "@kdlbs/runtime-darwin-x64",
  "@kdlbs/runtime-darwin-arm64",
  "@kdlbs/runtime-win32-x64",
];

async function writeExecutable(file, content) {
  await writeFile(file, content);
  await chmod(file, 0o755);
}

function git(root, ...args) {
  return execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
}

async function createFixture() {
  const root = await mkdtemp(path.join(tmpdir(), "kandev-nightly-release-"));
  const releaseDir = path.join(root, "scripts/release");
  const binDir = path.join(root, "bin");
  const assetsDir = path.join(root, "assets");
  const stateFile = path.join(root, "npm-state.tsv");
  const outputFile = path.join(root, "github-output");
  const publishLog = path.join(root, "publish.log");
  await Promise.all([
    mkdir(releaseDir, { recursive: true }),
    mkdir(binDir, { recursive: true }),
    mkdir(assetsDir, { recursive: true }),
    writeFile(stateFile, ""),
    writeFile(outputFile, ""),
    writeFile(publishLog, ""),
  ]);
  await Promise.all(
    [
      "nightly-release.sh",
      "nightly-version.mjs",
      "npm-packages.sh",
      "npm-view-version.sh",
    ].map((name) =>
      copyFile(
        path.join(sourceRoot, "scripts/release", name),
        path.join(releaseDir, name),
      ),
    ),
  );
  await writeExecutable(
    path.join(releaseDir, "publish-npm.sh"),
    `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" >> "$MOCK_PUBLISH_LOG"
`,
  );
  await writeExecutable(
    path.join(binDir, "npm"),
    `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" != "view" || "$3" != "version" ]]; then
  printf 'unexpected npm command: %s\n' "$*" >&2
  exit 2
fi
spec="$2"
while IFS=$'\t' read -r key value; do
  if [[ "$key" != "$spec" ]]; then
    continue
  fi
  if [[ "$value" == "!ERROR!" ]]; then
    printf '%s\n' 'npm error code EAI_AGAIN' >&2
    exit 1
  fi
  printf '%s\n' "$value"
  exit 0
done < "$MOCK_NPM_STATE"
printf '%s\n' 'npm error code E404' >&2
exit 1
`,
  );
  await writeExecutable(
    path.join(binDir, "sleep"),
    "#!/usr/bin/env bash\nexit 0\n",
  );

  git(root, "init", "-q");
  git(root, "config", "user.name", "Nightly Test");
  git(root, "config", "user.email", "nightly@example.test");
  await writeFile(path.join(root, "history.txt"), "stable\n");
  git(root, "add", "history.txt");
  git(root, "commit", "-q", "-m", "stable");
  const stableSha = git(root, "rev-parse", "HEAD");
  git(root, "tag", "v1.2.3");
  await writeFile(path.join(root, "history.txt"), "stable\nmain\n");
  git(root, "add", "history.txt");
  git(root, "commit", "-q", "-m", "main");
  const mainSha = git(root, "rev-parse", "HEAD");
  await writeFile(path.join(root, "history.txt"), "stable\nmain\nnewer\n");
  git(root, "add", "history.txt");
  git(root, "commit", "-q", "-m", "newer");
  const newerSha = git(root, "rev-parse", "HEAD");
  git(root, "checkout", "-q", "--detach", stableSha);
  await writeFile(path.join(root, "history.txt"), "stable\ndivergent\n");
  git(root, "add", "history.txt");
  git(root, "commit", "-q", "-m", "divergent");
  const divergentSha = git(root, "rev-parse", "HEAD");
  git(root, "checkout", "-q", "--detach", mainSha);

  return {
    root,
    releaseDir,
    binDir,
    assetsDir,
    stateFile,
    outputFile,
    publishLog,
    stableSha,
    mainSha,
    newerSha,
    divergentSha,
    version: `1.2.4-nightly.sha${mainSha.slice(0, 12)}`,
  };
}

async function setRegistry(fixture, entries) {
  const lines = [...entries].map(([spec, value]) => `${spec}\t${value}`);
  await writeFile(
    fixture.stateFile,
    lines.length === 0 ? "" : `${lines.join("\n")}\n`,
  );
}

function runScript(fixture, ...args) {
  return spawnSync(
    "bash",
    [path.join(fixture.releaseDir, "nightly-release.sh"), ...args],
    {
      cwd: fixture.root,
      encoding: "utf8",
      env: {
        ...process.env,
        MOCK_NPM_STATE: fixture.stateFile,
        MOCK_PUBLISH_LOG: fixture.publishLog,
        PATH: `${fixture.binDir}${path.delimiter}${process.env.PATH ?? ""}`,
      },
    },
  );
}

async function runPrepare(fixture, scheduledSha = fixture.mainSha) {
  await writeFile(fixture.outputFile, "");
  const result = runScript(
    fixture,
    "prepare",
    "--scheduled-sha",
    scheduledSha,
    "--output",
    fixture.outputFile,
  );
  const output = Object.fromEntries(
    (await readFile(fixture.outputFile, "utf8"))
      .trim()
      .split("\n")
      .filter(Boolean)
      .map((line) => {
        const separator = line.indexOf("=");
        return [line.slice(0, separator), line.slice(separator + 1)];
      }),
  );
  return { ...result, output };
}

function registryFor(version, nightly = "") {
  const entries = new Map([["kandev@latest", "1.2.3"]]);
  if (nightly) entries.set("kandev@nightly", nightly);
  for (const packageName of packages) {
    entries.set(`${packageName}@${version}`, version);
    entries.set(`${packageName}@nightly`, version);
  }
  return entries;
}

function tagsSnapshot(version) {
  return packages.map((packageName) => `${packageName}=${version};`).join("");
}

test("prepare publishes a changed main commit and snapshots empty tags", async () => {
  const fixture = await createFixture();
  try {
    await setRegistry(fixture, new Map([["kandev@latest", "1.2.3"]]));
    const result = await runPrepare(fixture);

    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.output.should_publish, "true");
    assert.equal(result.output.stable_version, "1.2.3");
    assert.equal(result.output.version, fixture.version);
    assert.equal(result.output.tag, `v${fixture.version}`);
    assert.equal(result.output.ref, fixture.mainSha);
    assert.equal(result.output.nightly_version_at_start, "");
    assert.equal(result.output.nightly_tags_at_start, tagsSnapshot(""));
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("prepare normalizes a legacy two-part stable tag", async () => {
  const fixture = await createFixture();
  try {
    git(fixture.root, "tag", "-d", "v1.2.3");
    git(fixture.root, "tag", "v1.2", fixture.stableSha);
    await setRegistry(fixture, new Map([["kandev@latest", "1.2.0"]]));
    const result = await runPrepare(fixture);

    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.output.should_publish, "true");
    assert.equal(result.output.stable_version, "1.2.0");
    assert.equal(
      result.output.version,
      `1.2.1-nightly.sha${fixture.mainSha.slice(0, 12)}`,
    );
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("prepare skips when main has no commit after stable", async () => {
  const fixture = await createFixture();
  try {
    git(fixture.root, "checkout", "-q", "--detach", fixture.stableSha);
    await setRegistry(fixture, new Map([["kandev@latest", "1.2.3"]]));
    const result = await runPrepare(fixture, fixture.stableSha);

    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.output.should_publish, "false");
    assert.match(result.stdout, /No commits on main since stable/);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("prepare skips while a stable Git tag is ahead of npm latest", async () => {
  const fixture = await createFixture();
  try {
    git(fixture.root, "tag", "v1.3.0", fixture.mainSha);
    await setRegistry(fixture, new Map([["kandev@latest", "1.2.3"]]));
    const result = await runPrepare(fixture);

    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.output.should_publish, "false");
    assert.match(result.stdout, /stable Git tag v1\.3\.0.*npm @latest 1\.2\.3/);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("prepare skips a scheduled commit superseded by the current stable", async () => {
  const fixture = await createFixture();
  try {
    git(fixture.root, "tag", "v1.3.0", fixture.newerSha);
    await setRegistry(fixture, new Map([["kandev@latest", "1.3.0"]]));
    const result = await runPrepare(fixture);

    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.output.should_publish, "false");
    assert.match(result.stdout, /superseded by stable v1\.3\.0/);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("prepare skips a complete publication and repairs a partial one", async () => {
  const fixture = await createFixture();
  try {
    const complete = registryFor(fixture.version, fixture.version);
    await setRegistry(fixture, complete);
    const completeResult = await runPrepare(fixture);
    assert.equal(completeResult.status, 0, completeResult.stderr);
    assert.equal(completeResult.output.should_publish, "false");

    complete.delete(`${packages.at(-1)}@${fixture.version}`);
    await setRegistry(fixture, complete);
    const partialResult = await runPrepare(fixture);
    assert.equal(partialResult.status, 0, partialResult.stderr);
    assert.equal(partialResult.output.should_publish, "true");
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("prepare skips a scheduled commit superseded by a newer main Nightly", async () => {
  const fixture = await createFixture();
  try {
    const newerVersion = `1.2.4-nightly.sha${fixture.newerSha.slice(0, 12)}`;
    await setRegistry(
      fixture,
      new Map([
        ["kandev@latest", "1.2.3"],
        ["kandev@nightly", newerVersion],
      ]),
    );
    const result = await runPrepare(fixture);

    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.output.should_publish, "false");
    assert.match(result.stdout, /newer main commit is already published/);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("prepare rejects a Nightly version that cannot identify a commit", async () => {
  const fixture = await createFixture();
  try {
    await setRegistry(
      fixture,
      new Map([
        ["kandev@latest", "1.2.3"],
        ["kandev@nightly", "1.2.4-nightly.bad"],
      ]),
    );
    const result = await runPrepare(fixture);

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /unsupported version/);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("prepare rejects registry commits outside scheduled main history", async () => {
  const cases = [
    {
      name: "stable baseline",
      configure(fixture, registry) {
        git(fixture.root, "tag", "-f", "v1.2.3", fixture.divergentSha);
        return registry;
      },
      error: /Latest stable .* is not an ancestor/,
    },
    {
      name: "top-level Nightly",
      configure(fixture, registry) {
        registry.set(
          "kandev@nightly",
          `1.2.4-nightly.sha${fixture.divergentSha.slice(0, 12)}`,
        );
        return registry;
      },
      error: /Published Nightly commit .* is not an ancestor/,
    },
    {
      name: "partial runtime tag",
      configure(fixture, registry) {
        const packageName = packages.at(-1);
        registry.set("kandev@nightly", fixture.version);
        registry.delete(`${packageName}@${fixture.version}`);
        registry.set(
          `${packageName}@nightly`,
          `1.2.4-nightly.sha${fixture.divergentSha.slice(0, 12)}`,
        );
        return registry;
      },
      error: /@nightly commit .* is not an ancestor/,
    },
  ];

  for (const scenario of cases) {
    const fixture = await createFixture();
    try {
      const registry = scenario.configure(
        fixture,
        registryFor(fixture.version, fixture.version),
      );
      await setRegistry(fixture, registry);
      const result = await runPrepare(fixture);

      assert.notEqual(result.status, 0, scenario.name);
      assert.match(result.stderr, scenario.error, scenario.name);
    } finally {
      await rm(fixture.root, { recursive: true, force: true });
    }
  }
});

test("prepare rejects an exact package whose Nightly tag points elsewhere", async () => {
  const fixture = await createFixture();
  try {
    const registry = registryFor(fixture.version, fixture.version);
    registry.set(
      `${packages.at(-1)}@nightly`,
      `1.2.4-nightly.sha${fixture.stableSha.slice(0, 12)}`,
    );
    await setRegistry(fixture, registry);
    const result = await runPrepare(fixture);

    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /exists but @nightly resolves/);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("prepare safely skips registry failures", async () => {
  const fixture = await createFixture();
  try {
    await setRegistry(fixture, new Map([["kandev@latest", "!ERROR!"]]));
    const result = await runPrepare(fixture);

    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.output.should_publish, "false");
    assert.match(result.stdout, /Could not resolve npm @latest/);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("prepare safely skips a package registry failure", async () => {
  const fixture = await createFixture();
  try {
    const registry = new Map([
      ["kandev@latest", "1.2.3"],
      [`${packages[1]}@${fixture.version}`, "!ERROR!"],
    ]);
    await setRegistry(fixture, registry);
    const result = await runPrepare(fixture);

    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.output.should_publish, "false");
    assert.match(result.stdout, /Could not verify/);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("publish revalidates the registry snapshot before delegating", async () => {
  const fixture = await createFixture();
  try {
    await setRegistry(fixture, registryFor(fixture.version, fixture.version));
    const result = runScript(
      fixture,
      "publish",
      "--stable-at-start",
      "1.2.3",
      "--nightly-at-start",
      fixture.version,
      "--tags-at-start",
      tagsSnapshot(fixture.version),
      "--version",
      fixture.version,
      "--assets-dir",
      fixture.assetsDir,
    );

    assert.equal(result.status, 0, result.stderr);
    assert.deepEqual(
      (await readFile(fixture.publishLog, "utf8")).trim().split("\n"),
      [
        "--version",
        fixture.version,
        "--dist-tag",
        "nightly",
        "--assets-dir",
        fixture.assetsDir,
      ],
    );
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});

test("publish skips when any locked registry value moves", async () => {
  const cases = [
    ["stable baseline", "kandev@latest", "1.2.4"],
    ["top-level Nightly", "kandev@nightly", "1.2.4-nightly.sha000000000000"],
    ["runtime tag", `${packages[1]}@nightly`, "1.2.4-nightly.sha000000000000"],
  ];
  for (const [name, spec, movedValue] of cases) {
    const fixture = await createFixture();
    try {
      const registry = registryFor(fixture.version, fixture.version);
      registry.set(spec, movedValue);
      await setRegistry(fixture, registry);
      const result = runScript(
        fixture,
        "publish",
        "--stable-at-start",
        "1.2.3",
        "--nightly-at-start",
        fixture.version,
        "--tags-at-start",
        tagsSnapshot(fixture.version),
        "--version",
        fixture.version,
        "--assets-dir",
        fixture.assetsDir,
      );

      assert.equal(result.status, 0, `${name}: ${result.stderr}`);
      assert.equal(await readFile(fixture.publishLog, "utf8"), "", name);
    } finally {
      await rm(fixture.root, { recursive: true, force: true });
    }
  }
});

test("publish skips while a stable Git tag is ahead of npm latest", async () => {
  const fixture = await createFixture();
  try {
    git(fixture.root, "tag", "v1.3.0", fixture.mainSha);
    await setRegistry(fixture, registryFor(fixture.version, fixture.version));
    const result = runScript(
      fixture,
      "publish",
      "--stable-at-start",
      "1.2.3",
      "--nightly-at-start",
      fixture.version,
      "--tags-at-start",
      tagsSnapshot(fixture.version),
      "--version",
      fixture.version,
      "--assets-dir",
      fixture.assetsDir,
    );

    assert.equal(result.status, 0, result.stderr);
    assert.equal(await readFile(fixture.publishLog, "utf8"), "");
    assert.match(result.stdout, /stable Git tag v1\.3\.0.*npm @latest 1\.2\.3/);
  } finally {
    await rm(fixture.root, { recursive: true, force: true });
  }
});
