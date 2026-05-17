import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { waitForServiceHealth } from "./health_check";

describe("waitForServiceHealth", () => {
  const originalFetch = global.fetch;

  const originalTimeout = process.env.KANDEV_HEALTH_TIMEOUT_MS;

  beforeEach(() => {
    // The error message in waitForServiceHealth suggests setting this env var
    // for slow first-launch scenarios. If a developer has it exported in their
    // shell, the timeout test would block waiting for the overridden deadline
    // instead of the 30s default we're asserting against.
    delete process.env.KANDEV_HEALTH_TIMEOUT_MS;
    vi.useFakeTimers();
    vi.spyOn(process.stderr, "write").mockImplementation(() => true);
  });

  afterEach(() => {
    if (originalTimeout === undefined) delete process.env.KANDEV_HEALTH_TIMEOUT_MS;
    else process.env.KANDEV_HEALTH_TIMEOUT_MS = originalTimeout;
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

  it("passes an AbortSignal so a hung fetch can't outrun the deadline", async () => {
    let lastSignal: AbortSignal | undefined;
    global.fetch = vi.fn(async (_url: string | URL | Request, init?: RequestInit) => {
      lastSignal = init?.signal ?? undefined;
      return new Response("ok", { status: 200 });
    }) as unknown as typeof fetch;

    const dump = vi.fn();
    const p = waitForServiceHealth(undefined, dump);
    await vi.advanceTimersByTimeAsync(500);
    await p;
    // The exact timeout value is an implementation detail; what matters is
    // that some abortable signal is passed through.
    expect(lastSignal).toBeInstanceOf(AbortSignal);
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
