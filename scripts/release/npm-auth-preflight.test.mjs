import assert from "node:assert/strict";
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
  const header = Buffer.from(JSON.stringify({ alg: "none", typ: "JWT" }))
    .toString("base64url");
  const payload = Buffer.from(JSON.stringify({ aud: audience, exp: expires }))
    .toString("base64url");
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
        return jsonResponse(sdkStatus, { error: "registry unavailable" }, target);
      return jsonResponse(
        200,
        { name: sdkPackageName, versions: {}, "dist-tags": {} },
        target,
      );
    }
    if (target.startsWith(`${registryURL}/-/npm/v1/oidc/token/exchange/package/`)) {
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
  assert.equal(
    summary.packages.at(-1).package,
    sdkPackageName,
  );
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
    transport.exchanges.some(
      (exchange) => exchange.package === sdkPackageName,
    ),
    false,
  );
  assert.equal(
    transport.calls.filter((call) => call.target.includes("/exchange/"))
      .length,
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
    transport.exchanges.some(
      (exchange) => exchange.package === sdkPackageName,
    ),
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
        assert.equal(error.category, "npm_authentication");
        assert.equal(error.message.includes("sensitive-server-body"), false);
        assert.equal(error.message.includes(`npm_${"b".repeat(36)}`), false);
        return true;
      },
    );
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
  assert.equal(observed.url, `${registryURL}/-/npm/v1/oidc/token/exchange/package/kandev`);
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
