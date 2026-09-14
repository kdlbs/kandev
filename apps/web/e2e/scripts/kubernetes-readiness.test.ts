// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { inClusterBackendPod, waitForInClusterBackendReady } from "../fixtures/kubernetes-tools";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("in-cluster backend readiness", () => {
  it("uses application readiness for the pod readiness probe", () => {
    expect(inClusterBackendPod("fixture:image")).toMatch(
      /readinessProbe:\s+httpGet:\s+path: \/ready\s/,
    );
  });

  it("does not return while the listener is live but application startup is incomplete", async () => {
    vi.useFakeTimers();
    let applicationReady = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => {
        const pathname = new URL(url).pathname;
        if (pathname !== "/health" && pathname !== "/ready") {
          return new Response(null, { status: 404 });
        }
        const ready = pathname === "/health" || applicationReady;
        return new Response(null, { status: ready ? 200 : 503 });
      }),
    );

    let returned = false;
    const startup = waitForInClusterBackendReady("http://fixture.test").then(() => {
      returned = true;
    });
    await vi.advanceTimersByTimeAsync(0);
    const returnedBeforeReady = returned;
    applicationReady = true;
    await vi.advanceTimersByTimeAsync(250);
    await startup;

    expect(returnedBeforeReady).toBe(false);
    expect(returned).toBe(true);
  });
});
