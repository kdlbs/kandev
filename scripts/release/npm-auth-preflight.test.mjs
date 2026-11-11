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
  expires = Math.floor(Date.now() / 1000) + 3600,
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
  exchangeBody = null,
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
      return jsonResponse(
        201,
        exchangeBody ?? {
          token_type: "oidc",
          token: `npm_${"a".repeat(36)}`,
          created: new Date().toISOString(),
          expires: new Date(Date.now() + 3_600_000).toISOString(),
        },
        target,
      );
    }
    throw new Error(`unexpected request: ${target}`);
  };
  return { fetchImpl, calls, exchanges };
}

test("one rejected publisher blocks the whole preflight before any release mutation", async () => {
  const transport = makeTransport({
    failPackage: "@kdlbs/runtime-linux-arm64",
    exchangeStatus: 403,
  });
  await assert.rejects(
    runPreflight({
      packages: RUNTIME_PACKAGES,
      env: env(),
      fetchImpl: transport.fetchImpl,
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

test("confirmed whole-package SDK absence defers only the SDK check", async () => {
  const transport = makeTransport({ sdkExists: false });
  const summary = await runPreflight({
    packages: RUNTIME_PACKAGES,
    env: env(),
    fetchImpl: transport.fetchImpl,
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

test("rejected, malformed, missing, and expired exchange responses fail safely", async () => {
  const cases = [
    { exchangeStatus: 500, failPackage: "kandev" },
    {
      exchangeBody: {
        token_type: "oidc",
        token: "",
        created: new Date().toISOString(),
        expires: new Date(Date.now() + 3_600_000).toISOString(),
      },
    },
    {
      exchangeBody: {
        token_type: "oidc",
        token: `npm_${"b".repeat(36)}`,
        created: new Date().toISOString(),
        expires: new Date(Date.now() - 1_000).toISOString(),
      },
    },
    {
      exchangeBody: {
        token_type: "other",
        token: `npm_${"b".repeat(36)}`,
        created: new Date().toISOString(),
        expires: new Date(Date.now() + 3_600_000).toISOString(),
      },
    },
  ];
  for (const options of cases) {
    const transport = makeTransport(options);
    await assert.rejects(
      runPreflight({
        packages: ["kandev"],
        env: env(),
        fetchImpl: transport.fetchImpl,
        checkoutSha: CHECKOUT_SHA,
      }),
      (error) => {
        assert.equal(error.package, "kandev");
        assert.equal(
          error.category,
          options.exchangeStatus || error.reason === "expired_token"
            ? "npm_authentication"
            : "npm_exchange_response",
        );
        assert.equal(error.message.includes("sensitive-server-body"), false);
        assert.equal(error.message.includes(`npm_${"b".repeat(36)}`), false);
        return true;
      },
    );
  }
});

test("exchange validation reports fixed reasons for each response condition", async () => {
  const cases = [
    { body: null, reason: "invalid_body_shape" },
    { body: [], reason: "invalid_body_shape" },
    {
      body: { token_type: "other", token: "x", created: "1", expires: "2" },
      reason: "invalid_token_type",
    },
    {
      body: { token_type: "oidc", token: "", created: "1", expires: "2" },
      reason: "missing_token",
    },
    {
      body: { token_type: "oidc", created: "1", expires: "2" },
      reason: "missing_token",
    },
    {
      body: { token_type: "oidc", token: 4, created: "1", expires: "2" },
      reason: "missing_token",
    },
    {
      body: { token_type: "oidc", token: "x", created: null, expires: "2" },
      reason: "invalid_created",
    },
    {
      body: { token_type: "oidc", token: "x", created: "bad", expires: "2" },
      reason: "invalid_created",
    },
    {
      body: { token_type: "oidc", token: "x", created: {}, expires: "2" },
      reason: "invalid_created",
    },
    {
      body: { token_type: "oidc", token: "x", created: "1", expires: "bad" },
      reason: "invalid_expires",
    },
    {
      body: { token_type: "oidc", token: "x", created: "1", expires: null },
      reason: "invalid_expires",
    },
    {
      body: {
        token_type: "oidc",
        token: "x",
        created: "1",
        expires: new Date(1000).toISOString(),
      },
      reason: "expired_token",
      category: "npm_authentication",
    },
  ];
  for (const { body, reason, category = "npm_exchange_response" } of cases) {
    const fetchImpl = async (input) => ({
      status: 201,
      url: String(input),
      json: async () => body,
    });
    await assert.rejects(
      exchangePackageToken("kandev", "oidc", { fetchImpl, now: () => 1000 }),
      (error) => {
        assert.equal(error.status, 201);
        assert.equal(error.category, category);
        assert.equal(error.reason, reason);
        if (reason === "expired_token") {
          assert.match(error.message, /trusted publisher/);
          assert.match(error.message, /kandev/);
        }
        return true;
      },
    );
  }
  await assert.rejects(
    exchangePackageToken("kandev", "oidc", {
      fetchImpl: async (input) => ({
        status: 201,
        url: String(input),
        json: async () => {
          throw new Error("untrusted-body-secret");
        },
      }),
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

test("failure summary serialization includes safe context and excludes untrusted metadata", () => {
  const secret = "npm_sentinel_secret_123";
  const payload = Buffer.from(
    JSON.stringify({ aud: OIDC_AUDIENCE, exp: 4_102_444_800 }),
  ).toString("base64url");
  const jwt = `e30.${payload}.sig`;
  const bootstrap = `globalThis.fetch = async (input) => ({ status: String(input).includes('actions.test') ? 200 : 201, url: String(input), json: async () => String(input).includes('actions.test') ? { value: ${JSON.stringify(jwt)} } : { token_type: 'oidc', token: 'valid', expires: '2030-01-01', malicious: ${JSON.stringify(secret)} } });`;
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
  assert.match(emitted, /"category":"npm_exchange_response"/);
  assert.match(emitted, /"http_status":201/);
  assert.match(emitted, /"reason":"invalid_created"/);
  assert.match(emitted, new RegExp(WORKFLOW_SHA));
  assert.match(emitted, new RegExp(CHECKOUT_SHA));
  assert.equal(emitted.includes(secret), false);

  const transportSecret = "transport-exception-secret";
  const failedTransport = `let calls = 0; globalThis.fetch = async (input) => { calls += 1; if (calls > 1) throw new Error(${JSON.stringify(transportSecret)}); return { status: 200, url: String(input), json: async () => ({ value: ${JSON.stringify(jwt)} }) }; };`;
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
  const payload = Buffer.from(
    JSON.stringify({ aud: OIDC_AUDIENCE, exp: 4_102_444_800 }),
  ).toString("base64url");
  const jwt = `e30.${payload}.sig`;
  const authRejected = `let calls = 0; globalThis.fetch = async (input) => { calls += 1; return calls === 1 ? { status: 200, url: String(input), json: async () => ({ value: ${JSON.stringify(jwt)} }) } : { status: 403, url: String(input), json: async () => ({ error: 'sensitive-body' }) }; };`;
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

test("HTTP 201 with a missing creation date reports invalid_created instead of publisher rejection", async () => {
  const transport = makeTransport({
    exchangeBody: {
      token_type: "oidc",
      token: `npm_${"c".repeat(36)}`,
      expires: new Date(Date.now() + 3_600_000).toISOString(),
    },
  });
  await assert.rejects(
    runPreflight({
      packages: ["kandev"],
      env: env(),
      fetchImpl: transport.fetchImpl,
      checkoutSha: CHECKOUT_SHA,
    }),
    (error) => {
      assert.equal(error.status, 201);
      assert.equal(error.category, "npm_exchange_response");
      assert.equal(error.reason, "invalid_created");
      return true;
    },
  );
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
    { value: makeJwt({ expires: Math.floor(Date.now() / 1000) - 60 }) },
    {},
  ]) {
    const transport = makeTransport({ oidcBody });
    await assert.rejects(
      runPreflight({
        packages: ["kandev"],
        env: env(),
        fetchImpl: transport.fetchImpl,
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
      checkoutSha: CHECKOUT_SHA,
    }),
    /workflow revision/,
  );
  await assert.rejects(
    runPreflight({
      packages: RUNTIME_PACKAGES,
      env: env(),
      fetchImpl: transport.fetchImpl,
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
