import net from "node:net";

import { afterEach, describe, expect, it } from "vitest";

import { ensureValidPort, pickAvailablePort, __testing } from "./ports";

describe("ensureValidPort", () => {
  it("returns undefined for undefined input", () => {
    expect(ensureValidPort(undefined, "test")).toBeUndefined();
  });

  it("returns the port for valid values", () => {
    expect(ensureValidPort(8080, "test")).toBe(8080);
  });

  it("accepts port 1 (minimum)", () => {
    expect(ensureValidPort(1, "test")).toBe(1);
  });

  it("accepts port 65535 (maximum)", () => {
    expect(ensureValidPort(65535, "test")).toBe(65535);
  });

  it("throws for port 0", () => {
    expect(() => ensureValidPort(0, "backend")).toThrow(
      "backend must be an integer between 1 and 65535",
    );
  });

  it("throws for port above 65535", () => {
    expect(() => ensureValidPort(65536, "web")).toThrow(
      "web must be an integer between 1 and 65535",
    );
  });

  it("throws for negative port", () => {
    expect(() => ensureValidPort(-1, "test")).toThrow();
  });

  it("throws for floating point port", () => {
    expect(() => ensureValidPort(8080.5, "test")).toThrow();
  });

  it("throws for NaN", () => {
    expect(() => ensureValidPort(NaN, "test")).toThrow();
  });
});

function listenOn(host: string): Promise<{ server: net.Server; port: number }> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.on("error", reject);
    server.listen(0, host, () => {
      const addr = server.address();
      if (addr && typeof addr === "object") {
        resolve({ server, port: addr.port });
      } else {
        reject(new Error("no address"));
      }
    });
  });
}

function closeServer(server: net.Server): Promise<void> {
  return new Promise((resolve) => server.close(() => resolve()));
}

describe("isPortAvailable", () => {
  const servers: net.Server[] = [];

  afterEach(async () => {
    while (servers.length) {
      const s = servers.pop();
      if (s) await closeServer(s);
    }
  });

  it("returns false when a server is listening on the IPv4 loopback", async () => {
    const { server, port } = await listenOn("127.0.0.1");
    servers.push(server);
    expect(await __testing.isPortAvailable(port)).toBe(false);
  });

  it("returns true for a port that nothing is bound to", async () => {
    const { server, port } = await listenOn("127.0.0.1");
    await closeServer(server);
    // Port may be in TIME_WAIT briefly, but bind-probe with SO_REUSEADDR
    // (Node default on POSIX) treats that as available.
    expect(await __testing.isPortAvailable(port)).toBe(true);
  });
});

describe("isPortInUse", () => {
  it("returns false within the timeout when the host black-holes packets", async () => {
    // 192.0.2.0/24 is TEST-NET-1 (RFC 5737): traffic to it is reliably
    // dropped, simulating the WSL2 mirrored-mode SYN black-hole. Without the
    // timeout in isPortInUse, this connect would hang for ~75 seconds.
    const start = Date.now();
    const result = await __testing.isPortInUse(38429, "192.0.2.1", 200);
    const elapsed = Date.now() - start;
    expect(result).toBe(false);
    expect(elapsed).toBeLessThan(1000);
  });
});

describe("pickAvailablePort", () => {
  it("returns the preferred port when it is free", async () => {
    const { server, port } = await listenOn("127.0.0.1");
    await closeServer(server);
    expect(await pickAvailablePort(port)).toBe(port);
  });

  it("returns a fallback port when the preferred port is taken", async () => {
    const { server, port } = await listenOn("127.0.0.1");
    try {
      const picked = await pickAvailablePort(port, 5);
      expect(picked).not.toBe(port);
      expect(picked).toBeGreaterThan(0);
    } finally {
      await closeServer(server);
    }
  });
});
