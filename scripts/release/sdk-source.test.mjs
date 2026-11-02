import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { SourceRepository } from "./sdk-source.mjs";

test("SDK tag push uses command-scoped GitHub credentials", async () => {
  const repository = new SourceRepository({
    root: ".",
    fingerprint: "fixture",
  });
  repository.readSDKTag = async () => null;
  repository.text = async () => {
    throw Object.assign(new Error("absent"), { code: 128 });
  };
  const calls = [];
  repository.git = async (...args) => {
    calls.push(args);
    return { stdout: "", stderr: "" };
  };
  await repository.createSDKTag({
    record: { sdk_version: "0.1.0" },
    sdkTag: "pluginsdk/v0.1.0",
    commit: "fixture-commit",
  });
  assert.deepEqual(calls.at(-1), [
    "-c",
    "credential.helper=",
    "-c",
    "credential.helper=!gh auth git-credential",
    "push",
    "origin",
    "refs/tags/pluginsdk/v0.1.0",
  ]);
});

// @covers AC-PLUGINS-SDK-DISTRIBUTION-003.2
test("verifies real signed host and SDK tags and resolves reuse to the original commit", async () => {
  const root = await mkdtemp(join(tmpdir(), "sdk-signed-source-"));
  try {
    const home = join(root, "keyring");
    await mkdir(home, { mode: 0o700 });
    const env = { ...process.env, GNUPGHOME: home };
    const exec = (file, args, cwd = root) =>
      execFileSync(file, args, {
        cwd,
        env,
        encoding: "utf8",
        stdio: ["ignore", "pipe", "pipe"],
      }).trim();
    exec("gpg", [
      "--batch",
      "--passphrase",
      "",
      "--quick-generate-key",
      "SDK fixture <sdk@example.invalid>",
      "ed25519",
      "sign",
      "0",
    ]);
    const fingerprint = exec("gpg", ["--batch", "--with-colons", "--list-keys"])
      .split("\n")
      .find((line) => line.startsWith("fpr:"))
      .split(":")[9];
    const remote = join(root, "remote");
    exec("git", ["init", "--bare", "--quiet", remote]);
    const repo = join(root, "repo");
    await mkdir(repo);
    exec("git", ["init", "--quiet"], repo);
    const git = (...args) => exec("git", args, repo);
    git("config", "user.name", "SDK fixture");
    git("config", "user.email", "sdk@example.invalid");
    git("config", "user.signingkey", fingerprint);
    git("remote", "add", "origin", remote);
    await mkdir(join(repo, "pluginsdk"));
    await mkdir(join(repo, "scripts/pluginsdk"), { recursive: true });
    const record = {
      schema_version: 1,
      sdk_version: "0.1.0",
      host_release: "0.100.0",
      min_kandev_version: "0.100.0",
      api_versions: [1, 2],
      change_kind: "initial",
    };
    await writeFile(
      join(repo, "pluginsdk/sdk-release.json"),
      JSON.stringify(record) + "\n",
    );
    await writeFile(join(repo, "pluginsdk/serve.go"), "package pluginsdk\n");
    await writeFile(
      join(repo, "scripts/pluginsdk/pack-config.json"),
      JSON.stringify({
        node: "24.19.0",
        npm: "11.21.0",
        files: ["package.json"],
      }),
    );
    git("add", ".");
    git("commit", "--quiet", "-m", "fixture");
    const original = git("rev-parse", "HEAD");
    git("tag", "-s", "v0.100.0", "-m", "fixture");
    git("push", "--quiet", "origin", "--tags");
    const sourceRepo = new SourceRepository({ root: repo, fingerprint, env });
    const first = await sourceRepo.loadSource("v0.100.0");
    assert.equal(first.commit, original);
    assert.equal(await sourceRepo.readSDKTag(first), null);
    await sourceRepo.createSDKTag(first);
    assert.equal((await sourceRepo.readSDKTag(first))?.commit, original);
    await sourceRepo.createSDKTag(first);
    const receipt = join(root, "receipt");
    const signature = join(root, "receipt.asc");
    await writeFile(receipt, "original bytes");
    await sourceRepo.signReceipt(receipt, signature);
    assert.equal(
      await sourceRepo.verifyReceipt(receipt, signature),
      fingerprint,
    );
    await writeFile(receipt, "changed bytes");
    await assert.rejects(sourceRepo.verifyReceipt(receipt, signature));
    await writeFile(join(repo, "pluginsdk/README.md"), "later host README\n");
    git("add", ".");
    git("commit", "--quiet", "-m", "later host");
    git("tag", "-s", "v0.101.0", "-m", "later");
    git("push", "--quiet", "origin", "--tags");
    assert.equal((await sourceRepo.loadSource("v0.101.0")).commit, original);
    const wrong = new SourceRepository({
      root: repo,
      fingerprint: "F".repeat(40),
      env,
    });
    await assert.rejects(wrong.loadSource("v0.100.0"), /signature/);
    git("tag", "v0.102.0");
    git("push", "--quiet", "origin", "refs/tags/v0.102.0");
    await assert.rejects(sourceRepo.loadSource("v0.102.0"), /signature|tag/);
    await writeFile(
      join(repo, "pluginsdk/serve.go"),
      "package pluginsdk\nconst Changed = true\n",
    );
    git("add", ".");
    git("commit", "--quiet", "-m", "changed contract");
    git("tag", "-s", "v0.103.0", "-m", "changed");
    git("push", "--quiet", "origin", "--tags");
    await assert.rejects(
      sourceRepo.loadSource("v0.103.0"),
      /contract.*version/,
    );
  } finally {
    execFileSync("gpgconf", [
      "--homedir",
      join(root, "keyring"),
      "--kill",
      "gpg-agent",
    ]);
    await rm(root, { recursive: true, force: true });
  }
});
