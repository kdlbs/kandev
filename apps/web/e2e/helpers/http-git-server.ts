import { execFileSync, spawn, spawnSync } from "node:child_process";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import fs from "node:fs";
import path from "node:path";

type HTTPGitFixture = {
  remoteURL: string;
  checkoutPath: string;
  /**
   * Backend-only config for fixture consumers whose Git path honors global
   * configuration. Managed workspace clones deliberately sanitize it.
   */
  backendEnv: Record<string, string>;
  /**
   * Test-executor-only Git configuration that rewrites the trusted public
   * origin to this fixture's bridge-reachable HTTP server.
   */
  gitConfigEnvVars: Array<{ key: string; value: string }>;
  close: () => Promise<void>;
};

export type HTTPGitFixtureOptions = {
  /** Override the Docker bridge address for direct host-side fixture tests. */
  bridgeGateway?: string;
  onListening?: (server: Server, port: number) => void;
  writeBackendGitConfig?: (file: string, content: string) => void;
  closeServer?: (server: Server) => Promise<void>;
};

/**
 * Serves a disposable bare repository from the Docker bridge gateway. This
 * exercises the same HTTP clone path used by Docker and SSH executors without
 * relying on an external provider or developer checkout.
 */
export async function startHTTPGitFixture(
  root: string,
  name: string,
  options: HTTPGitFixtureOptions = {},
): Promise<HTTPGitFixture> {
  const remoteDir = path.join(root, "fixture", `${name}.git`);
  const checkout = path.join(root, `${name}-checkout`);
  fs.mkdirSync(checkout, { recursive: true });
  execFileSync("git", ["init", "--bare", "-b", "main", remoteDir]);
  // The fixture is served as static (dumb) HTTP. Keep its advertised refs in
  // sync when E2E tests push additional commits before launching a task.
  const postUpdateHook = path.join(remoteDir, "hooks", "post-update");
  fs.writeFileSync(postUpdateHook, "#!/bin/sh\nexec git update-server-info\n");
  fs.chmodSync(postUpdateHook, 0o755);
  execFileSync("git", ["init", "-b", "main"], { cwd: checkout });
  fs.writeFileSync(path.join(checkout, "remote-source.txt"), `${name} fixture\n`);
  execFileSync("git", ["add", "."], { cwd: checkout });
  execFileSync(
    "git",
    ["-c", "user.name=E2E Test", "-c", "user.email=e2e@test.local", "commit", "-m", "fixture"],
    { cwd: checkout },
  );
  execFileSync("git", ["remote", "add", "origin", remoteDir], { cwd: checkout });
  execFileSync("git", ["push", "origin", "main"], { cwd: checkout });
  execFileSync("git", ["--git-dir", remoteDir, "update-server-info"]);

  const server = createStaticGitServer(root);
  const port = await listen(server);
  try {
    options.onListening?.(server, port);
    const fixtureOrigin = `http://${options.bridgeGateway ?? dockerBridgeGateway()}:${port}/`;
    const remoteURL = `https://gitlab.com/fixture/${name}.git`;
    execFileSync("git", ["remote", "set-url", "origin", remoteURL], { cwd: checkout });
    const backendGitConfigPath = path.join(root, "fixture", `${name}.gitconfig`);
    const config = `[url "${fixtureOrigin}fixture/${name}.git"]\n\tinsteadOf = ${remoteURL}\n`;
    (options.writeBackendGitConfig ?? fs.writeFileSync)(backendGitConfigPath, config);
    return {
      // The source endpoint must receive the real GitLab identity so the
      // production trusted-origin validation remains exercised. Disposable test
      // executor profiles and their isolated backend fixture rewrite Git's clone
      // transport to this local HTTP server.
      remoteURL,
      checkoutPath: checkout,
      backendEnv: { GIT_CONFIG_GLOBAL: backendGitConfigPath },
      gitConfigEnvVars: [
        { key: "GIT_CONFIG_COUNT", value: "1" },
        { key: "GIT_CONFIG_KEY_0", value: `url.${fixtureOrigin}.insteadOf` },
        { key: "GIT_CONFIG_VALUE_0", value: "https://gitlab.com/" },
      ],
      // Leave the config under the backend fixture root until that fixture has
      // released its environment and stopped its process, then removes the root.
      close: () => closeServer(server),
    };
  } catch (setupError) {
    try {
      await (options.closeServer ?? closeServer)(server);
    } catch (closeError) {
      throw new AggregateError(
        [setupError, closeError],
        "HTTP Git fixture setup failed and its server did not close",
      );
    }
    throw setupError;
  }
}

function createStaticGitServer(root: string): Server {
  return createServer(async (request, response) => {
    const pathname = decodeURIComponent(new URL(request.url ?? "/", "http://fixture").pathname);
    const relative = pathname.replace(/^\/+/, "");
    if (await serveSmartGitRequest(root, relative, request, response)) return;

    const file = path.resolve(root, relative);
    if (file !== root && !file.startsWith(`${root}${path.sep}`)) {
      response.writeHead(400).end();
      return;
    }
    try {
      refreshGitAdvertisement(root, relative);
      const stat = fs.statSync(file);
      if (!stat.isFile()) throw new Error("not a file");
      response.writeHead(200, { "content-length": stat.size });
      fs.createReadStream(file).pipe(response);
    } catch {
      response.writeHead(404).end();
    }
  });
}

/**
 * Serve Git's smart upload-pack protocol as well as static files. Docker's
 * historical prepare script uses a shallow clone, which needs upload-pack's
 * capability negotiation and cannot use dumb HTTP's info/refs transport.
 */
async function serveSmartGitRequest(
  root: string,
  relative: string,
  request: IncomingMessage,
  response: ServerResponse,
): Promise<boolean> {
  const requestURL = new URL(request.url ?? "/", "http://fixture");
  const repo = smartGitRepository(root, relative);
  if (!repo) return false;

  if (
    request.method === "GET" &&
    relative.endsWith("/info/refs") &&
    requestURL.searchParams.get("service") === "git-upload-pack"
  ) {
    response.writeHead(200, {
      "content-type": "application/x-git-upload-pack-advertisement",
      "cache-control": "no-cache",
    });
    response.write("001e# service=git-upload-pack\n0000");
    await pipeGitUploadPack(response, repo, ["--stateless-rpc", "--advertise-refs"]);
    return true;
  }

  if (request.method === "POST" && relative.endsWith("/git-upload-pack")) {
    response.writeHead(200, {
      "content-type": "application/x-git-upload-pack-result",
      "cache-control": "no-cache",
    });
    await pipeGitUploadPack(response, repo, ["--stateless-rpc"], request);
    return true;
  }
  return false;
}

function smartGitRepository(root: string, relative: string): string | undefined {
  let suffix = "";
  if (relative.endsWith("/info/refs")) suffix = "/info/refs";
  if (relative.endsWith("/git-upload-pack")) suffix = "/git-upload-pack";
  if (!relative.startsWith("fixture/") || suffix === "") return undefined;

  const repo = path.resolve(root, relative.slice(0, -suffix.length));
  if (repo === root || !repo.startsWith(`${root}${path.sep}`) || !repo.endsWith(".git")) {
    return undefined;
  }
  try {
    if (!fs.statSync(repo).isDirectory()) return undefined;
  } catch {
    return undefined;
  }
  return repo;
}

function pipeGitUploadPack(
  response: ServerResponse,
  repo: string,
  args: string[],
  request?: IncomingMessage,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const git = spawn("git", ["upload-pack", ...args, repo]);
    git.once("error", reject);
    git.stderr.on("data", () => undefined);
    git.stdout.pipe(response, { end: false });
    if (request) request.pipe(git.stdin);
    else git.stdin.end();
    git.once("close", (code) => {
      if (code !== 0) {
        reject(new Error(`git upload-pack exited with ${code}`));
        return;
      }
      response.end();
      resolve();
    });
  });
}

/**
 * A static HTTP Git server is a dumb transport, so clients discover refs from
 * `info/refs`. Refresh that advertisement immediately before it is read: the
 * test workers push source commits after fixture startup and Git's hook is not
 * a sufficient synchronization point across all transport implementations.
 */
function refreshGitAdvertisement(root: string, relative: string): void {
  if (!relative.startsWith("fixture/") || !relative.endsWith(".git/info/refs")) return;

  const repo = path.resolve(root, relative.slice(0, -"/info/refs".length));
  if (repo !== root && !repo.startsWith(`${root}${path.sep}`)) return;
  execFileSync("git", ["--git-dir", repo, "update-server-info"]);
}

function dockerBridgeGateway(): string {
  const result = spawnSync(
    "docker",
    ["network", "inspect", "bridge", "-f", "{{(index .IPAM.Config 0).Gateway}}"],
    { encoding: "utf8" },
  );
  const gateway = result.status === 0 ? result.stdout.trim() : "";
  if (!gateway) throw new Error(`Could not determine Docker bridge gateway: ${result.stderr}`);
  return gateway;
}

function listen(server: Server): Promise<number> {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "0.0.0.0", () => {
      server.off("error", reject);
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("HTTP Git fixture did not receive a TCP port"));
        return;
      }
      resolve(address.port);
    });
  });
}

function closeServer(server: Server): Promise<void> {
  return new Promise((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
}
