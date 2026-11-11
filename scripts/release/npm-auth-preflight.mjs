import { appendFile } from "node:fs/promises";
import { parseArgs } from "node:util";
import { pathToFileURL } from "node:url";
import { resolve } from "node:path";
import { assertReleaseContext, ReleaseContextError } from "./sdk-runtime.mjs";
import {
  packageName as sdkPackageName,
  publicRegistryURL,
  readPackage,
  registryURL,
} from "./sdk-registry.mjs";

export const OIDC_AUDIENCE = "npm:registry.npmjs.org";
export const OIDC_EXCHANGE_PATH = "/-/npm/v1/oidc/token/exchange/package/";
export const REQUEST_TIMEOUT_MS = 20_000;

const JWT_PATTERN = /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;

export class NpmAuthPreflightError extends Error {
  constructor(
    packageName,
    message,
    {
      category = "npm_authentication",
      status = null,
      reason = null,
      cause,
    } = {},
  ) {
    super(message, cause ? { cause } : undefined);
    this.name = "NpmAuthPreflightError";
    this.code = "NPM_AUTH_PREFLIGHT";
    this.package = packageName;
    this.category = category;
    this.status = status;
    this.reason = reason;
  }
}

function decodeJwtPayload(token) {
  const [, payload] = token.split(".");
  return JSON.parse(Buffer.from(payload, "base64url").toString("utf8"));
}

function authenticationGuidance(packageName, status) {
  const reason =
    status === 404
      ? "package not found or trusted publisher is not configured"
      : `HTTP ${status}`;
  return `npm rejected authentication for ${packageName} (${reason}). Configure this package's trusted publisher for kdlbs/kandev, workflow release.yml, environment release, and the required direct-publish and dist-tag actions.`;
}

export async function requestGitHubOidcToken({
  env = process.env,
  fetchImpl = fetch,
  now = Date.now,
} = {}) {
  const requestURL = env.ACTIONS_ID_TOKEN_REQUEST_URL;
  const requestToken = env.ACTIONS_ID_TOKEN_REQUEST_TOKEN;
  if (!requestURL || !requestToken)
    throw new NpmAuthPreflightError(
      null,
      "GitHub OIDC token request is unavailable; grant id-token: write to the preflight job.",
      { category: "release_context" },
    );
  let url;
  try {
    url = new URL(requestURL);
  } catch {
    throw new NpmAuthPreflightError(
      null,
      "GitHub OIDC token request URL is invalid.",
      { category: "release_context" },
    );
  }
  url.searchParams.set("audience", OIDC_AUDIENCE);
  let response;
  try {
    response = await fetchImpl(url, {
      headers: {
        authorization: `Bearer ${requestToken}`,
        accept: "application/json",
      },
      redirect: "error",
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });
  } catch (error) {
    throw new NpmAuthPreflightError(
      null,
      "GitHub OIDC token request failed or was redirected.",
      { category: "release_context", cause: error },
    );
  }
  if (response.status !== 200)
    throw new NpmAuthPreflightError(
      null,
      `GitHub OIDC token request was rejected with HTTP ${response.status}.`,
      { category: "release_context", status: response.status },
    );
  let body;
  try {
    body = await response.json();
  } catch {
    throw new NpmAuthPreflightError(
      null,
      "GitHub OIDC token response was unreadable.",
      { category: "release_context" },
    );
  }
  const token = body?.value;
  if (typeof token !== "string" || !JWT_PATTERN.test(token))
    throw new NpmAuthPreflightError(
      null,
      "GitHub OIDC token response was malformed.",
      { category: "release_context" },
    );
  let claims;
  try {
    claims = decodeJwtPayload(token);
  } catch {
    throw new NpmAuthPreflightError(
      null,
      "GitHub OIDC token claims were unreadable.",
      { category: "release_context" },
    );
  }
  const audiences = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
  if (!audiences.includes(OIDC_AUDIENCE))
    throw new NpmAuthPreflightError(
      null,
      `GitHub OIDC token audience must be ${OIDC_AUDIENCE}.`,
      { category: "release_context" },
    );
  if (typeof claims.exp !== "number" || claims.exp * 1000 <= now())
    throw new NpmAuthPreflightError(
      null,
      "GitHub OIDC token is missing an expiry or has expired.",
      { category: "release_context" },
    );
  return token;
}

export async function exchangePackageToken(
  packageName,
  idToken,
  { fetchImpl = fetch, now = Date.now } = {},
) {
  const url = `${registryURL}${OIDC_EXCHANGE_PATH}${encodeURIComponent(packageName)}`;
  let response;
  try {
    response = await fetchImpl(url, {
      method: "POST",
      headers: {
        authorization: `Bearer ${idToken}`,
        accept: "application/json",
      },
      redirect: "error",
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });
  } catch (error) {
    throw new NpmAuthPreflightError(
      packageName,
      `npm authentication request failed for ${packageName}; the request timed out or was redirected. Configure this package's trusted publisher and retry the release.`,
      { cause: error },
    );
  }
  if (response.url) {
    try {
      publicRegistryURL(response.url);
    } catch (error) {
      throw new NpmAuthPreflightError(
        packageName,
        `npm authentication for ${packageName} left the public registry.`,
        { cause: error },
      );
    }
  }
  if (response.status !== 201)
    throw new NpmAuthPreflightError(
      packageName,
      authenticationGuidance(packageName, response.status),
      { status: response.status, reason: "authentication_rejected" },
    );
  let state;
  try {
    state = await response.json();
  } catch {
    throw new NpmAuthPreflightError(
      packageName,
      `npm returned an unreadable exchange response for ${packageName}.`,
      {
        category: "npm_exchange_response",
        status: response.status,
        reason: "unreadable_json",
      },
    );
  }
  const invalidResponse = (reason) =>
    new NpmAuthPreflightError(
      packageName,
      `npm returned an invalid exchange response for ${packageName} (${reason}).`,
      { category: "npm_exchange_response", status: response.status, reason },
    );
  if (!state || typeof state !== "object" || Array.isArray(state))
    throw invalidResponse("invalid_body_shape");
  if (state.token_type !== "oidc") throw invalidResponse("invalid_token_type");
  if (typeof state.token !== "string" || state.token.length === 0)
    throw invalidResponse("missing_token");
  const parseDate = (value) => {
    try {
      return Date.parse(value);
    } catch {
      return Number.NaN;
    }
  };
  const expires = parseDate(state?.expires);
  if (!Number.isFinite(expires)) throw invalidResponse("invalid_expires");
  if (expires <= now())
    throw new NpmAuthPreflightError(
      packageName,
      `npm returned an expired exchange token for ${packageName}. Configure this package's trusted publisher and retry the release.`,
      { status: response.status, reason: "expired_token" },
    );
  return {
    package: packageName,
    status: "authenticated",
    expires_at: new Date(expires).toISOString(),
  };
}

async function checkSdkPublisher({
  sdkPackage,
  env,
  fetchImpl,
  readPackageImpl,
  now,
}) {
  let state;
  try {
    state = await readPackageImpl("0.0.0", fetchImpl);
  } catch (error) {
    throw new NpmAuthPreflightError(
      sdkPackage,
      `npm registry read failed for ${sdkPackage}; only a confirmed package absence may defer its early authentication check.`,
      { category: "registry_read", cause: error },
    );
  }
  if (!state.packageExists)
    return {
      package: sdkPackage,
      status: "deferred",
      reason: "package_absent",
    };
  const token = await requestGitHubOidcToken({ env, fetchImpl, now });
  return exchangePackageToken(sdkPackage, token, { fetchImpl, now });
}

export async function runPreflight({
  packages,
  sdkPackage = sdkPackageName,
  env = process.env,
  fetchImpl = fetch,
  readPackageImpl = readPackage,
  now = Date.now,
  checkoutSha,
} = {}) {
  try {
    assertReleaseContext(env);
  } catch (error) {
    const knownContext = error instanceof ReleaseContextError;
    throw new NpmAuthPreflightError(
      null,
      knownContext
        ? `npm authentication preflight requires release context ${error.setting}=${error.expected}.`
        : "npm authentication preflight requires the protected release workflow context on GitHub-hosted runners.",
      {
        category: "release_context",
        reason: knownContext
          ? `invalid_${error.setting.toLowerCase()}`
          : "invalid_release_context",
      },
    );
  }
  if (!Array.isArray(packages) || packages.length === 0)
    throw new NpmAuthPreflightError(
      null,
      "npm authentication preflight requires at least one runtime package.",
      { category: "release_context" },
    );
  if (packages.some((name) => typeof name !== "string" || !name.trim()))
    throw new NpmAuthPreflightError(
      null,
      "npm authentication preflight received an invalid package name.",
      { category: "release_context" },
    );
  if (typeof sdkPackage !== "string" || !sdkPackage.trim())
    throw new NpmAuthPreflightError(
      null,
      "npm authentication preflight received an invalid SDK package name.",
      { category: "release_context" },
    );
  if (sdkPackage !== sdkPackageName)
    throw new NpmAuthPreflightError(
      sdkPackage,
      `npm authentication preflight only supports the canonical SDK package ${sdkPackageName}.`,
      { category: "release_context" },
    );
  if (!/^[0-9a-f]{40}$/.test(checkoutSha ?? ""))
    throw new NpmAuthPreflightError(
      null,
      "npm authentication preflight requires the exact checkout revision.",
      { category: "release_context" },
    );
  if (!/^[0-9a-f]{40}$/.test(env.GITHUB_WORKFLOW_SHA ?? ""))
    throw new NpmAuthPreflightError(
      null,
      "npm authentication preflight requires the exact workflow revision.",
      { category: "release_context" },
    );
  const results = [];
  for (const packageName of [...new Set(packages)]) {
    const token = await requestGitHubOidcToken({ env, fetchImpl, now });
    results.push(
      await exchangePackageToken(packageName, token, { fetchImpl, now }),
    );
  }
  results.push(
    await checkSdkPublisher({
      sdkPackage,
      env,
      fetchImpl,
      readPackageImpl,
      now,
    }),
  );
  return {
    status: "passed",
    workflow_sha: env.GITHUB_WORKFLOW_SHA,
    checkout_sha: checkoutSha,
    packages: results,
  };
}

async function writePreflightSummary(summary, env) {
  const text = JSON.stringify(summary, null, 2);
  if (env.GITHUB_STEP_SUMMARY)
    await appendFile(
      env.GITHUB_STEP_SUMMARY,
      `\n### npm authentication preflight\n\n\`\`\`json\n${text}\n\`\`\`\n`,
    );
  console.log(JSON.stringify(summary));
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  const { values } = parseArgs({
    options: {
      package: { type: "string", multiple: true, default: [] },
      "sdk-package": { type: "string" },
      "checkout-sha": { type: "string" },
    },
  });
  try {
    const summary = await runPreflight({
      packages: values.package,
      sdkPackage: values["sdk-package"] ?? sdkPackageName,
      checkoutSha: values["checkout-sha"],
    });
    await writePreflightSummary(summary, process.env);
  } catch (error) {
    const summary = {
      status: "failed",
      package: error?.package ?? null,
      category: error?.category ?? "npm_authentication",
      http_status: Number.isInteger(error?.status) ? error.status : null,
      reason: typeof error?.reason === "string" ? error.reason : null,
      workflow_sha: /^[0-9a-f]{40}$/.test(process.env.GITHUB_WORKFLOW_SHA ?? "")
        ? process.env.GITHUB_WORKFLOW_SHA
        : null,
      checkout_sha: /^[0-9a-f]{40}$/.test(values["checkout-sha"] ?? "")
        ? values["checkout-sha"]
        : null,
      message:
        error instanceof NpmAuthPreflightError
          ? error.message
          : "npm authentication preflight failed before package authentication completed.",
    };
    await writePreflightSummary(summary, process.env).catch(() => {});
    console.error(JSON.stringify(summary));
    process.exitCode = 1;
  }
}
