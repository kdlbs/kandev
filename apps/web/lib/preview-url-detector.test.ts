import { describe, expect, it, vi } from "vitest";
import {
  detectPreviewUrl,
  detectPreviewUrlFromOutput,
  rewritePreviewUrlForProxy,
} from "./preview-url-detector";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({
    apiBaseUrl: "http://localhost:8080",
  }),
}));

const LOCALHOST_3000_URL = "http://localhost:3000/";
const BARE_LOCALHOST_3000_URL = "http://localhost:3000";
const LOCAL_HOSTS = ["localhost", "127.0.0.1", "0.0.0.0"];
const INVALID_PORTS = [65536, 70000, 123456];
const INVALID_LOCALHOST_CANDIDATES = INVALID_PORTS.flatMap((port) => [
  `localhost:${port}`,
  `http://localhost:${port}`,
]);

describe("detectPreviewUrl - full URL patterns", () => {
  it("detects localhost URL with port", () => {
    const result = detectPreviewUrl("Server running at http://localhost:3000");
    expect(result).toEqual({ url: LOCALHOST_3000_URL, port: 3000, scheme: "http" });
  });

  it("detects 127.0.0.1 URL with port", () => {
    const result = detectPreviewUrl("Listening on http://127.0.0.1:8080");
    expect(result).toEqual({ url: "http://127.0.0.1:8080/", port: 8080, scheme: "http" });
  });

  it("detects 0.0.0.0 URL with port", () => {
    const result = detectPreviewUrl("Server at http://0.0.0.0:4000");
    expect(result).toEqual({ url: "http://0.0.0.0:4000/", port: 4000, scheme: "http" });
  });

  it("detects HTTPS URLs", () => {
    const result = detectPreviewUrl("Ready on https://localhost:3000");
    expect(result).toEqual({ url: "https://localhost:3000/", port: 3000, scheme: "https" });
  });

  it("detects URLs with paths", () => {
    const result = detectPreviewUrl("Visit http://localhost:3000/admin");
    expect(result).toEqual({ url: "http://localhost:3000/admin", port: 3000, scheme: "http" });
  });

  it("rejects localhost URL without port", () => {
    expect(detectPreviewUrl("Server running at http://localhost")).toBeNull();
  });

  it("rejects 127.0.0.1 URL without port", () => {
    expect(detectPreviewUrl("Available at http://127.0.0.1")).toBeNull();
  });

  it("rejects 0.0.0.0 URL without port", () => {
    expect(detectPreviewUrl("Listening on http://0.0.0.0")).toBeNull();
  });
});

describe("detectPreviewUrl - host:port patterns", () => {
  it("detects localhost:port pattern", () => {
    const result = detectPreviewUrl("Server started on localhost:3000");
    expect(result).toEqual({ url: BARE_LOCALHOST_3000_URL, port: 3000, scheme: "http" });
  });

  it("detects 127.0.0.1:port pattern", () => {
    const result = detectPreviewUrl("Bound to 127.0.0.1:8080");
    expect(result).toEqual({ url: "http://127.0.0.1:8080", port: 8080, scheme: "http" });
  });

  it("detects 0.0.0.0:port pattern", () => {
    const result = detectPreviewUrl("Listening 0.0.0.0:4000");
    expect(result).toEqual({ url: "http://0.0.0.0:4000", port: 4000, scheme: "http" });
  });

  it("infers https from context", () => {
    const result = detectPreviewUrl("HTTPS server on localhost:3000");
    expect(result).toEqual({ url: "https://localhost:3000", port: 3000, scheme: "https" });
  });

  it("handles multi-digit ports", () => {
    const result = detectPreviewUrl("Running on localhost:12345");
    expect(result).toEqual({ url: "http://localhost:12345", port: 12345, scheme: "http" });
  });
});

describe("detectPreviewUrl - edge cases", () => {
  it("returns null for empty string", () => {
    expect(detectPreviewUrl("")).toBeNull();
  });

  it("returns null for non-matching text", () => {
    expect(detectPreviewUrl("Server is starting...")).toBeNull();
  });

  it("returns null for invalid URLs", () => {
    expect(detectPreviewUrl("Invalid: http://[::1]:abc")).toBeNull();
  });

  it("handles URLs with special characters", () => {
    const result = detectPreviewUrl("Ready: http://localhost:3000?debug=true");
    expect(result?.url).toBe("http://localhost:3000/?debug=true");
  });

  it("handles multiple URLs in one line (returns first valid)", () => {
    const result = detectPreviewUrl("Server at http://localhost:3000 and http://localhost:3001");
    expect(result?.port).toBe(3000);
  });

  it("handles ANSI color codes", () => {
    const result = detectPreviewUrl("\x1b[32mRunning at http://localhost:3000\x1b[0m");
    expect(result?.port).toBe(3000);
  });
});

describe("detectPreviewUrl - numeric port validation", () => {
  it.each(
    LOCAL_HOSTS.flatMap((host) =>
      INVALID_PORTS.flatMap((port) => [`${host}:${port}`, `http://${host}:${port}`]),
    ),
  )("rejects overflow and overlong numeric port tokens: %s", (candidate) => {
    expect(detectPreviewUrl(candidate)).toBeNull();
  });

  it.each(LOCAL_HOSTS)("accepts the upper numeric boundary for %s", (host) => {
    expect(detectPreviewUrl(`${host}:65535`)).toEqual({
      url: `http://${host}:65535`,
      port: 65535,
      scheme: "http",
    });
    expect(detectPreviewUrl(`http://${host}:65535`)).toEqual({
      url: `http://${host}:65535/`,
      port: 65535,
      scheme: "http",
    });
  });

  it.each([
    ["http://localhost:0", "http://localhost:0/", 0],
    ["localhost:00", "http://localhost:00", 0],
    ["http://localhost:1", "http://localhost:1/", 1],
    ["localhost:10", "http://localhost:10", 10],
    ["localhost:03000", "http://localhost:03000", 3000],
    ["http://localhost:0065535", "http://localhost:65535/", 65535],
    ["http://localhost:80", "http://localhost:80", 80],
    ["https://localhost:443", "https://localhost:443", 443],
  ])("preserves existing lower-bound and normalization behavior: %s", (input, url, port) => {
    expect(detectPreviewUrl(input as string)).toMatchObject({ url, port });
  });

  it.each(["localhost:1", "localhost:0065535", "localhost:000000"])(
    "does not accept a partial bare token: %s",
    (candidate) => {
      expect(detectPreviewUrl(candidate)).toBeNull();
    },
  );

  it.each([
    ["localhost:3000 localhost:70000", BARE_LOCALHOST_3000_URL],
    ["localhost:70000 localhost:3000 localhost:3001", "http://localhost:3001"],
    ["localhost:3000 localhost:3001 localhost:70000", "http://localhost:3001"],
    ["localhost:3001 http://localhost:3000 localhost:70000", LOCALHOST_3000_URL],
    ["http://localhost:3000 localhost:3001 localhost:70000", LOCALHOST_3000_URL],
    ["http://localhost:65536 http://localhost:3000 http://localhost:3001", LOCALHOST_3000_URL],
    ["http://localhost:3000 http://localhost:3001 http://localhost:65536", LOCALHOST_3000_URL],
    ["http://localhost:70000 localhost:3000 localhost:123456", BARE_LOCALHOST_3000_URL],
    ["localhost:3000 http://localhost:70000", BARE_LOCALHOST_3000_URL],
    ["HTTPS localhost:3000 localhost:70000", "https://localhost:3000"],
  ])("preserves full-first and last-valid-bare preference: %s", (input, expected) => {
    expect(detectPreviewUrl(input)?.url).toBe(expected);
  });
});

describe("detectPreviewUrl - real-world examples", () => {
  it("detects Next.js dev server", () => {
    expect(detectPreviewUrl("  ▲ Local:        http://localhost:3000")?.url).toBe(
      LOCALHOST_3000_URL,
    );
  });

  it("detects Vite dev server", () => {
    expect(detectPreviewUrl("  ➜  Local:   http://localhost:5173/")?.url).toBe(
      "http://localhost:5173/",
    );
  });

  it("detects Create React App", () => {
    expect(detectPreviewUrl("On Your Network:  http://localhost:3000")?.url).toBe(
      LOCALHOST_3000_URL,
    );
  });

  it("detects Rails server", () => {
    expect(detectPreviewUrl("* Listening on http://127.0.0.1:3000")?.url).toBe(
      "http://127.0.0.1:3000/",
    );
  });

  it("detects Django dev server", () => {
    expect(detectPreviewUrl("Starting development server at http://127.0.0.1:8000/")?.url).toBe(
      "http://127.0.0.1:8000/",
    );
  });

  it("detects Express server", () => {
    expect(detectPreviewUrl("Server listening on localhost:3000")?.url).toBe(
      BARE_LOCALHOST_3000_URL,
    );
  });

  it("detects Flask dev server", () => {
    expect(detectPreviewUrl(" * Running on http://127.0.0.1:5000")?.url).toBe(
      "http://127.0.0.1:5000/",
    );
  });
});

describe("detectPreviewUrlFromOutput", () => {
  it.each(INVALID_LOCALHOST_CANDIDATES)(
    "retains the working proxy after invalid later output: %s",
    (invalidCandidate) => {
      const workingUrl = "http://localhost:3000/app?debug=true#route";
      const detected = detectPreviewUrlFromOutput(
        `Ready: ${workingUrl}\nRetry: ${invalidCandidate}`,
      );
      expect(detected).toBe(workingUrl);
      expect(rewritePreviewUrlForProxy(detected!, "test-session-123")).toBe(
        "http://localhost:8080/port-proxy/test-session-123/3000/app?debug=true#route",
      );
    },
  );

  it.each(["localhost:65535", "http://localhost:65535"])(
    "rewrites an upper-bound output candidate through the real pipeline: %s",
    (candidate) => {
      const detected = detectPreviewUrlFromOutput(`Ready: ${candidate}`);
      expect(detected).not.toBeNull();
      expect(rewritePreviewUrlForProxy(detected!, "test-session-123")).toBe(
        "http://localhost:8080/port-proxy/test-session-123/65535/",
      );
    },
  );

  it.each(INVALID_LOCALHOST_CANDIDATES)("returns null for invalid-only output: %s", (candidate) => {
    expect(detectPreviewUrlFromOutput(`Starting...\nListening: ${candidate}\nReady!`)).toBeNull();
  });

  it("returns null for empty output", () => {
    expect(detectPreviewUrlFromOutput("")).toBeNull();
  });

  it("finds URL in multi-line output", () => {
    const output = `
Starting server...
Compiling...
Server running at http://localhost:3000
Ready!
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe(LOCALHOST_3000_URL);
  });

  it("returns the last valid URL when multiple exist", () => {
    const output = `
Starting on http://localhost:3000
Error: Port in use
Starting on http://localhost:3001
Ready!
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe("http://localhost:3001/");
  });

  it("skips invalid URLs and finds valid ones", () => {
    const output = `
Trying http://localhost
Failed
Trying http://localhost:3000
Success!
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe(LOCALHOST_3000_URL);
  });

  it("handles output with no valid URLs", () => {
    const output = `
Server starting...
Initializing...
Done
    `;
    expect(detectPreviewUrlFromOutput(output)).toBeNull();
  });

  it("handles real Next.js output", () => {
    const output = `
  ▲ Next.js 14.0.0
  - Local:        http://localhost:3000
  - Environments: .env

 ✓ Ready in 1.5s
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe(LOCALHOST_3000_URL);
  });

  it("handles real Vite output", () => {
    const output = `
  VITE v5.0.0  ready in 500 ms

  ➜  Local:   http://localhost:5173/
  ➜  Network: use --host to expose
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe("http://localhost:5173/");
  });

  it("ignores URLs without ports mixed with valid ones", () => {
    const output = `
Checking http://localhost
Port available
Starting on localhost:3000
Ready!
    `;
    expect(detectPreviewUrlFromOutput(output)).toBe(BARE_LOCALHOST_3000_URL);
  });
});

describe("rewritePreviewUrlForProxy", () => {
  const SESSION_ID = "test-session-123";
  const proxyPath = (port: number, path: string) =>
    `http://localhost:8080/port-proxy/${SESSION_ID}/${port}${path}`;

  it("rewrites localhost URL through the proxy", () => {
    expect(rewritePreviewUrlForProxy(LOCALHOST_3000_URL, SESSION_ID)).toBe(proxyPath(3000, "/"));
  });

  it("keeps an existing backend proxy URL unchanged", () => {
    const proxiedUrl = proxyPath(3000, "/app?debug=true");
    expect(rewritePreviewUrlForProxy(proxiedUrl, SESSION_ID)).toBe(proxiedUrl);
  });

  it("preserves path and query string", () => {
    expect(rewritePreviewUrlForProxy("http://localhost:8080/api/test?debug=true", SESSION_ID)).toBe(
      proxyPath(8080, "/api/test?debug=true"),
    );
  });

  it("preserves hash fragment", () => {
    expect(rewritePreviewUrlForProxy("http://localhost:3000/app#/route", SESSION_ID)).toBe(
      proxyPath(3000, "/app#/route"),
    );
  });

  it("handles 127.0.0.1 URLs", () => {
    expect(rewritePreviewUrlForProxy("http://127.0.0.1:5000/", SESSION_ID)).toBe(
      proxyPath(5000, "/"),
    );
  });

  it("handles 0.0.0.0 URLs", () => {
    expect(rewritePreviewUrlForProxy("http://0.0.0.0:4000/", SESSION_ID)).toBe(
      proxyPath(4000, "/"),
    );
  });

  it("returns null for URLs without a port", () => {
    expect(rewritePreviewUrlForProxy("http://localhost/", SESSION_ID)).toBeNull();
  });

  it("returns null for non-localhost URLs", () => {
    expect(rewritePreviewUrlForProxy("https://example.com:443/", SESSION_ID)).toBeNull();
  });

  it("returns null for invalid URLs", () => {
    expect(rewritePreviewUrlForProxy("not-a-url", SESSION_ID)).toBeNull();
  });
});
