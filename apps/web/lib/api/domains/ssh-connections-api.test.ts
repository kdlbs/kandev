import { afterEach, describe, expect, it, vi } from "vitest";
vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));
import { createSSHIdentity, trustManagedSSH } from "./ssh-connections-api";
import type { SSHRoute } from "@/lib/types/http-ssh-connections";
afterEach(() => vi.unstubAllGlobals());
describe("managed SSH requests", () => {
  it("retains the required mutation body and content type when options are provided", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    const request = { request_id: "request", kind: "generate" as const, label: "test" };
    await createSSHIdentity(request, {
      init: { method: "GET", body: "wrong", headers: { "X-Test": "yes" } },
    });
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe("http://api.test/api/v1/ssh/managed/identities");
    expect(options.method).toBe("POST");
    expect(JSON.parse(options.body)).toEqual(request);
    expect(new Headers(options.headers).get("Content-Type")).toBe("application/json");
  });
  it("binds trust to the observed route revision and one-use receipt", async () => {
    const fetch = vi.fn().mockResolvedValue(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    await trustManagedSSH({ destination: { id: "target" }, stamp: "current" } as SSHRoute, {
      receipt: "receipt",
      fingerprint: "SHA256:key",
      public_key: "public",
      stamp: "observed",
    });
    expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({
      expected_stamp: "observed",
      receipt: "receipt",
      fingerprint: "SHA256:key",
    });
  });
});
