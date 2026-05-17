import os from "node:os";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
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

describe("buildWebEnv allowedDevOrigins", () => {
  const originalAllowed = process.env.NEXT_ALLOWED_DEV_ORIGINS;

  beforeEach(() => {
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
          address: "192.168.1.34",
          netmask: "255.255.252.0",
          family: "IPv4",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "192.168.1.34/22",
        },
        {
          address: "fe80::1",
          netmask: "ffff:ffff:ffff:ffff::",
          family: "IPv6",
          mac: "aa:bb:cc:dd:ee:ff",
          internal: false,
          cidr: "fe80::1/64",
          scopeid: 2,
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
  });

  afterEach(() => {
    vi.restoreAllMocks();
    if (originalAllowed === undefined) {
      delete process.env.NEXT_ALLOWED_DEV_ORIGINS;
    } else {
      process.env.NEXT_ALLOWED_DEV_ORIGINS = originalAllowed;
    }
  });

  it("populates NEXT_ALLOWED_DEV_ORIGINS with LAN + Tailscale IPs in dev mode", () => {
    delete process.env.NEXT_ALLOWED_DEV_ORIGINS;
    const env = buildWebEnv({ ports });
    const origins = (env.NEXT_ALLOWED_DEV_ORIGINS ?? "").split(",");
    expect(origins).toContain("192.168.1.34");
    expect(origins).toContain("100.94.173.104");
  });

  it("excludes loopback and link-local IPv6 addresses", () => {
    delete process.env.NEXT_ALLOWED_DEV_ORIGINS;
    const env = buildWebEnv({ ports });
    const origins = (env.NEXT_ALLOWED_DEV_ORIGINS ?? "").split(",");
    expect(origins).not.toContain("127.0.0.1");
    expect(origins).not.toContain("fe80::1");
  });

  it("merges with a user-supplied NEXT_ALLOWED_DEV_ORIGINS, deduped", () => {
    process.env.NEXT_ALLOWED_DEV_ORIGINS = "dev.example, 192.168.1.34";
    const env = buildWebEnv({ ports });
    const origins = (env.NEXT_ALLOWED_DEV_ORIGINS ?? "").split(",");
    expect(origins).toContain("dev.example");
    expect(origins).toContain("192.168.1.34");
    expect(origins).toContain("100.94.173.104");
    expect(origins.filter((s) => s === "192.168.1.34")).toHaveLength(1);
  });

  it("does not set NEXT_ALLOWED_DEV_ORIGINS in production", () => {
    delete process.env.NEXT_ALLOWED_DEV_ORIGINS;
    const env = buildWebEnv({ ports, production: true });
    expect(env.NEXT_ALLOWED_DEV_ORIGINS).toBeUndefined();
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

  it("excludes IPv6 link-local (fe80::) addresses", () => {
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
    expect(listHostNetworkAddresses()).toEqual(["2001:db8::1"]);
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

  it("prints network URLs on the backend port by default (start/run mode)", () => {
    mockTwoInterfaces();
    logStartupInfo({ header: "test", ports });
    const lines = log.mock.calls.map((c) => c.join(" "));
    expect(lines).toContain("[kandev]   network: http://192.168.1.34:38429");
    expect(lines).toContain("[kandev]   network: http://100.94.173.104:38429");
    // Web port network URLs are NOT printed — in start/run the backend
    // reverse-proxies the web app, so the web port is internal-only and would
    // only mislead a remote user.
    expect(lines.some((l) => l.includes("network:") && l.includes(":37429"))).toBe(false);
  });

  it('prints network URLs on the web port when primary="web" (dev mode)', () => {
    mockTwoInterfaces();
    logStartupInfo({ header: "test", ports, primary: "web" });
    const lines = log.mock.calls.map((c) => c.join(" "));
    expect(lines).toContain("[kandev]   network: http://192.168.1.34:37429");
    expect(lines).toContain("[kandev]   network: http://100.94.173.104:37429");
    expect(lines.some((l) => l.includes("network:") && l.includes(":38429"))).toBe(false);
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
