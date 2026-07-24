import { execFileSync, spawnSync } from "node:child_process";
import { createServer, type Server } from "node:http";
import fs from "node:fs";
import path from "node:path";

type HTTPGitFixture = {
  remoteURL: string;
  /**
   * Backend-only config that survives restarts without colliding with
   * repoclone's command-scoped credential configuration.
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
  onListening?: (port: number) => void;
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
    options.onListening?.(port);
    const fixtureOrigin = `http://${dockerBridgeGateway()}:${port}/`;
    const remoteURL = `https://gitlab.com/fixture/${name}.git`;
    const backendGitConfigPath = path.join(root, "fixture", `${name}.gitconfig`);
    const config = `[url "${fixtureOrigin}fixture/${name}.git"]\n\tinsteadOf = ${remoteURL}\n`;
    (options.writeBackendGitConfig ?? fs.writeFileSync)(backendGitConfigPath, config);
    return {
      // The source endpoint must receive the real GitLab identity so the
      // production trusted-origin validation remains exercised. Disposable test
      // executor profiles and their isolated backend fixture rewrite Git's clone
      // transport to this local HTTP server.
      remoteURL,
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
  return createServer((request, response) => {
    const pathname = decodeURIComponent(new URL(request.url ?? "/", "http://fixture").pathname);
    const relative = pathname.replace(/^\/+/, "");
    const file = path.resolve(root, relative);
    if (file !== root && !file.startsWith(`${root}${path.sep}`)) {
      response.writeHead(400).end();
      return;
    }
    try {
      const stat = fs.statSync(file);
      if (!stat.isFile()) throw new Error("not a file");
      response.writeHead(200, { "content-length": stat.size });
      fs.createReadStream(file).pipe(response);
    } catch {
      response.writeHead(404).end();
    }
  });
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
