import os from "node:os";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  buildBackendEnv,
  buildWebEnv,
  listHostNetworkAddresses,
  logStartupInfo,
  networkUrlsForPort,
  type PortConfig,
} from "./shared";

const ports: PortConfig = {
  backendPort: 38429,
  webPort: 37429,
  agentctlPort: 36429,
  backendUrl: "http://localhost:38429",
};

describe("buildWebEnv", () => {
  const originalViteApiPort = process.env.VITE_KANDEV_API_PORT;
  afterEach(() => {
    if (originalViteApiPort === undefined) {
      delete process.env.VITE_KANDEV_API_PORT;
    } else {
      process.env.VITE_KANDEV_API_PORT = originalViteApiPort;
    }
  });

  it("does not set browser API port env vars in dev mode", () => {
    const env = buildWebEnv({ ports });

    expect(env.VITE_KANDEV_API_PORT).toBeUndefined();
    expect(env.NODE_ENV).not.toBe("production");
  });

  it("does not set browser API port env vars in production single-port mode", () => {
    // The Go backend serves the SPA, so the client must use same-origin.
    // A non-empty value would cause the client to build cross-origin URLs like
    // `https://host:38429/...` that aren't reachable behind a reverse proxy.
    const env = buildWebEnv({ ports, production: true });

    expect(env.VITE_KANDEV_API_PORT).toBeUndefined();
    expect(env.NODE_ENV).toBe("production");
  });

  it("strips host-level browser API port env vars in production", () => {
    // If an operator has the var in their shell/Docker env, the process.env
    // spread would otherwise leak it into the web process and reintroduce
    // the cross-origin URL problem this fix addresses.
    process.env.VITE_KANDEV_API_PORT = "99999";

    const env = buildWebEnv({ ports, production: true });

    expect(env.VITE_KANDEV_API_PORT).toBeUndefined();
  });

  it("always sets KANDEV_API_BASE_URL for SSR fetches", () => {
    expect(buildWebEnv({ ports }).KANDEV_API_BASE_URL).toBe("http://localhost:38429");
    expect(buildWebEnv({ ports, production: true }).KANDEV_API_BASE_URL).toBe(
      "http://localhost:38429",
    );
  });

  it("enables debug flag when requested", () => {
    expect(buildWebEnv({ ports, debug: true }).VITE_KANDEV_DEBUG).toBe("true");
    expect(buildWebEnv({ ports, debug: true }).KANDEV_DEBUG).toBe("true");
    expect(buildWebEnv({ ports }).VITE_KANDEV_DEBUG).toBeUndefined();
    expect(buildWebEnv({ ports }).KANDEV_DEBUG).toBeUndefined();
  });
});

describe("buildBackendEnv", () => {
  it("points backend non-API routes at the dev web server by default", () => {
    const env = buildBackendEnv({ ports });

    expect(env.KANDEV_WEB_INTERNAL_URL).toBe("http://localhost:37429");
  });

  it("omits the web proxy in production single-process mode", () => {
    const env = buildBackendEnv({ ports, webProxy: false });

    expect(env.KANDEV_WEB_INTERNAL_URL).toBeUndefined();
  });
});

describe("listHostNetworkAddresses", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns a deduplicated list of non-internal addresses", () => {
    vi.spyOn(os, "networkInterfaces").mockReturnValue({
      eth0: [
        {
          address: "10.0.0.1",
          netmask: "255.0.0.0",
          family: "IPv4",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "10.0.0.1/8",
        },
      ],
      eth1: [
        {
          address: "10.0.0.1",
          netmask: "255.0.0.0",
          family: "IPv4",
          mac: "11:22:33:44:55:66",
          internal: false,
          cidr: "10.0.0.1/8",
        },
      ],
    });
    expect(listHostNetworkAddresses()).toEqual(["10.0.0.1"]);
  });

  it("excludes internal loopback addresses", () => {
    vi.spyOn(os, "networkInterfaces").mockReturnValue({
      lo: [
        {
          address: "127.0.0.1",
          netmask: "255.0.0.0",
          family: "IPv4",
          mac: "00:00:00:00:00:00",
          internal: true,
          cidr: "127.0.0.1/8",
        },
      ],
      eth0: [
        {
          address: "10.0.0.5",
          netmask: "255.0.0.0",
          family: "IPv4",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "10.0.0.5/8",
        },
      ],
    });
    expect(listHostNetworkAddresses()).toEqual(["10.0.0.5"]);
  });

  it("excludes IPv4 link-local (169.254.0.0/16) addresses", () => {
    vi.spyOn(os, "networkInterfaces").mockReturnValue({
      eth0: [
        {
          address: "169.254.83.107",
          netmask: "255.255.0.0",
          family: "IPv4",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "169.254.83.107/16",
        },
        {
          address: "192.168.1.34",
          netmask: "255.255.252.0",
          family: "IPv4",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "192.168.1.34/22",
        },
      ],
    });
    expect(listHostNetworkAddresses()).toEqual(["192.168.1.34"]);
  });

  it("excludes the full IPv6 link-local range (fe80::/10, covers fe80–febf)", () => {
    // In practice OS stacks only assign fe80::/64, but per RFC 4291 the link-
    // local range is fe80::/10 — anything from fe80:: through febf:ffff:... is
    // link-local and never reachable from a remote machine.
    vi.spyOn(os, "networkInterfaces").mockReturnValue({
      eth0: [
        {
          address: "fe80::abcd",
          netmask: "ffff:ffff:ffff:ffff::",
          family: "IPv6",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "fe80::abcd/64",
          scopeid: 2,
        },
        {
          address: "fea0::1",
          netmask: "ffff:ffff:ffff:ffff::",
          family: "IPv6",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "fea0::1/64",
          scopeid: 2,
        },
        {
          address: "FEBF::1",
          netmask: "ffff:ffff:ffff:ffff::",
          family: "IPv6",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "FEBF::1/64",
          scopeid: 2,
        },
        {
          address: "fec0::1",
          netmask: "ffff:ffff:ffff:ffff::",
          family: "IPv6",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "fec0::1/64",
          scopeid: 0,
        },
        {
          address: "2001:db8::1",
          netmask: "ffff:ffff:ffff:ffff::",
          family: "IPv6",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "2001:db8::1/64",
          scopeid: 0,
        },
      ],
    });
    // fe80::abcd, fea0::1, FEBF::1 are link-local (excluded).
    // fec0::1 was deprecated site-local (kept — outside fe80::/10).
    // 2001:db8::1 is global (kept).
    expect(listHostNetworkAddresses()).toEqual(["fec0::1", "2001:db8::1"]);
  });
});

describe("networkUrlsForPort", () => {
  it("formats IPv4 hosts without brackets", () => {
    expect(networkUrlsForPort(38429, ["192.168.1.34", "100.94.173.104"])).toEqual([
      "http://192.168.1.34:38429",
      "http://100.94.173.104:38429",
    ]);
  });

  it("wraps IPv6 hosts in brackets per RFC 3986", () => {
    expect(networkUrlsForPort(38429, ["2001:db8::1"])).toEqual(["http://[2001:db8::1]:38429"]);
  });

  it("returns an empty list when there are no hosts", () => {
    expect(networkUrlsForPort(38429, [])).toEqual([]);
  });
});

describe("logStartupInfo", () => {
  let log: ReturnType<typeof vi.spyOn<typeof console, "log">>;

  beforeEach(() => {
    log = vi.spyOn(console, "log").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  function mockTwoInterfaces() {
    vi.spyOn(os, "networkInterfaces").mockReturnValue({
      eth0: [
        {
          address: "192.168.1.34",
          netmask: "255.255.252.0",
          family: "IPv4",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "192.168.1.34/22",
        },
      ],
      tailscale0: [
        {
          address: "100.94.173.104",
          netmask: "255.255.255.255",
          family: "IPv4",
          mac: "00:00:00:00:00:00",
          internal: false,
          cidr: "100.94.173.104/32",
        },
      ],
    });
  }

  it("prints the backend URL and network URLs by default (start/run mode)", () => {
    mockTwoInterfaces();
    logStartupInfo({ header: "test", ports });
    const lines = log.mock.calls.map((c) => c.join(" "));
    expect(lines).toContain("[kandev] url: http://localhost:38429");
    expect(lines).toContain("[kandev]   network: http://192.168.1.34:38429");
    expect(lines).toContain("[kandev]   network: http://100.94.173.104:38429");
    // Internal-only ports must not appear — the user opens exactly one URL.
    expect(lines.some((l) => l.includes(":37429"))).toBe(false);
    expect(lines.some((l) => l.includes("agentctl"))).toBe(false);
  });

  it('prints the web URL and network URLs when primary="web" is explicitly requested', () => {
    mockTwoInterfaces();
    logStartupInfo({ header: "test", ports, primary: "web" });
    const lines = log.mock.calls.map((c) => c.join(" "));
    expect(lines).toContain("[kandev] url: http://localhost:37429");
    expect(lines).toContain("[kandev]   network: http://192.168.1.34:37429");
    expect(lines).toContain("[kandev]   network: http://100.94.173.104:37429");
    // Explicit web-primary mode is for diagnostics; normal dev/start modes
    // use the backend URL as the browser entrypoint.
    expect(lines.some((l) => l === "[kandev] url: http://localhost:38429")).toBe(false);
    expect(lines.some((l) => l.includes("network:") && l.includes(":38429"))).toBe(false);
  });

  it("still emits the mcp endpoint pointing at the backend port", () => {
    vi.spyOn(os, "networkInterfaces").mockReturnValue({});
    logStartupInfo({ header: "test", ports, primary: "web" });
    const lines = log.mock.calls.map((c) => c.join(" "));
    expect(lines).toContain("[kandev] mcp: http://localhost:38429/mcp");
  });

  it("emits no network lines when only loopback interfaces exist", () => {
    vi.spyOn(os, "networkInterfaces").mockReturnValue({
      lo: [
        {
          address: "127.0.0.1",
          netmask: "255.0.0.0",
          family: "IPv4",
          mac: "00:00:00:00:00:00",
          internal: true,
          cidr: "127.0.0.1/8",
        },
      ],
    });

    logStartupInfo({ header: "test", ports });

    const lines = log.mock.calls.map((c) => c.join(" "));
    expect(lines.some((l) => l.includes("network:"))).toBe(false);
  });
});
