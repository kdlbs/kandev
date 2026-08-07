import { execFileSync, spawn, spawnSync } from "node:child_process";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { createServer as createTLSServer, type Server as TLSServer } from "node:https";
import { connect } from "node:net";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { pipeline, type Duplex, type Readable, type Writable } from "node:stream";

export const fixtureGitProvider = "fixture-source-control";
export const fixtureGitHost = "bitbucket.example.test";
export const fixtureGitPath = "/scm/TEAM/fixture.git";
export const fixtureGitURL = `https://${fixtureGitHost}${fixtureGitPath}`;
export const fixtureGitUser = "fixture-user";
export const fixtureGitSecret = "fixture-credential-secret";

const fixturePluginID = "kandev-plugin-e2e";
const fixturePluginPackage = path.resolve(
  __dirname,
  "../../../../backend/.build/kandev-plugin-e2e-1.0.0.tar.gz",
);

type GitHTTPFixture = {
  checkoutPath: string;
  proxyURL: string;
  profileGitEnv: Array<{ key: string; value: string }>;
  requestPaths: () => string[];
  requestHosts: () => string[];
  pushed: () => boolean;
  close: () => Promise<void>;
};

/**
 * A real HTTPS Git server which insists on Basic auth, reached through a
 * bridge-visible CONNECT proxy. The remote URL remains the fixture provider's
 * exact HTTPS identity, so the credential helper receives the host/path that
 * its broker lease was issued for rather than a test-server substitute.
 */
export async function startPluginGitFixture(root: string): Promise<GitHTTPFixture> {
  const repoRoot = path.join(root, "credential-git");
  const bareRepo = path.join(repoRoot, fixtureGitPath);
  const checkoutPath = path.join(root, "credential-git-checkout");
  fs.mkdirSync(path.dirname(bareRepo), { recursive: true });
  fs.mkdirSync(checkoutPath, { recursive: true });
  execFileSync("git", ["init", "--bare", "-b", "main", bareRepo]);
  execFileSync("git", ["init", "-b", "main"], { cwd: checkoutPath });
  fs.writeFileSync(path.join(checkoutPath, "README.md"), "credential fixture\n");
  execFileSync("git", ["add", "."], { cwd: checkoutPath });
  execFileSync(
    "git",
    ["-c", "user.name=E2E", "-c", "user.email=e2e@test.local", "commit", "-m", "initial"],
    { cwd: checkoutPath },
  );
  execFileSync("git", ["remote", "add", "origin", bareRepo], { cwd: checkoutPath });
  execFileSync("git", ["push", "origin", "main"], { cwd: checkoutPath });
  execFileSync("git", ["remote", "set-url", "origin", fixtureGitURL], { cwd: checkoutPath });

  const certDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-git-cert-"));
  const keyPath = path.join(certDir, "key.pem");
  const certPath = path.join(certDir, "cert.pem");
  execFileSync(
    "openssl",
    [
      "req",
      "-x509",
      "-newkey",
      "rsa:2048",
      "-nodes",
      "-keyout",
      keyPath,
      "-out",
      certPath,
      "-subj",
      `/CN=${fixtureGitHost}`,
      "-days",
      "1",
    ],
    { stdio: process.env.E2E_DEBUG ? "inherit" : "ignore" },
  );

  const requestPaths: string[] = [];
  const requestHosts: string[] = [];
  const gitServer = createTLSServer(
    { key: fs.readFileSync(keyPath), cert: fs.readFileSync(certPath) },
    (req, res) => {
      requestPaths.push(new URL(req.url ?? "/", "https://fixture").pathname);
      requestHosts.push(req.headers.host ?? "");
      if (!requestHasFixtureAuth(req)) {
        res.writeHead(401, { "www-authenticate": 'Basic realm="fixture"' }).end();
        return;
      }
      serveGitHTTP(req, res, repoRoot);
    },
  );
  const gitPort = await listen(gitServer);
  const proxy = createFixtureConnectProxy(gitPort);
  const proxyPort = await listen(proxy);
  const gateway = dockerBridgeGateway();
  const proxyURL = `http://${gateway}:${proxyPort}`;

  return {
    checkoutPath,
    proxyURL,
    profileGitEnv: [
      { key: "GIT_CONFIG_COUNT", value: "2" },
      { key: "GIT_CONFIG_KEY_0", value: "http.proxy" },
      { key: "GIT_CONFIG_VALUE_0", value: proxyURL },
      { key: "GIT_CONFIG_KEY_1", value: "http.sslVerify" },
      { key: "GIT_CONFIG_VALUE_1", value: "false" },
    ],
    requestPaths: () => [...requestPaths],
    requestHosts: () => [...requestHosts],
    pushed: () =>
      execFileSync("git", ["--git-dir", bareRepo, "log", "-1", "--format=%s"], {
        encoding: "utf8",
      }).trim() === "credential fixture push",
    close: async () => {
      await closeServer(proxy);
      await closeServer(gitServer);
      fs.rmSync(certDir, { recursive: true, force: true });
    },
  };
}

export function credentialBrokerPublicURL(backendPort: number): string | undefined {
  const raw = process.env.KANDEV_E2E_CREDENTIAL_BROKER_PUBLIC_BASE_URL;
  return raw?.trim().replace("{port}", String(backendPort)) || undefined;
}

export async function installFixturePlugin(baseURL: string): Promise<void> {
  if (!fs.existsSync(fixturePluginPackage)) {
    throw new Error(`fixture plugin package missing: ${fixturePluginPackage}`);
  }
  const response = await fetch(`${baseURL}/api/plugins/install`, {
    method: "POST",
    body: (() => {
      const form = new FormData();
      form.append(
        "package",
        new Blob([fs.readFileSync(fixturePluginPackage)], { type: "application/gzip" }),
        path.basename(fixturePluginPackage),
      );
      return form;
    })(),
  });
  if (!response.ok) throw new Error(`fixture plugin install failed (${response.status})`);
}

export async function uninstallFixturePlugin(baseURL: string): Promise<void> {
  await fetch(`${baseURL}/api/plugins/${fixturePluginID}`, { method: "DELETE" });
}

export async function revokeFixtureConnection(
  baseURL: string,
  workspaceId: string,
): Promise<Response> {
  return fetch(`${baseURL}/api/plugins/${fixturePluginID}/actions/connection-status`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ workspaceId, body: { revoke: true } }),
  });
}

export function assertExactFixtureTransport(fixture: GitHTTPFixture): void {
  const paths = fixture.requestPaths();
  const hosts = fixture.requestHosts();
  if (paths.length === 0 || paths.some((requestPath) => !requestPath.startsWith(fixtureGitPath))) {
    throw new Error("Git did not request the exact fixture repository path");
  }
  if (hosts.some((host) => host !== fixtureGitHost)) {
    throw new Error("Git did not preserve the exact fixture repository host");
  }
}

export function runDockerGit(
  containerID: string,
  command: string,
): { output: string; status: number } {
  const result = spawnSync("docker", ["exec", containerID, "sh", "-lc", command], {
    encoding: "utf8",
  });
  return { output: `${result.stdout ?? ""}${result.stderr ?? ""}`, status: result.status ?? 1 };
}

export function runRemoteGit(
  agentctlPID: number,
  command: string,
  targetName: string,
): {
  output: string;
  status: number;
} {
  const result = spawnSync(
    "docker",
    [
      "exec",
      targetName,
      "sh",
      "-lc",
      `xargs -0 sh -c 'env "$@" sh -lc "$0"' ${shellQuote(command)} < /proc/${agentctlPID}/environ`,
    ],
    { encoding: "utf8" },
  );
  return { output: `${result.stdout ?? ""}${result.stderr ?? ""}`, status: result.status ?? 1 };
}

function requestHasFixtureAuth(req: IncomingMessage): boolean {
  const expected = `Basic ${Buffer.from(`${fixtureGitUser}:${fixtureGitSecret}`).toString("base64")}`;
  return req.headers.authorization === expected;
}

function serveGitHTTP(req: IncomingMessage, res: ServerResponse, root: string): void {
  const parsed = new URL(req.url ?? "/", "https://fixture");
  const child = spawn("git", ["http-backend"], {
    env: {
      ...process.env,
      GIT_PROJECT_ROOT: root,
      GIT_HTTP_EXPORT_ALL: "1",
      PATH_INFO: parsed.pathname,
      QUERY_STRING: parsed.search.slice(1),
      REQUEST_METHOD: req.method ?? "GET",
      CONTENT_TYPE: req.headers["content-type"] ?? "",
      CONTENT_LENGTH: req.headers["content-length"] ?? "",
      REMOTE_USER: fixtureGitUser,
    },
  });
  const chunks: Buffer[] = [];
  let failed = false;
  child.stdout.on("data", (chunk: Buffer) => chunks.push(chunk));
  child.once("error", () => {
    failed = true;
    res.writeHead(500).end();
  });
  child.once("close", () => {
    if (!failed) writeCGIResponse(res, Buffer.concat(chunks));
  });
  pipeFixtureGitRequest(req, child.stdin, () => {
    if (failed) return;
    failed = true;
    child.kill();
    if (!res.destroyed) res.destroy();
  });
}

/** Pipes an HTTP request into git-http-backend with deterministic EPIPE handling. */
export function pipeFixtureGitRequest(
  request: Readable,
  childInput: Writable,
  onError: (error: Error) => void,
): void {
  pipeline(request, childInput, (error) => {
    if (error) onError(error);
  });
}

function writeCGIResponse(res: ServerResponse, body: Buffer): void {
  const splitAt = body.indexOf("\r\n\r\n");
  if (splitAt < 0) {
    res.writeHead(500).end();
    return;
  }
  let status = 200;
  const headers: Record<string, string> = {};
  for (const line of body.subarray(0, splitAt).toString().split("\r\n")) {
    const [name, value] = line.split(/:\s*/, 2);
    if (name.toLowerCase() === "status") status = Number(value?.split(" ")[0]) || 500;
    else if (name && value) headers[name] = value;
  }
  res.writeHead(status, headers).end(body.subarray(splitAt + 4));
}

function createFixtureConnectProxy(gitPort: number): Server {
  const proxy = createServer((_, res) => res.writeHead(405).end());
  proxy.on("connect", (_req, client, head) => {
    const upstream = connect(gitPort, "127.0.0.1", () => {
      client.write("HTTP/1.1 200 Connection Established\r\n\r\n");
      if (head.length > 0) upstream.write(head);
      bridgeFixtureConnectStreams(client, upstream);
    });
    bindFixtureConnectErrors(client, upstream);
  });
  return proxy;
}

export function bindFixtureConnectErrors(client: Duplex, upstream: Duplex): void {
  const destroyBoth = () => destroyFixtureConnectStreams(client, upstream);
  client.once("error", destroyBoth);
  upstream.once("error", destroyBoth);
}

export function bridgeFixtureConnectStreams(client: Duplex, upstream: Duplex): void {
  const destroyBoth = () => destroyFixtureConnectStreams(client, upstream);
  pipeline(client, upstream, destroyBoth);
  pipeline(upstream, client, destroyBoth);
}

function destroyFixtureConnectStreams(client: Duplex, upstream: Duplex): void {
  client.destroy();
  upstream.destroy();
}

function dockerBridgeGateway(): string {
  const result = spawnSync(
    "docker",
    ["network", "inspect", "bridge", "-f", "{{(index .IPAM.Config 0).Gateway}}"],
    { encoding: "utf8" },
  );
  const gateway = result.status === 0 ? result.stdout.trim() : "";
  if (!gateway) throw new Error("could not determine Docker bridge gateway");
  return gateway;
}

function listen(server: Server | TLSServer): Promise<number> {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "0.0.0.0", () => {
      server.off("error", reject);
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("fixture server did not receive a TCP port"));
        return;
      }
      resolve(address.port);
    });
  });
}

function closeServer(server: Server | TLSServer): Promise<void> {
  return new Promise((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
}

function shellQuote(value: string): string {
  return `'${value.replaceAll("'", "'\\''")}'`;
}
