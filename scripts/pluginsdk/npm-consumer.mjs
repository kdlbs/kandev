import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { join } from "node:path";

// Only the SDK is served locally. Other package metadata comes from the public registry.
export async function startRegistry(candidate) {
  const bytes = await readFile(candidate.archive);
  let url;
  const server = createServer(async (request, response) => {
    try {
      const path = decodeURIComponent(request.url.split("?")[0]);
      if (path === "/@kdlbs/plugin-sdk/-/plugin-sdk.tgz") {
        response.setHeader("content-type", "application/octet-stream");
        response.end(bytes);
      } else if (path === "/@kdlbs/plugin-sdk") {
        response.setHeader("content-type", "application/json");
        response.end(
          JSON.stringify({
            name: "@kdlbs/plugin-sdk",
            "dist-tags": { latest: candidate.sdk_version },
            versions: {
              [candidate.sdk_version]: {
                name: "@kdlbs/plugin-sdk",
                version: candidate.sdk_version,
                dist: {
                  tarball: url + "/@kdlbs/plugin-sdk/-/plugin-sdk.tgz",
                  integrity: candidate.integrity,
                },
              },
            },
          }),
        );
      } else if (path.startsWith("/@kdlbs/plugin-sdk")) {
        response.writeHead(404).end();
      } else {
        const upstream = await fetch(
          "https://registry.npmjs.org" + request.url,
        );
        response.writeHead(upstream.status, {
          "content-type":
            upstream.headers.get("content-type") ?? "application/json",
        });
        response.end(Buffer.from(await upstream.arrayBuffer()));
      }
    } catch {
      response.writeHead(502).end();
    }
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  url = `http://127.0.0.1:${server.address().port}`;
  return {
    url,
    close: () =>
      new Promise((resolve, reject) => {
        server.closeAllConnections();
        server.close((error) => (error ? reject(error) : resolve()));
      }),
  };
}

export function npmEnvironment(root) {
  const env = Object.fromEntries(
    Object.entries(process.env).filter(([key]) => !/^npm_config_/i.test(key)),
  );
  return {
    ...env,
    NPM_CONFIG_CACHE: join(root, "npm-cache"),
    NPM_CONFIG_USERCONFIG: join(root, "user.npmrc"),
    NPM_CONFIG_GLOBALCONFIG: join(root, "global.npmrc"),
    NPM_CONFIG_IGNORE_SCRIPTS: "true",
    NPM_CONFIG_AUDIT: "false",
    NPM_CONFIG_FUND: "false",
  };
}
