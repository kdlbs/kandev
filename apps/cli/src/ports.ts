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

/**
 * Tries to connect to a port on the given host. Returns true if something
 * is already listening (i.e. the port is in use).
 *
 * This is more reliable than a bind-based check on macOS where
 * SO_REUSEADDR (set by default in Node.js) can allow a bind to succeed
 * even when another process is already listening on the same port.
 */
function isPortInUse(port: number, host: string): Promise<boolean> {
  return new Promise((resolve) => {
    const socket = net.createConnection({ port, host });
    socket.once("connect", () => {
      socket.destroy();
      resolve(true);
    });
    socket.once("error", () => {
      resolve(false);
    });
  });
}

/**
 * Checks if a port is available by probing both IPv4 and IPv6 loopback.
 *
 * Uses a connect-based check: if we can connect to the port on either
 * 127.0.0.1 or ::1, something is already listening and the port is taken.
 */
async function isPortAvailable(port: number): Promise<boolean> {
  const [v4InUse, v6InUse] = await Promise.all([
    isPortInUse(port, "127.0.0.1"),
    isPortInUse(port, "::1"),
  ]);
  return !v4InUse && !v6InUse;
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
  if (await isPortAvailable(preferred)) {
    const reservedPreferred = await reserveSpecificPort(preferred);
    if (reservedPreferred) {
      return {
        port: preferred,
        release: () => new Promise((resolve) => reservedPreferred.close(() => resolve())),
      };
    }
  }

  for (let i = 0; i < retries; i += 1) {
    const candidate = crypto.randomInt(RANDOM_PORT_MIN, RANDOM_PORT_MAX + 1);
    if (!(await isPortAvailable(candidate))) continue;
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
