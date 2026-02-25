import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { waitForUrlReady } from "./health";

type ProcState = { exitCode: number | null };

const fetchMock = vi.fn();

describe("waitForUrlReady", () => {
  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns once web URL is reachable", async () => {
    const proc: ProcState = { exitCode: null };
    fetchMock
      .mockRejectedValueOnce(new Error("ECONNREFUSED"))
      .mockResolvedValue({ status: 404 } as Response);

    await expect(waitForUrlReady("http://localhost:3000", proc, 1000)).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("fails when process exits before URL becomes reachable", async () => {
    const proc: ProcState = { exitCode: 1 };
    fetchMock.mockRejectedValue(new Error("ECONNREFUSED"));

    await expect(waitForUrlReady("http://localhost:3000", proc, 1000)).rejects.toThrow(
      "Web process exited before URL became reachable",
    );
  });

  it("fails on timeout when URL stays unreachable", async () => {
    const proc: ProcState = { exitCode: null };
    fetchMock.mockRejectedValue(new Error("ECONNREFUSED"));

    await expect(waitForUrlReady("http://localhost:3000", proc, 120)).rejects.toThrow(
      "Web URL readiness timed out",
    );
  });
});
