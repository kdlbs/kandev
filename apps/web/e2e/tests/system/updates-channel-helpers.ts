import fs from "node:fs";
import { createServer, type Server } from "node:http";
import path from "node:path";

import type { BackendContext } from "../../fixtures/backend";

export const NIGHTLY_VERSION = "1.0.1-nightly.shaabcdef123456";
export const NIGHTLY_TAG = `v${NIGHTLY_VERSION}`;

type ManagedNPMUpdatesFixture = {
  registryRequests: () => number;
  release: () => Promise<void>;
};

export async function useManagedNPMUpdates(
  backend: BackendContext,
  version = NIGHTLY_VERSION,
): Promise<ManagedNPMUpdatesFixture> {
  const registry = await startRegistry(version);
  let releaseEnv: (() => Promise<void>) | undefined;
  try {
    const metadataPath = writeManagedServiceMetadata(backend);
    releaseEnv = await backend.useEnv({
      KANDEV_RUNNING_AS_SERVICE: "true",
      KANDEV_SERVICE_MODE: "user",
      KANDEV_SERVICE_MANAGER: "systemd",
      KANDEV_INSTALL_KIND: "npm",
      KANDEV_SERVICE_METADATA: metadataPath,
      KANDEV_E2E_NPM_REGISTRY_URL: registry.url,
    });
  } catch (error) {
    await closeServer(registry.server);
    throw error;
  }

  return {
    registryRequests: () => registry.requests(),
    release: async () => {
      try {
        await releaseEnv?.();
      } finally {
        await closeServer(registry.server);
      }
    },
  };
}

function writeManagedServiceMetadata(backend: BackendContext): string {
  const fixtureDir = path.join(backend.tmpDir, "nightly-service");
  const metadataPath = path.join(fixtureDir, "install.json");
  const servicePath = path.join(fixtureDir, "kandev.service");
  fs.mkdirSync(fixtureDir, { recursive: true });
  fs.writeFileSync(
    servicePath,
    [
      "# managed by kandev",
      "[Service]",
      "Environment=KANDEV_RUNNING_AS_SERVICE=true",
      `Environment=KANDEV_SERVICE_METADATA=${metadataPath}`,
      "",
    ].join("\n"),
  );
  fs.writeFileSync(
    metadataPath,
    JSON.stringify({
      version: 1,
      manager: "systemd",
      mode: "user",
      kind: "npm",
      home_dir: backend.tmpDir,
      log_dir: path.join(backend.tmpDir, "logs"),
      service_path: servicePath,
      launcher_path: path.join(fixtureDir, "kandev"),
      installed_at: "2026-07-31T12:00:00Z",
    }),
  );
  return metadataPath;
}

async function startRegistry(version: string): Promise<{
  server: Server;
  url: string;
  requests: () => number;
}> {
  let requestCount = 0;
  const server = createServer((_request, response) => {
    requestCount += 1;
    response.writeHead(200, { "Content-Type": "application/json" });
    response.end(
      JSON.stringify({
        "dist-tags": { nightly: version },
        versions: { [version]: { name: "kandev", version } },
      }),
    );
  });
  await new Promise<void>((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const address = server.address();
  if (!address || typeof address === "string") {
    await closeServer(server);
    throw new Error("nightly registry fixture did not bind a TCP port");
  }
  return {
    server,
    url: `http://127.0.0.1:${address.port}/kandev`,
    requests: () => requestCount,
  };
}

function closeServer(server: Server): Promise<void> {
  if (!server.listening) {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()));
    server.closeAllConnections();
  });
}
