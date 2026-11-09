import assert from "node:assert/strict";
import test from "node:test";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import {
  PHASES,
  SdkDiagnostics,
  classifyFailure,
  collectSecrets,
  sanitizeOutput,
} from "./sdk-diagnostics.mjs";
import { ReleaseIO } from "./sdk-release-io.mjs";
import { runRelease } from "./publish-plugin-sdk.mjs";

test("sanitizes known credentials, bearer headers, npmrc lines, JWTs, and signed URLs", () => {
  const secret = "known-secret-value";
  const npmToken = "npm_" + "a".repeat(36);
  const jwt = "eyJabcdefgh.abcdefgh.abcdefgh";
  const raw = [
    `Authorization: Bearer ${secret}`,
    "Proxy-Authorization: Basic cHJveHk6cGFzcw==",
    "authorization: basic bG93ZXI6c2VjcmV0",
    "AUTHORIZATION: BASIC dXBwZXI6c2VjcmV0",
    "Proxy-authorization: Bearer proxy-secret-token",
    `//registry.npmjs.org/:_authToken=${npmToken}`,
    `_auth=${secret}`,
    `JWT ${jwt}`,
    `token=${secret}`,
    "https://user:password@example.test/archive",
    "https://example.test/download?X-Amz-Signature=abcdef&x=1",
    "https://example.test/download?access_token=abcdef",
  ].join("\n");
  const safe = sanitizeOutput(raw, { secrets: [secret] });
  assert.equal(safe.truncated, false);
  for (const value of [
    secret,
    "cHJveHk6cGFzcw==",
    "bG93ZXI6c2VjcmV0",
    "dXBwZXI6c2VjcmV0",
    "proxy-secret-token",
    npmToken,
    jwt,
    "user:password",
    "X-Amz-Signature=abcdef",
    "access_token=abcdef",
  ])
    assert.equal(safe.text.includes(value), false, value);
  assert.match(safe.text, /\[redacted\]/);
  assert.match(safe.text, /\[redacted-jwt\]/);
  assert.match(safe.text, /\[redacted-url\]/);
});

test("retains only a bounded tail of sanitized command output", () => {
  const raw = "secret\n" + "x".repeat(70 * 1024);
  const safe = sanitizeOutput(raw, {
    secrets: ["secret"],
    limit: 1024,
  });
  assert.equal(safe.truncated, true);
  assert.ok(safe.original_bytes > 70 * 1024, safe.original_bytes);
  assert.ok(safe.retained_bytes <= 1024, safe.retained_bytes);
  assert.ok(safe.text.length <= 1024);
  assert.ok(safe.text.includes("[truncated"));
  assert.equal(safe.text.includes("secret\n"), false);
});

test("collects only credential-shaped environment values", () => {
  const secrets = collectSecrets({
    NPM_TOKEN: "npm-secret-value",
    GITHUB_TOKEN: "github-secret-value",
    RELEASE_GPG_FINGERPRINT: "A".repeat(40),
    PATH: "/bin",
  });
  assert.deepEqual(secrets.sort(), ["github-secret-value", "npm-secret-value"]);
});

test("classifies authentication, registry, visibility, identity, and consumer failures", () => {
  const cases = [
    [
      Object.assign(new Error("npm error code ENEEDAUTH"), {
        code: "ENEEDAUTH",
      }),
      "authentication",
    ],
    [
      Object.assign(
        new Error(
          "Command failed: npm publish /tmp/plugin-sdk.tgz --provenance --access public",
        ),
        { npmCode: "ENEEDAUTH" },
      ),
      "authentication",
    ],
    [
      Object.assign(
        new Error("Command failed: npm publish --provenance"),
        { category: "authentication" },
      ),
      "authentication",
    ],
    [
      Object.assign(new Error("registry read failed with HTTP 500"), {
        code: "REGISTRY_READ_FAILURE",
      }),
      "registry_read",
    ],
    [
      Object.assign(new Error("has not become visible"), {
        code: "VISIBILITY_TIMEOUT",
        acceptedUpload: true,
      }),
      "accepted_upload_awaiting_visibility",
    ],
    [
      Object.assign(new Error("did not become visible"), {
        code: "VISIBILITY_TIMEOUT",
        acceptedUpload: false,
      }),
      "uncertain_upload",
    ],
    [
      Object.assign(new Error("latest did not become visible"), {
        code: "VISIBILITY_TIMEOUT",
        acceptedUpload: true,
        phase: "channel_promotion",
      }),
      "accepted_channel_awaiting_visibility",
    ],
    [
      Object.assign(new Error("latest did not become visible"), {
        code: "VISIBILITY_TIMEOUT",
        acceptedUpload: false,
        phase: "channel_promotion",
      }),
      "uncertain_channel_update",
    ],
    [
      new Error("npm version integrity or record conflict"),
      "identity_conflict",
    ],
    [
      Object.assign(new Error("public consumer verification failed"), {
        code: "CONSUMER_VERIFICATION",
      }),
      "consumer_verification",
    ],
    [
      Object.assign(
        new Error("public consumer verification failed for provenance build"),
        { code: "CONSUMER_VERIFICATION" },
      ),
      "consumer_verification",
    ],
    [new Error("unexpected"), "unknown"],
  ];
  for (const [error, category] of cases)
    assert.equal(classifyFailure(error).category, category);
});

test("accepted upload visibility timeout reports upload success and pending registry state", () => {
  const diagnostics = new SdkDiagnostics({
    hostTag: "v1.2.3",
    sdkVersion: "0.1.0",
    log: null,
  });
  diagnostics.phase("source_verification", "passed");
  diagnostics.start("candidate_verification");
  diagnostics.phase("candidate_verification", "passed");
  diagnostics.start("npm_upload");
  diagnostics.setWriteOutcome("accepted");
  diagnostics.phase("npm_upload", "passed", { write_outcome: "accepted" });
  diagnostics.start("version_visibility");
  const failure = Object.assign(
    new Error("npm publication was accepted by npm but is not visible"),
    { code: "VISIBILITY_TIMEOUT", acceptedUpload: true },
  );
  diagnostics.fail(failure);
  const summary = diagnostics.summary({ status: "failed", failure });
  assert.equal(summary.publication, "accepted_upload_awaiting_visibility");
  assert.equal(summary.write_outcome, "accepted");
  assert.equal(summary.last_confirmed_phase, "npm_upload");
  assert.equal(
    summary.failure_category,
    "accepted_upload_awaiting_visibility",
  );
  assert.match(summary.recovery_action, /rerun the failed job in the same release/);
});

test("consumer failure preserves confirmed publication and channel promotion", () => {
  const diagnostics = new SdkDiagnostics({
    hostTag: "v1.2.3",
    sdkVersion: "0.1.0",
    log: null,
  });
  diagnostics.phase("source_verification", "passed");
  diagnostics.phase("candidate_verification", "passed");
  diagnostics.phase("npm_upload", "passed", { write_outcome: "accepted" });
  diagnostics.phase("version_visibility", "passed");
  diagnostics.phase("channel_promotion", "passed");
  diagnostics.setWriteOutcome("accepted");
  diagnostics.setPublication("confirmed");
  diagnostics.start("public_consumers");
  const failure = Object.assign(
    new Error("public consumer verification failed: npm install failed"),
    { code: "CONSUMER_VERIFICATION" },
  );
  diagnostics.fail(failure);
  const summary = diagnostics.summary({ status: "failed", failure });
  assert.equal(summary.publication, "confirmed");
  assert.equal(summary.channel_promotion, "passed");
  assert.equal(summary.public_consumers, "failed");
  assert.equal(summary.last_confirmed_phase, "channel_promotion");
  assert.equal(summary.failure_category, "consumer_verification");
});

test("uncertain upload summary neither claims publication failure nor success", () => {
  const diagnostics = new SdkDiagnostics({
    hostTag: "v1.2.3",
    sdkVersion: "0.1.0",
    log: null,
  });
  diagnostics.setWriteOutcome("uncertain");
  diagnostics.start("version_visibility");
  const failure = Object.assign(new Error("did not become visible"), {
    code: "VISIBILITY_TIMEOUT",
    acceptedUpload: false,
  });
  diagnostics.fail(failure);
  const summary = diagnostics.summary({ status: "failed", failure });
  assert.equal(summary.publication, "uncertain_upload");
  assert.equal(summary.failure_category, "uncertain_upload");
  assert.equal(JSON.stringify(summary).includes('"publication":"confirmed"'), false);
});

test("observation records stay bounded while retaining the latest attempts", () => {
  const diagnostics = new SdkDiagnostics({
    hostTag: "v1.2.3",
    sdkVersion: "0.1.0",
    log: null,
  });
  for (let attempt = 1; attempt <= 50; attempt++)
    diagnostics.observation({
      phase: "version_visibility",
      attempt,
      elapsed_ms: attempt * 1000,
      next_wait_ms: 1000,
    });
  const summary = diagnostics.summary({ status: "failed" });
  assert.equal(summary.observations.length, 20);
  assert.equal(summary.observations.at(-1).attempt, 50);
  assert.equal(
    Object.keys(summary.phases).length,
    PHASES.length,
  );
});

test("command adapter sanitizes successful and failed npm output", async () => {
  const root = await mkdtemp(join(tmpdir(), "sdk-diagnostics-command-"));
  try {
    const secret = "command-secret-value";
    const basicToken = "dXNlcjpwYXNz";
    const proxyToken = "cHJveHk6cGFzcw==";
    const env = { NPM_TOKEN: secret };
    const diagnostics = new SdkDiagnostics({
      hostTag: "v1.2.3",
      sdkVersion: "0.1.0",
      log: null,
    });
    const success = new ReleaseIO({
      root,
      directory: join(root, "success"),
      repository: {},
      source: {},
      env,
      diagnostics,
      run: async () => ({
        stdout: `Authorization: Bearer ${secret}\nProxy-Authorization: Basic ${proxyToken}\nauthorization: basic ${basicToken}\nnpm notice published`,
        stderr: "",
      }),
    });
    await success.npm(["publish", "--registry", "https://registry.npmjs.org"]);
    assert.equal(JSON.stringify(diagnostics.commands).includes(secret), false);
    assert.equal(
      JSON.stringify(diagnostics.commands).includes(basicToken),
      false,
    );
    assert.equal(
      JSON.stringify(diagnostics.commands).includes(proxyToken),
      false,
    );
    assert.match(diagnostics.commands[0].output, /npm notice published/);
    const successSummary = diagnostics.summary({
      status: "failed",
      failure: new Error("synthetic failure"),
    });
    assert.equal(JSON.stringify(successSummary).includes(basicToken), false);
    assert.equal(JSON.stringify(successSummary).includes(proxyToken), false);

    const failedDiagnostics = new SdkDiagnostics({
      hostTag: "v1.2.3",
      sdkVersion: "0.1.0",
      log: null,
    });
    const failed = new ReleaseIO({
      root,
      directory: join(root, "failed"),
      repository: {},
      source: {},
      env,
      diagnostics: failedDiagnostics,
      run: async () => {
        throw Object.assign(new Error("command failed"), {
          code: 1,
          stdout: "",
          stderr: `npm error code ENEEDAUTH\nAuthorization: Bearer ${secret}\nProxy-Authorization: Basic ${proxyToken}\nauthorization: basic ${basicToken}`,
        });
      },
    });
    let failure;
    await assert.rejects(failed.npm(["publish"]), (error) => {
      failure = error;
      assert.equal(error.message.includes(secret), false);
      assert.equal(error.message.includes(basicToken), false);
      assert.equal(error.message.includes(proxyToken), false);
      assert.match(error.message, /ENEEDAUTH/);
      assert.equal(classifyFailure(error).category, "authentication");
      return true;
    });
    assert.equal(
      JSON.stringify(failedDiagnostics.commands).includes(secret),
      false,
    );
    assert.equal(
      JSON.stringify(failedDiagnostics.commands).includes(basicToken),
      false,
    );
    const failedSummary = failedDiagnostics.summary({
      status: "failed",
      failure,
    });
    assert.equal(JSON.stringify(failedSummary).includes(basicToken), false);
    assert.equal(JSON.stringify(failedSummary).includes(proxyToken), false);
    assert.equal(failedSummary.failure_category, "authentication");
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test("npm publish argv provenance does not override an authentication failure", async () => {
  const root = await mkdtemp(join(tmpdir(), "sdk-diagnostics-auth-"));
  try {
    const diagnostics = new SdkDiagnostics({
      hostTag: "v1.2.3",
      sdkVersion: "0.1.0",
      log: null,
    });
    const io = new ReleaseIO({
      root,
      directory: join(root, "work"),
      repository: {},
      source: {},
      env: {},
      diagnostics,
      run: async () => {
        throw Object.assign(
          new Error(
            "Command failed: npm publish /tmp/plugin-sdk-0.1.0.tgz --provenance --access public --registry https://registry.npmjs.org",
          ),
          {
            code: 1,
            stdout: "",
            stderr: "npm error code ENEEDAUTH\nnpm error need auth",
          },
        );
      },
    });
    let failure;
    await assert.rejects(
      io.npm([
        "publish",
        "/tmp/plugin-sdk-0.1.0.tgz",
        "--provenance",
        "--access",
        "public",
        "--registry",
        "https://registry.npmjs.org",
      ]),
      (error) => {
        failure = error;
        return true;
      },
    );
    assert.equal(failure.category, "authentication");
    assert.equal(failure.npmCode, "ENEEDAUTH");
    const classification = classifyFailure(failure);
    assert.equal(classification.category, "authentication");
    assert.match(classification.recovery_action, /trusted publisher/);
    assert.equal(classification.recovery_action.includes("immutable"), false);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

function runnerFixture({ summaryWriter }) {
  const order = [];
  const errors = [];
  const record = {
    schema_version: 1,
    sdk_version: "0.1.0",
    host_release: "1.2.3",
    min_kandev_version: "1.2.3",
    api_versions: [1],
    change_kind: "initial",
  };
  const source = {
    commit: "b".repeat(40),
    currentCommit: "b".repeat(40),
    hostTag: "v1.2.3",
    sdkTag: "pluginsdk/v0.1.0",
    recordDigest: "c".repeat(64),
    record,
    packConfig: { node: "24.19.0", npm: "11.21.0" },
  };
  const repository = {
    loadSource: async () => source,
    readSDKTag: async () => {
      order.push("read-tag");
      throw new Error("tag read failed");
    },
  };
  return {
    order,
    errors,
    run: () =>
      runRelease({
        mode: "verify",
        hostTag: "v1.2.3",
        env: { RELEASE_GPG_FINGERPRINT: "A".repeat(40) },
        makeRepository: () => repository,
        makeDirectory: async () => {
          order.push("directory");
          return "/tmp/sdk-runner-fixture";
        },
        removeDirectory: async () => order.push("cleanup"),
        summaryWriter:
          summaryWriter ??
          (async () => {
            order.push("summary");
          }),
        verifyToolsImpl: () => {},
        execFileImpl: async (command) => ({
          stdout: command === "npm" ? "11.21.0" : "",
        }),
        diagnosticsLog: null,
        diagnosticsError: (...args) => errors.push(args.join(" ")),
      }),
  };
}

test("runner writes its phase summary before cleanup and preserves the original failure", async () => {
  const fixture = runnerFixture({});
  await assert.rejects(fixture.run(), /tag read failed/);
  assert.deepEqual(fixture.order, [
    "directory",
    "read-tag",
    "summary",
    "cleanup",
  ]);
});

test("runner summary failure does not replace the original release error", async () => {
  const fixture = runnerFixture({
    summaryWriter: async () => {
      fixture.order.push("summary");
      throw new Error("summary unavailable");
    },
  });
  await assert.rejects(fixture.run(), /tag read failed/);
  assert.deepEqual(fixture.order, [
    "directory",
    "read-tag",
    "summary",
    "cleanup",
  ]);
  assert.ok(
    fixture.errors.some((message) =>
      message.includes("summary could not be written"),
    ),
    fixture.errors,
  );
});
