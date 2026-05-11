import { afterEach, describe, expect, it } from "vitest";

import { buildWebEnv, type PortConfig } from "./shared";

const ports: PortConfig = {
  backendPort: 38429,
  webPort: 37429,
  agentctlPort: 36429,
  backendUrl: "http://localhost:38429",
};

describe("buildWebEnv", () => {
  const originalApiPort = process.env.NEXT_PUBLIC_KANDEV_API_PORT;
  afterEach(() => {
    if (originalApiPort === undefined) {
      delete process.env.NEXT_PUBLIC_KANDEV_API_PORT;
    } else {
      process.env.NEXT_PUBLIC_KANDEV_API_PORT = originalApiPort;
    }
  });

  it("sets NEXT_PUBLIC_KANDEV_API_PORT in dev mode so the browser knows the backend port", () => {
    const env = buildWebEnv({ ports });

    expect(env.NEXT_PUBLIC_KANDEV_API_PORT).toBe("38429");
    expect(env.NODE_ENV).not.toBe("production");
  });

  it("does not set NEXT_PUBLIC_KANDEV_API_PORT in production single-port mode", () => {
    // The Go backend reverse-proxies Next.js, so the client must use same-origin.
    // A non-empty value would cause the client to build cross-origin URLs like
    // `https://host:38429/...` that aren't reachable behind a reverse proxy.
    const env = buildWebEnv({ ports, production: true });

    expect(env.NEXT_PUBLIC_KANDEV_API_PORT).toBeUndefined();
    expect(env.NODE_ENV).toBe("production");
  });

  it("strips a host-level NEXT_PUBLIC_KANDEV_API_PORT in production", () => {
    // If an operator has the var in their shell/Docker env, the process.env
    // spread would otherwise leak it into the Next.js process and reintroduce
    // the cross-origin URL problem this fix addresses.
    process.env.NEXT_PUBLIC_KANDEV_API_PORT = "99999";

    const env = buildWebEnv({ ports, production: true });

    expect(env.NEXT_PUBLIC_KANDEV_API_PORT).toBeUndefined();
  });

  it("always sets KANDEV_API_BASE_URL for SSR fetches", () => {
    expect(buildWebEnv({ ports }).KANDEV_API_BASE_URL).toBe("http://localhost:38429");
    expect(buildWebEnv({ ports, production: true }).KANDEV_API_BASE_URL).toBe(
      "http://localhost:38429",
    );
  });

  it("enables debug flag when requested", () => {
    expect(buildWebEnv({ ports, debug: true }).NEXT_PUBLIC_KANDEV_DEBUG).toBe("true");
    expect(buildWebEnv({ ports }).NEXT_PUBLIC_KANDEV_DEBUG).toBeUndefined();
  });
});
