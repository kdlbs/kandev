import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import test from "node:test";
import {
  OIDC_AUDIENCE,
  REQUEST_TIMEOUT_MS,
  exchangePackageToken,
  runPreflight,
} from "./npm-auth-preflight.mjs";
import { packageName as sdkPackageName, registryURL } from "./sdk-registry.mjs";

const RUNTIME_PACKAGES = [
  "kandev",
  "@kdlbs/runtime-linux-x64",
  "@kdlbs/runtime-linux-arm64",
  "@kdlbs/runtime-darwin-x64",
  "@kdlbs/runtime-darwin-arm64",
  "@kdlbs/runtime-win32-x64",
];
const WORKFLOW_SHA = "d".repeat(40);
const CHECKOUT_SHA = "e".repeat(40);
const FIXED_NOW = Date.UTC(2026, 10, 11, 12);
const APPROVED_ENV = {
  GITHUB_ACTIONS: "true",
  GITHUB_REPOSITORY: "kdlbs/kandev",
  GITHUB_EVENT_NAME: "workflow_dispatch",
  GITHUB_REF: "refs/heads/main",
  GITHUB_WORKFLOW_REF:
    "kdlbs/kandev/.github/workflows/release.yml@refs/heads/main",
  RUNNER_ENVIRONMENT: "github-hosted",
  SDK_SCOPE_OWNERSHIP_CONFIRMED: "true",
  GITHUB_WORKFLOW_SHA: WORKFLOW_SHA,
  ACTIONS_ID_TOKEN_REQUEST_URL: "https://actions.test/token",
  ACTIONS_ID_TOKEN_REQUEST_TOKEN: "request-token-value",
};

function env(overrides = {}) {
  return { ...APPROVED_ENV, ...overrides };
}

function makeJwt({
  audience = OIDC_AUDIENCE,
  expires = Math.floor((FIXED_NOW + 3_600_000) / 1000),
} = {}) {
  const header = Buffer.from(
    JSON.stringify({ alg: "none", typ: "JWT" }),
  ).toString("base64url");
  const payload = Buffer.from(
    JSON.stringify({ aud: audience, exp: expires }),
  ).toString("base64url");
  return `${header}.${payload}.c2lnbmF0dXJl`;
}

function jsonResponse(status, body, url) {
  const response = new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
  Object.defineProperty(response, "url", { value: url });
  return response;
}

function makeTransport({
  sdkExists = true,
  sdkStatus = sdkExists ? 200 : 404,
  failPackage = null,
  exchangeStatus = 201,
  exchangeBody,
  oidcBody = null,
} = {}) {
  const calls = [];
  const exchanges = [];
  const fetchImpl = async (input, options = {}) => {
    const target = input instanceof URL ? input.toString() : input;
    calls.push({
      target,
      method: options.method ?? "GET",
      redirect: options.redirect,
      headers: options.headers ?? {},
      signal: options.signal,
    });
    if (target.startsWith("https://actions.test/token")) {
      const url = new URL(target);
      assert.equal(url.searchParams.get("audience"), OIDC_AUDIENCE);
      return jsonResponse(200, oidcBody ?? { value: makeJwt() }, target);
    }
    if (target === `${registryURL}/@kdlbs%2fplugin-sdk`) {
      if (sdkStatus === 404)
        return jsonResponse(404, { error: "Not found" }, target);
      if (sdkStatus !== 200)
        return jsonResponse(
          sdkStatus,
          { error: "registry unavailable" },
          target,
        );
      return jsonResponse(
        200,
        { name: sdkPackageName, versions: {}, "dist-tags": {} },
        target,
      );
    }
    if (
      target.startsWith(`${registryURL}/-/npm/v1/oidc/token/exchange/package/`)
    ) {
      const encoded = target.slice(
        target.indexOf("/package/") + "/package/".length,
      );
      const packageName = decodeURIComponent(encoded);
      exchanges.push({
        package: packageName,
        encoded,
        headers: options.headers ?? {},
        method: options.method,
      });
      if (packageName === failPackage)
        return jsonResponse(
          exchangeStatus,
          { error: "sensitive-server-body" },
          target,
        );
      const body =
        typeof exchangeBody === "function"
          ? exchangeBody(packageName)
          : exchangeBody;
      return jsonResponse(
        201,
        body === undefined ? { token: `npm_${"a".repeat(36)}` } : body,
        target,
      );
    }
    throw new Error(`unexpected request: ${target}`);
  };
  return { fetchImpl, calls, exchanges };
}

function cliFetchBootstrap({
  oidcToken,
  exchangeStatus = 201,
  exchangeBody = { token: "npm_token_only_secret" },
}) {
  const sdkMetadata = {
    name: sdkPackageName,
    versions: {},
    "dist-tags": {},
  };
  return `Date.now = () => ${FIXED_NOW}; const response = (status, body, url) => { const value = new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } }); Object.defineProperty(value, 'url', { value: url }); return value; }; globalThis.fetch = async (input) => { const target = String(input); if (target.startsWith('https://actions.test/token')) return response(200, { value: ${JSON.stringify(oidcToken)} }, target); if (target === ${JSON.stringify(`${registryURL}/@kdlbs%2fplugin-sdk`)}) return response(200, ${JSON.stringify(sdkMetadata)}, target); return response(${exchangeStatus}, ${JSON.stringify(exchangeBody)}, target); };`;
}

test("one rejected publisher blocks the whole preflight before any release mutation", async () => {
  const transport = makeTransport({
    failPackage: "@kdlbs/runtime-linux-arm64",
    exchangeStatus: 403,
    exchangeBody: () => ({ token: `npm_${"a".repeat(36)}` }),
  });
  await assert.rejects(
    runPreflight({
      packages: RUNTIME_PACKAGES,
      env: env(),
      fetchImpl: transport.fetchImpl,
      now: () => FIXED_NOW,
      checkoutSha: CHECKOUT_SHA,
    }),
    (error) => {
      assert.equal(error.package, "@kdlbs/runtime-linux-arm64");
      assert.equal(error.category, "npm_authentication");
      assert.equal(error.status, 403);
      assert.match(error.message, /trusted publisher/);
      assert.equal(error.message.includes("sensitive-server-body"), false);
      return true;
    },
  );
  assert.deepEqual(
    transport.exchanges.map((exchange) => exchange.package),
    ["kandev", "@kdlbs/runtime-linux-x64", "@kdlbs/runtime-linux-arm64"],
  );
  for (const call of transport.calls) {
    assert.equal(call.redirect, "error");
    assert.equal(call.signal !== undefined, true);
    assert.equal(["GET", "POST"].includes(call.method), true);
    assert.equal(
      /whoami|\/trust|\/access|\/tokens|publish|signing/i.test(call.target),
      false,
      call.target,
    );
  }
});

test("checks the audience and encoded package route for every exchange", async () => {
  const transport = makeTransport();
  const summary = await runPreflight({
    packages: RUNTIME_PACKAGES,
    env: env(),
    fetchImpl: transport.fetchImpl,
    now: () => FIXED_NOW,
    checkoutSha: CHECKOUT_SHA,
  });
  assert.equal(summary.status, "passed");
  assert.equal(summary.workflow_sha, WORKFLOW_SHA);
  assert.equal(summary.checkout_sha, CHECKOUT_SHA);
  assert.deepEqual(
    transport.exchanges.map((exchange) => exchange.package),
    [...RUNTIME_PACKAGES, sdkPackageName],
  );
  assert.ok(
    transport.exchanges
      .map((exchange) => exchange.encoded)
      .includes("%40kdlbs%2Fruntime-linux-x64"),
  );
  for (const exchange of transport.exchanges) {
    assert.equal(exchange.method, "POST");
    assert.match(exchange.headers.authorization, /^Bearer eyJ/);
  }
  const oidcRequests = transport.calls.filter((call) =>
    call.target.startsWith("https://actions.test/token"),
  );
  assert.equal(oidcRequests.length, RUNTIME_PACKAGES.length + 1);
  assert.equal(summary.packages.at(-1).package, sdkPackageName);
  assert.equal(summary.packages.at(-1).status, "authenticated");
  const serialized = JSON.stringify(summary);
  assert.equal(serialized.includes("request-token-value"), false);
  assert.equal(serialized.includes(`npm_${"a".repeat(36)}`), false);
  assert.equal(serialized.includes("eyJ"), false);
});

test("token-only package exchanges pass the full preflight", async () => {
  const transport = makeTransport({
    oidcBody: {
      value: makeJwt({ expires: Math.floor((FIXED_NOW + 3_600_000) / 1000) }),
    },
    exchangeBody: () => ({ token: "npm_token_only_secret" }),
  });

  const summary = await runPreflight({
    packages: RUNTIME_PACKAGES,
    env: env(),
    fetchImpl: transport.fetchImpl,
    now: () => FIXED_NOW,
    checkoutSha: CHECKOUT_SHA,
  });

  assert.equal(summary.status, "passed");
  assert.deepEqual(
    summary.packages.map(({ package: packageName, status }) => ({
      package: packageName,
      status,
    })),
    [...RUNTIME_PACKAGES, sdkPackageName].map((packageName) => ({
      package: packageName,
      status: "authenticated",
    })),
  );
  assert.equal(
    JSON.stringify(summary).includes("npm_token_only_secret"),
    false,
  );
  assert.equal(JSON.stringify(summary).includes("expires_at"), false);
});

test("exchange metadata is ignored across the full package inventory", async () => {
  const metadataFields = ["token_type", "created", "expires"];
  const metadataValues = [
    { name: "absent", absent: true },
    { name: "null", value: null },
    { name: "malformed", value: "ignored-metadata-sentinel" },
    { name: "numeric", value: 1_234_567_890 },
    {
      name: "object",
      value: { source: "ignored-metadata-sentinel" },
    },
    {
      name: "past date",
      value: new Date(FIXED_NOW - 3_600_000).toISOString(),
    },
  ];

  for (const field of metadataFields) {
    for (const { name, absent, value } of metadataValues) {
      const responseMetadataSentinel = `ignored-${field}-${name}-metadata-sentinel`;
      const transport = makeTransport({
        oidcBody: {
          value: makeJwt({
            expires: Math.floor((FIXED_NOW + 3_600_000) / 1000),
          }),
        },
        exchangeBody: () => {
          const body = {
            token_type: "oidc",
            token: "npm_ignored_exchange_credential",
            created: new Date(FIXED_NOW).toISOString(),
            expires: new Date(FIXED_NOW + 3_600_000).toISOString(),
            provider_notice: responseMetadataSentinel,
          };
          if (absent) delete body[field];
          else body[field] = value;
          return body;
        },
      });

      const summary = await runPreflight({
        packages: RUNTIME_PACKAGES,
        env: env(),
        fetchImpl: transport.fetchImpl,
        now: () => FIXED_NOW,
        checkoutSha: CHECKOUT_SHA,
      });
      const caseName = `${field} ${name}`;

      assert.equal(summary.status, "passed", caseName);
      assert.deepEqual(
        summary.packages.map(({ package: packageName, status }) => ({
          package: packageName,
          status,
        })),
        [...RUNTIME_PACKAGES, sdkPackageName].map((packageName) => ({
          package: packageName,
          status: "authenticated",
        })),
        caseName,
      );
      const serialized = JSON.stringify(summary);
      assert.equal(
        serialized.includes("npm_ignored_exchange_credential"),
        false,
      );
      assert.equal(serialized.includes(responseMetadataSentinel), false);
      assert.equal(serialized.includes("expires_at"), false);
    }
  }
});

test("full documented exchange metadata remains accepted", async () => {
  const transport = makeTransport({
    oidcBody: {
      value: makeJwt({ expires: Math.floor((FIXED_NOW + 3_600_000) / 1000) }),
    },
    exchangeBody: () => ({
      token_type: "oidc",
      token: "npm_documented_metadata_token",
      created: new Date(FIXED_NOW).toISOString(),
      expires: new Date(FIXED_NOW + 3_600_000).toISOString(),
    }),
  });
  const summary = await runPreflight({
    packages: RUNTIME_PACKAGES,
    env: env(),
    fetchImpl: transport.fetchImpl,
    now: () => FIXED_NOW,
    checkoutSha: CHECKOUT_SHA,
  });

  assert.equal(summary.status, "passed");
  assert.equal(JSON.stringify(summary).includes("expires_at"), false);
});

test("confirmed whole-package SDK absence defers only the SDK check", async () => {
  const transport = makeTransport({ sdkExists: false });
  const summary = await runPreflight({
    packages: RUNTIME_PACKAGES,
    env: env(),
    fetchImpl: transport.fetchImpl,
    now: () => FIXED_NOW,
    checkoutSha: CHECKOUT_SHA,
  });
  assert.equal(summary.status, "passed");
  assert.deepEqual(summary.packages.at(-1), {
    package: sdkPackageName,
    status: "deferred",
    reason: "package_absent",
  });
  assert.equal(
    transport.exchanges.some((exchange) => exchange.package === sdkPackageName),
    false,
  );
  assert.equal(
    transport.calls.filter((call) => call.target.includes("/exchange/")).length,
    RUNTIME_PACKAGES.length,
  );
});

test("a missing runtime package fails explicitly while SDK absence defers", async () => {
  const transport = makeTransport({
    sdkExists: false,
    failPackage: "kandev",
    exchangeStatus: 404,
  });
  await assert.rejects(
    runPreflight({
      packages: RUNTIME_PACKAGES,
      env: env(),
      fetchImpl: transport.fetchImpl,
      now: () => FIXED_NOW,
      checkoutSha: CHECKOUT_SHA,
    }),
    (error) => {
      assert.equal(error.package, "kandev");
      assert.equal(error.status, 404);
      assert.equal(error.category, "npm_authentication");
      assert.match(error.message, /package not found/);
      return true;
    },
  );
});

test("unreadable SDK registry state never becomes bootstrap deferral", async () => {
  const transport = makeTransport({ sdkStatus: 500 });
  await assert.rejects(
    runPreflight({
      packages: RUNTIME_PACKAGES,
      env: env(),
      fetchImpl: transport.fetchImpl,
      now: () => FIXED_NOW,
      checkoutSha: CHECKOUT_SHA,
    }),
    (error) => {
      assert.equal(error.package, sdkPackageName);
      assert.equal(error.category, "registry_read");
      assert.match(error.message, /confirmed package absence/);
      return true;
    },
  );
  assert.equal(
    transport.exchanges.some((exchange) => exchange.package === sdkPackageName),
    false,
  );
});

test("publisher rejection and invalid exchange bodies or tokens still fail", async () => {
  const cases = [
    { exchangeStatus: 500, failPackage: "kandev" },
    { exchangeBody: null },
    { exchangeBody: [] },
    { exchangeBody: {} },
    { exchangeBody: { token: "" } },
    { exchangeBody: { token: 4 } },
  ];
  for (const options of cases) {
    const transport = makeTransport(options);
    await assert.rejects(
      runPreflight({
        packages: ["kandev"],
        env: env(),
        fetchImpl: transport.fetchImpl,
        now: () => FIXED_NOW,
        checkoutSha: CHECKOUT_SHA,
      }),
      (error) => {
        assert.equal(error.package, "kandev");
        assert.equal(
          error.category,
          options.exchangeStatus
            ? "npm_authentication"
            : "npm_exchange_response",
        );
        assert.equal(error.message.includes("sensitive-server-body"), false);
        return true;
      },
    );
  }
});

test("exchange validation reports fixed reasons for each response condition", async () => {
  const cases = [
    { body: null, reason: "invalid_body_shape" },
    { body: [], reason: "invalid_body_shape" },
    { body: { token: "" }, reason: "missing_token" },
    { body: {}, reason: "missing_token" },
    { body: { token: 4 }, reason: "missing_token" },
  ];
  for (const { body, reason, category = "npm_exchange_response" } of cases) {
    const fetchImpl = async (input) => jsonResponse(201, body, String(input));
    await assert.rejects(
      exchangePackageToken("kandev", "oidc", { fetchImpl }),
      (error) => {
        assert.equal(error.status, 201);
        assert.equal(error.category, category);
        assert.equal(error.reason, reason);
        return true;
      },
    );
  }
  await assert.rejects(
    exchangePackageToken("kandev", "oidc", {
      fetchImpl: async (input) => {
        const response = new Response('{"token":"untrusted-body-secret"', {
          status: 201,
        });
        Object.defineProperty(response, "url", { value: String(input) });
        return response;
      },
    }),
    (error) => {
      assert.equal(error.category, "npm_exchange_response");
      assert.equal(error.status, 201);
      assert.equal(error.reason, "unreadable_json");
      assert.equal(error.message.includes("untrusted-body-secret"), false);
      return true;
    },
  );
});

test("CLI emits a credential-free summary for token-only package exchanges", () => {
  const jwt = makeJwt();
  const exchangeToken = "npm_cli_token_only_secret";
  const bootstrap = cliFetchBootstrap({
    oidcToken: jwt,
    exchangeBody: { token: exchangeToken },
  });
  const result = spawnSync(
    process.execPath,
    [
      "--import",
      `data:text/javascript,${encodeURIComponent(bootstrap)}`,
      fileURLToPath(new URL("./npm-auth-preflight.mjs", import.meta.url)),
      "--package",
      "kandev",
      "--checkout-sha",
      CHECKOUT_SHA,
    ],
    { encoding: "utf8", env: env() },
  );
  const emitted = `${result.stdout}\n${result.stderr}`;

  assert.equal(result.status, 0, emitted);
  assert.deepEqual(JSON.parse(result.stdout.trim()), {
    status: "passed",
    workflow_sha: WORKFLOW_SHA,
    checkout_sha: CHECKOUT_SHA,
    packages: [
      { package: "kandev", status: "authenticated" },
      { package: sdkPackageName, status: "authenticated" },
    ],
  });
  for (const secret of [
    exchangeToken,
    jwt,
    "request-token-value",
    "token_type",
    "created",
    "expires",
    "expires_at",
  ])
    assert.equal(emitted.includes(secret), false, secret);
});

test("failure summary serialization includes safe context and excludes credentials and metadata", () => {
  const credentialSecret = "npm_sentinel_secret_123";
  const createdMetadata = "ignored-created-metadata-sentinel";
  const untrustedMetadata = "untrusted-response-metadata-sentinel";
  const ignoredExpiry = "ignored-expiry-metadata-sentinel";
  const jwt = makeJwt();
  const bootstrap = cliFetchBootstrap({
    oidcToken: jwt,
    exchangeBody: {
      token: 4,
      token_echo: credentialSecret,
      token_type: null,
      created: createdMetadata,
      expires: ignoredExpiry,
      malicious: untrustedMetadata,
    },
  });
  const result = spawnSync(
    process.execPath,
    [
      "--import",
      `data:text/javascript,${encodeURIComponent(bootstrap)}`,
      fileURLToPath(new URL("./npm-auth-preflight.mjs", import.meta.url)),
      "--package",
      "kandev",
      "--checkout-sha",
      CHECKOUT_SHA,
    ],
    {
      encoding: "utf8",
      env: env(),
    },
  );
  const emitted = `${result.stdout}\n${result.stderr}`;
  assert.equal(result.status, 1);
  assert.deepEqual(JSON.parse(result.stdout.trim()), {
    status: "failed",
    package: "kandev",
    category: "npm_exchange_response",
    http_status: 201,
    reason: "missing_token",
    workflow_sha: WORKFLOW_SHA,
    checkout_sha: CHECKOUT_SHA,
    message:
      "npm returned an invalid exchange response for kandev (missing_token).",
  });
  assert.equal(emitted.includes(credentialSecret), false);
  assert.equal(emitted.includes(createdMetadata), false);
  assert.equal(emitted.includes(ignoredExpiry), false);
  assert.equal(emitted.includes(untrustedMetadata), false);
  assert.equal(emitted.includes("request-token-value"), false);
  assert.equal(emitted.includes(jwt), false);
  assert.equal(emitted.includes("expires_at"), false);

  const transportSecret = "transport-exception-secret";
  const failedTransport = `Date.now = () => ${FIXED_NOW}; let calls = 0; globalThis.fetch = async (input) => { calls += 1; if (calls > 1) throw new Error(${JSON.stringify(transportSecret)}); const target = String(input); const response = new Response(JSON.stringify({ value: ${JSON.stringify(jwt)} }), { status: 200, headers: { 'content-type': 'application/json' } }); Object.defineProperty(response, 'url', { value: target }); return response; };`;
  const transportResult = spawnSync(
    process.execPath,
    [
      "--import",
      `data:text/javascript,${encodeURIComponent(failedTransport)}`,
      fileURLToPath(new URL("./npm-auth-preflight.mjs", import.meta.url)),
      "--package",
      "kandev",
      "--checkout-sha",
      CHECKOUT_SHA,
    ],
    { encoding: "utf8", env: env() },
  );
  assert.equal(transportResult.status, 1);
  assert.equal(
    `${transportResult.stdout}\n${transportResult.stderr}`.includes(
      transportSecret,
    ),
    false,
  );
});

test("failure summaries retain safe publisher and OIDC recovery guidance", () => {
  const jwt = makeJwt();
  const authRejected = cliFetchBootstrap({
    oidcToken: jwt,
    exchangeStatus: 403,
    exchangeBody: { error: "sensitive-body" },
  });
  const script = fileURLToPath(
    new URL("./npm-auth-preflight.mjs", import.meta.url),
  );
  const publisherResult = spawnSync(
    process.execPath,
    [
      "--import",
      `data:text/javascript,${encodeURIComponent(authRejected)}`,
      script,
      "--package",
      "kandev",
      "--checkout-sha",
      CHECKOUT_SHA,
    ],
    { encoding: "utf8", env: env() },
  );
  const publisherOutput = `${publisherResult.stdout}\n${publisherResult.stderr}`;
  assert.equal(publisherResult.status, 1);
  assert.match(publisherOutput, /trusted publisher/);
  assert.match(publisherOutput, /kandev/);
  assert.equal(publisherOutput.includes("sensitive-body"), false);

  const oidcEnv = env({
    ACTIONS_ID_TOKEN_REQUEST_URL: undefined,
    ACTIONS_ID_TOKEN_REQUEST_TOKEN: undefined,
  });
  delete oidcEnv.ACTIONS_ID_TOKEN_REQUEST_URL;
  delete oidcEnv.ACTIONS_ID_TOKEN_REQUEST_TOKEN;
  const oidcResult = spawnSync(
    process.execPath,
    [script, "--package", "kandev", "--checkout-sha", CHECKOUT_SHA],
    { encoding: "utf8", env: oidcEnv },
  );
  assert.equal(oidcResult.status, 1);
  assert.match(`${oidcResult.stdout}\n${oidcResult.stderr}`, /id-token: write/);
});

test("failure summaries identify every protected setup condition without exposing input", () => {
  const cases = [
    ["GITHUB_ACTIONS", "invalid_github_actions"],
    ["GITHUB_REPOSITORY", "invalid_github_repository"],
    ["GITHUB_EVENT_NAME", "invalid_github_event_name"],
    ["GITHUB_REF", "invalid_github_ref"],
    ["GITHUB_WORKFLOW_REF", "invalid_github_workflow_ref"],
    ["RUNNER_ENVIRONMENT", "invalid_runner_environment"],
    ["SDK_SCOPE_OWNERSHIP_CONFIRMED", "invalid_sdk_scope_ownership_confirmed"],
  ];
  const script = fileURLToPath(
    new URL("./npm-auth-preflight.mjs", import.meta.url),
  );
  const secret = "untrusted-context-secret";
  for (const [setting, reason] of cases) {
    const result = spawnSync(
      process.execPath,
      [
        "--import",
        "data:text/javascript,globalThis.fetch=async()=>{throw new Error('network must not run')}",
        script,
        "--package",
        "kandev",
        "--checkout-sha",
        CHECKOUT_SHA,
      ],
      { encoding: "utf8", env: env({ [setting]: secret }) },
    );
    assert.equal(result.status, 1);
    const summary = JSON.parse(result.stdout.trim());
    assert.equal(summary.category, "release_context");
    assert.equal(summary.reason, reason);
    assert.ok(summary.message.includes(`${setting}=${APPROVED_ENV[setting]}`));
    assert.equal(`${result.stdout}\n${result.stderr}`.includes(secret), false);
  }
});

test("unsupported release context is rejected before a network request", async () => {
  const calls = [];
  const fetchImpl = async (input) => {
    calls.push(input);
    throw new Error("must not be called");
  };
  for (const override of [
    { RUNNER_ENVIRONMENT: "self-hosted" },
    { GITHUB_REPOSITORY: "other/repository" },
    { GITHUB_REF: "refs/heads/release" },
    { SDK_SCOPE_OWNERSHIP_CONFIRMED: "false" },
  ]) {
    await assert.rejects(
      runPreflight({
        packages: RUNTIME_PACKAGES,
        env: env(override),
        fetchImpl,
        now: () => FIXED_NOW,
        checkoutSha: CHECKOUT_SHA,
      }),
      /release|scope|workflow|runner/,
    );
  }
  assert.deepEqual(calls, []);
});

test("GitHub OIDC token failures are rejected before package exchange", async () => {
  for (const oidcBody of [
    { value: "not-a-jwt" },
    { value: makeJwt({ audience: "other" }) },
    { value: makeJwt({ expires: Math.floor(FIXED_NOW / 1000) - 60 }) },
    {},
  ]) {
    const transport = makeTransport({ oidcBody });
    await assert.rejects(
      runPreflight({
        packages: ["kandev"],
        env: env(),
        fetchImpl: transport.fetchImpl,
        now: () => FIXED_NOW,
        checkoutSha: CHECKOUT_SHA,
      }),
      (error) => {
        assert.equal(error.category, "release_context");
        return true;
      },
    );
    assert.equal(
      transport.calls.some((call) => call.target.includes("/exchange/")),
      false,
    );
  }
  await assert.rejects(
    runPreflight({
      packages: ["kandev"],
      env: env({ ACTIONS_ID_TOKEN_REQUEST_URL: "" }),
      fetchImpl: makeTransport().fetchImpl,
      now: () => FIXED_NOW,
      checkoutSha: CHECKOUT_SHA,
    }),
    (error) => {
      assert.equal(error.category, "release_context");
      assert.match(error.message, /id-token: write/);
      return true;
    },
  );
});

test("authentication requests reject redirects and remain bounded", async () => {
  assert.ok(Number.isFinite(REQUEST_TIMEOUT_MS));
  assert.ok(REQUEST_TIMEOUT_MS > 0 && REQUEST_TIMEOUT_MS <= 60_000);
  let observed;
  await assert.rejects(
    exchangePackageToken("kandev", makeJwt(), {
      fetchImpl: async (url, options) => {
        observed = { url, options };
        throw new Error("redirect rejected");
      },
    }),
    (error) => {
      assert.equal(error.package, "kandev");
      assert.match(error.message, /timed out or was redirected/);
      return true;
    },
  );
  assert.equal(observed.options.redirect, "error");
  assert.equal(observed.options.signal !== undefined, true);
  assert.equal(observed.options.method, "POST");
  assert.equal(
    observed.url,
    `${registryURL}/-/npm/v1/oidc/token/exchange/package/kandev`,
  );
});

test("requires exact workflow and checkout revisions", async () => {
  const transport = makeTransport();
  await assert.rejects(
    runPreflight({
      packages: RUNTIME_PACKAGES,
      env: env({ GITHUB_WORKFLOW_SHA: "" }),
      fetchImpl: transport.fetchImpl,
      now: () => FIXED_NOW,
      checkoutSha: CHECKOUT_SHA,
    }),
    /workflow revision/,
  );
  await assert.rejects(
    runPreflight({
      packages: RUNTIME_PACKAGES,
      env: env(),
      fetchImpl: transport.fetchImpl,
      now: () => FIXED_NOW,
      checkoutSha: "",
    }),
    /checkout revision/,
  );
  assert.equal(transport.calls.length, 0);
});

test("rejects an alternate SDK package name before any request", async () => {
  const transport = makeTransport();
  await assert.rejects(
    runPreflight({
      packages: ["kandev"],
      sdkPackage: "@other/plugin-sdk",
      env: env(),
      fetchImpl: transport.fetchImpl,
      now: () => FIXED_NOW,
      checkoutSha: CHECKOUT_SHA,
    }),
    (error) => {
      assert.equal(error.category, "release_context");
      assert.match(error.message, /canonical SDK package/);
      return true;
    },
  );
  assert.equal(transport.calls.length, 0);
});
