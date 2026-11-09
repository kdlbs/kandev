import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { once } from "node:events";

export type ManagedRuntimeTestRegistry = {
  url: string;
  requests: () => number;
  close: () => Promise<void>;
};

/** Serves one locally packed managed runtime package without contacting npmjs. */
export async function startManagedRuntimeTestRegistry(options: {
  root: string;
  packageSpec: string;
  binName: string;
  advertisedHost?: string;
}): Promise<ManagedRuntimeTestRegistry> {
  const separator = options.packageSpec.lastIndexOf("@");
  if (separator <= 0) throw new Error(`invalid exact package spec: ${options.packageSpec}`);
  const packageName = options.packageSpec.slice(0, separator);
  const version = options.packageSpec.slice(separator + 1);
  const registryRoot = path.join(options.root, "managed-runtime-registry");
  const packageRoot = path.join(registryRoot, "package");
  const archiveRoot = path.join(registryRoot, "archives");
  fs.mkdirSync(path.join(packageRoot, "bin"), { recursive: true });
  fs.mkdirSync(archiveRoot, { recursive: true });
  fs.writeFileSync(
    path.join(packageRoot, "package.json"),
    JSON.stringify({
      name: packageName,
      version,
      bin: { [options.binName]: `bin/${options.binName}.js` },
    }),
  );
  fs.writeFileSync(
    path.join(packageRoot, "bin", `${options.binName}.js`),
    `#!/usr/bin/env node\nconst { spawn } = require("node:child_process");\n` +
      `const child = spawn(process.env.E2E_MOCK_AGENT_PATH || "/usr/local/bin/mock-agent", process.argv.slice(2), { stdio: "inherit", env: process.env });\n` +
      `child.once("error", (error) => { console.error(error.message); process.exit(1); });\n` +
      `child.once("exit", (code, signal) => { if (signal) process.kill(process.pid, signal); else process.exit(code ?? 1); });\n`,
    { mode: 0o755 },
  );

  const npmPath = execFileSync("which", ["npm"], { encoding: "utf8" }).trim();
  const packCache = path.join(registryRoot, "npm-cache");
  fs.mkdirSync(packCache, { recursive: true });
  const packedName = execFileSync(
    npmPath,
    ["pack", packageRoot, "--pack-destination", archiveRoot],
    {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
      env: { ...process.env, NPM_CONFIG_CACHE: packCache, npm_config_cache: packCache },
    },
  )
    .trim()
    .split(/\r?\n/)
    .at(-1);
  if (!packedName) throw new Error("npm pack did not return an archive name");
  const archive = fs.readFileSync(path.join(archiveRoot, packedName));
  const shasum = createHash("sha1").update(archive).digest("hex");
  const integrity = `sha512-${createHash("sha512").update(archive).digest("base64")}`;
  let requestCount = 0;
  let serverAddress = "";
  const tarballPath = `/archives/${encodeURIComponent(packedName)}`;
  const server = http.createServer((request, response) => {
    requestCount++;
    if (request.method !== "GET") {
      response.writeHead(405).end();
      return;
    }
    if (request.url === tarballPath) {
      response.writeHead(200, { "content-type": "application/octet-stream" }).end(archive);
      return;
    }
    const tarball = `${serverAddress}${tarballPath}`;
    const metadata = {
      name: packageName,
      "dist-tags": { latest: version },
      versions: {
        [version]: {
          name: packageName,
          version,
          bin: { [options.binName]: `bin/${options.binName}.js` },
          dist: { tarball, shasum, integrity },
        },
      },
    };
    response.writeHead(200, { "content-type": "application/json" }).end(JSON.stringify(metadata));
  });
  server.listen(0, "0.0.0.0");
  await once(server, "listening");
  const address = server.address();
  if (!address || typeof address === "string")
    throw new Error("npm registry failed to bind TCP port");
  const advertisedHost = options.advertisedHost ?? "127.0.0.1";
  serverAddress = `http://${advertisedHost}:${address.port}`;

  return {
    url: serverAddress,
    requests: () => requestCount,
    close: async () => {
      server.close();
      await once(server, "close");
    },
  };
}
