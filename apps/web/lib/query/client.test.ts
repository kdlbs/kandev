import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import { getBrowserQueryClient, isAuthError, makeQueryClient } from "./client";

describe("makeQueryClient", () => {
  it("uses Kandev defaults for server-state reads and mutations", () => {
    const client = makeQueryClient();

    expect(client.getDefaultOptions().queries).toMatchObject({
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
    });
    expect(client.getDefaultOptions().mutations).toMatchObject({ retry: 0 });
  });

  it("does not retry auth failures", () => {
    const client = makeQueryClient();
    const retry = client.getDefaultOptions().queries?.retry;

    expect(typeof retry).toBe("function");
    expect((retry as (failureCount: number, error: unknown) => boolean)(0, new Error("net"))).toBe(
      true,
    );
    expect(
      (retry as (failureCount: number, error: unknown) => boolean)(
        0,
        new ApiError("forbidden", 403, null),
      ),
    ).toBe(false);
    expect((retry as (failureCount: number, error: unknown) => boolean)(2, new Error("net"))).toBe(
      false,
    );
  });
});

describe("isAuthError", () => {
  it("identifies 401 and 403 API errors", () => {
    expect(isAuthError(new ApiError("unauthorized", 401, null))).toBe(true);
    expect(isAuthError(new ApiError("forbidden", 403, null))).toBe(true);
    expect(isAuthError(new ApiError("missing", 404, null))).toBe(false);
    expect(isAuthError(new Error("forbidden"))).toBe(false);
  });
});

describe("getBrowserQueryClient", () => {
  it("reuses one browser client", () => {
    expect(getBrowserQueryClient()).toBe(getBrowserQueryClient());
  });
});
