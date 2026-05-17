import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { waitForServiceHealth } from "./health_check";

describe("waitForServiceHealth", () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(process.stderr, "write").mockImplementation(() => true);
  });

  afterEach(() => {
    vi.useRealTimers();
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("resolves once /health responds with 200", async () => {
    let calls = 0;
    global.fetch = vi.fn(async () => {
      calls += 1;
      if (calls < 3) throw new Error("ECONNREFUSED");
      return new Response("ok", { status: 200 });
    }) as unknown as typeof fetch;

    const dump = vi.fn();
    const p = waitForServiceHealth(38429, dump);
    // Advance through 2 failed polls + 1 success
    await vi.advanceTimersByTimeAsync(3000);
    await p;
    expect(calls).toBeGreaterThanOrEqual(3);
    expect(dump).not.toHaveBeenCalled();
  });

  it("times out and dumps logs when /health never responds", async () => {
    global.fetch = vi.fn(async () => {
      throw new Error("ECONNREFUSED");
    }) as unknown as typeof fetch;

    const dump = vi.fn();
    const p = waitForServiceHealth(38429, dump);
    const expected = expect(p).rejects.toThrow(/never responded/);
    // Push past the 30s deadline
    await vi.advanceTimersByTimeAsync(31_000);
    await expected;
    expect(dump).toHaveBeenCalledTimes(1);
  });

  it("uses the default port when none is given", async () => {
    const seenUrls: string[] = [];
    global.fetch = vi.fn(async (url: string | URL | Request) => {
      seenUrls.push(typeof url === "string" ? url : url.toString());
      return new Response("ok", { status: 200 });
    }) as unknown as typeof fetch;

    const dump = vi.fn();
    const p = waitForServiceHealth(undefined, dump);
    await vi.advanceTimersByTimeAsync(500);
    await p;
    expect(seenUrls[0]).toContain(":38429/health");
  });
});
