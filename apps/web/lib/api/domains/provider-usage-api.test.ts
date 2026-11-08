import { describe, expect, it, vi } from "vitest";
import { fetchJson } from "../client";
import { readCachedProfileExecutionUsage } from "./provider-usage-api";

vi.mock("../client", () => ({ fetchJson: vi.fn().mockResolvedValue({}) }));

describe("safe cached usage route", () => {
  it("uses a distinct cached path and never falls back to the refreshing legacy route", async () => {
    await readCachedProfileExecutionUsage("profile/one");
    expect(fetchJson).toHaveBeenCalledWith(
      "/api/v1/agent-profiles/profile%2Fone/usage/cached",
      undefined,
    );
  });
});
