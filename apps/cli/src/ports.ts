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
 * Tries to bind a port on the given host. Returns true if the bind succeeds
 * (port is free). Closes immediately on success.
 *
 * This catches Windows "phantom" port reservations: Hyper-V/WSL silently
 * reserve random port ranges at boot that don't appear in netstat or via a
 * connect probe (nothing is listening, so connect-check thinks the port is
 * free) — but bind fails with "Only one usage of each socket address". A
 * connect-only check causes kandev's backend to choose a reserved port and
 * then die when it tries to actually listen on it.
 */
function canBindPort(port: number, host: string): Promise<boolean> {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once("error", () => resolve(false));
    server.listen(port, host, () => {
      server.close(() => resolve(true));
    });
  });
}

/**
 * Checks if a port is available by probing both IPv4 and IPv6 loopback.
 *
 * Uses BOTH a connect check and a bind check:
 *   - connect: detects ports where a listener is bound with SO_REUSEADDR
 *     (Node's default on macOS — bind-only check would falsely succeed)
 *   - bind:    detects Windows phantom reservations (Hyper-V/WSL) and
 *     ports in TIME_WAIT that connect-only check misses
 *
 * The port is available IFF nobody answers a connect AND a fresh bind
 * succeeds — covers macOS, Linux, and Windows.
 */
async function isPortAvailable(port: number): Promise<boolean> {
  // Run connect probes first, then the bind probe — they cannot share the
  // port concurrently. On loopback, server.listen() completes in the kernel
  // before a connect SYN to the same address is processed, so a concurrent
  // canBindPort+isPortInUse pair can answer each other and report a free
  // port as occupied. Sequencing keeps the bind probe's temporary listener
  // out of the connect probes' view.
  const [v4InUse, v6InUse] = await Promise.all([
    isPortInUse(port, "127.0.0.1"),
    isPortInUse(port, "::1"),
  ]);
  if (v4InUse || v6InUse) return false;
  return canBindPort(port, "127.0.0.1");
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
