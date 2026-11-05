import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { join, resolve, sep } from "node:path";

export const sdkModule = "github.com/kdlbs/kandev/pluginsdk";
const publicProxy = "https://proxy.golang.org";

export function goEnvironment(mode, root, proxy, inherited = process.env) {
  if (!["local", "public"].includes(mode))
    throw new Error("invalid consumer mode");
  if (mode === "public") {
    for (const key of ["GOPRIVATE", "GONOPROXY", "GONOSUMDB", "GOINSECURE"]) {
      if (inherited[key]) throw new Error(`public trust rejects ${key}`);
    }
    if (
      (inherited.GOPROXY && inherited.GOPROXY !== publicProxy) ||
      (inherited.GOSUMDB && inherited.GOSUMDB !== "sum.golang.org")
    ) {
      throw new Error(
        "public trust requires the public Go proxy and checksum database",
      );
    }
  }
  return {
    ...inherited,
    GOWORK: "off",
    GOENV: "off",
    GOFLAGS: "-modcacherw",
    GOPROXY: mode === "local" ? proxy : publicProxy,
    GOSUMDB: "sum.golang.org",
    GOPRIVATE: "",
    GONOPROXY: "",
    GONOSUMDB: mode === "local" ? sdkModule : "",
    GOINSECURE: "",
    GOMODCACHE: join(root, "mod"),
    GOCACHE: join(root, "build"),
    GOPATH: join(root, "gopath"),
  };
}

export async function startProxy(root) {
  const server = createServer(async (request, response) => {
    try {
      const path = new URL(request.url, "http://localhost").pathname;
      if (path === `/${sdkModule}` || path.startsWith(`/${sdkModule}/`)) {
        const file = resolve(root, `.${path}`);
        if (!file.startsWith(resolve(root) + sep))
          throw new Error("invalid proxy path");
        try {
          response.end(await readFile(file));
        } catch (error) {
          if (error.code !== "ENOENT") throw error;
          response.writeHead(404).end("SDK archive unavailable");
        }
      } else {
        const upstream = await fetch(publicProxy + path, {
          signal: AbortSignal.timeout(60_000),
        });
        response.writeHead(upstream.status);
        response.end(Buffer.from(await upstream.arrayBuffer()));
      }
    } catch (error) {
      response.writeHead(502).end(`Proxy request failed: ${error.message}`);
    }
  });
  await new Promise((accept, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", accept);
  });
  return {
    url: `http://127.0.0.1:${server.address().port}`,
    close: () =>
      new Promise((accept, reject) => {
        server.close((error) => (error ? reject(error) : accept()));
        server.closeAllConnections();
      }),
  };
}
