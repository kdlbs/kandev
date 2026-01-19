import crypto from "node:crypto";
import net from "node:net";

import { RANDOM_PORT_MAX, RANDOM_PORT_MIN, RANDOM_PORT_RETRIES } from "./constants";

export function ensureValidPort(port: number | undefined, name: string): number | undefined {
  if (port === undefined) {
    return undefined;
  }
  if (!Number.isInteger(port) || port <= 0 || port > 65535) {
    throw new Error(`${name} must be an integer between 1 and 65535`);
  }
  return port;
}

function isPortAvailable(port: number, host = "0.0.0.0"): Promise<boolean> {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.unref();
    server.on("error", (err: NodeJS.ErrnoException) => {
      if (err.code === "EADDRINUSE" || err.code === "EACCES") {
        resolve(false);
      } else {
        resolve(false);
      }
    });
    server.listen(port, host, () => {
      server.close(() => resolve(true));
    });
  });
}

async function reserveSpecificPort(port: number, host = "127.0.0.1"): Promise<net.Server | null> {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.on("error", () => resolve(null));
    server.listen(port, host, () => resolve(server));
  });
}

export async function pickAvailablePort(
  preferred: number,
  retries = RANDOM_PORT_RETRIES,
): Promise<number> {
  if (await isPortAvailable(preferred)) {
    return preferred;
  }
  for (let i = 0; i < retries; i += 1) {
    const candidate = crypto.randomInt(RANDOM_PORT_MIN, RANDOM_PORT_MAX + 1);
    if (await isPortAvailable(candidate)) {
      return candidate;
    }
  }
  throw new Error(`Unable to find a free port after ${retries + 1} attempts`);
}

export async function pickAndReservePort(
  preferred: number,
  retries = RANDOM_PORT_RETRIES,
): Promise<{ port: number; release: () => Promise<void> }> {
  const reservedPreferred = await reserveSpecificPort(preferred);
  if (reservedPreferred) {
    return {
      port: preferred,
      release: () => new Promise((resolve) => reservedPreferred.close(() => resolve())),
    };
  }

  for (let i = 0; i < retries; i += 1) {
    const candidate = crypto.randomInt(RANDOM_PORT_MIN, RANDOM_PORT_MAX + 1);
    const reserved = await reserveSpecificPort(candidate);
    if (reserved) {
      return {
        port: candidate,
        release: () => new Promise((resolve) => reserved.close(() => resolve())),
      };
    }
  }

  throw new Error(`Unable to reserve a free port after ${retries + 1} attempts`);
}
