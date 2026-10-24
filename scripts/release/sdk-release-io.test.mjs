import test from "node:test";
import assert from "node:assert/strict";
import { ReleaseIO } from "./sdk-release-io.mjs";

test("npm adapter publishes the retained archive without changing latest implicitly", async () => {
  const calls = [];
  const io = Object.create(ReleaseIO.prototype);
  io.npm = async (args) => calls.push(args);
  await io.publish({ archive: "/tmp/original/plugin-sdk-0.1.0.tgz" });
  await io.moveLatest("0.1.0");
  assert.deepEqual(calls, [
    [
      "publish",
      "/tmp/original/plugin-sdk-0.1.0.tgz",
      "--provenance",
      "--access",
      "public",
      "--ignore-scripts",
      "--registry",
      "https://registry.npmjs.org",
      "--tag",
      "sdk-release",
    ],
    [
      "dist-tag",
      "add",
      "@kandev/plugin-sdk@0.1.0",
      "latest",
      "--registry",
      "https://registry.npmjs.org",
    ],
  ]);
});

test("public consumer visibility retries are bounded and never report exhausted verification as success", async () => {
  const io = Object.create(ReleaseIO.prototype);
  io.source = {
    record: { sdk_version: "0.1.0" },
    recordDigest: "a".repeat(64),
    commit: "b".repeat(40),
    hostTag: "v0.100.0",
    sdkTag: "pluginsdk/v0.1.0",
  };
  let attempts = 0;
  io.smoke = async () => {
    attempts++;
    throw new Error("proxy not visible");
  };
  io.wait = async () => {};
  await assert.rejects(
    io.verifyConsumers({ integrity: "sha512-fixture" }, "oidc"),
  );
  assert.equal(attempts, 3);
});
