import { describe, expect, it } from "vitest";
import type { DiskUsageResponse } from "@/lib/types/system";
import { createDiskUsageQueryOptions, type DiskUsageQueryIdentity } from "./disk-usage-query";

const IDENTITY: DiskUsageQueryIdentity = {
  apiBaseUrl: "https://backend.example/api",
  bootId: "boot-1",
  authMode: "enabled",
  authenticated: true,
  userId: "user-1",
};

const response = (computing: boolean): DiskUsageResponse => ({
  data: null,
  computing,
  home_dir: "/data/kandev",
});

describe("disk usage query policy", () => {
  it("keeps the snapshot for the shell lifetime without implicit focus or reconnect reads", () => {
    const options = createDiskUsageQueryOptions(IDENTITY);

    expect(options).toMatchObject({
      staleTime: Infinity,
      gcTime: Infinity,
      refetchOnMount: true,
      refetchOnWindowFocus: false,
      refetchOnReconnect: false,
      retry: false,
      networkMode: "always",
      refetchIntervalInBackground: true,
    });
    const interval = options.refetchInterval;
    if (typeof interval !== "function")
      throw new Error("The disk polling interval should be conditional");
    const queryWithData = (data: DiskUsageResponse | undefined) =>
      ({ state: { data } }) as Parameters<typeof interval>[0];
    expect(interval(queryWithData(response(true)))).toBe(1500);
    expect(interval(queryWithData(response(false)))).toBe(false);
    expect(interval(queryWithData(undefined))).toBe(false);
  });
});
