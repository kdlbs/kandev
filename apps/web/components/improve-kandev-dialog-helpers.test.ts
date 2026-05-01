import { describe, it, expect, vi, beforeEach } from "vitest";

import { buildImproveKandevDescription } from "./improve-kandev-dialog-helpers";
import type { ImproveKandevBootstrapResponse } from "@/lib/api/domains/improve-kandev-api";

const uploadFrontendLog = vi.fn();
vi.mock("@/lib/api/domains/improve-kandev-api", async (orig) => {
  const actual = await orig<typeof import("@/lib/api/domains/improve-kandev-api")>();
  return { ...actual, uploadFrontendLog: (...args: unknown[]) => uploadFrontendLog(...args) };
});

vi.mock("@/lib/logger/buffer", () => ({
  snapshotLogs: () => [
    { timestamp: "2026-04-29T10:00:00Z", level: "info", source: "console", message: "hi" },
  ],
}));

const bootstrap: ImproveKandevBootstrapResponse = {
  repository_id: "r1",
  workflow_id: "w1",
  branch: "main",
  bundle_dir: "/tmp/kandev-improve-abc",
  bundle_files: {
    metadata: "/tmp/kandev-improve-abc/metadata.json",
    backend_log: "/tmp/kandev-improve-abc/backend.log",
    frontend_log: "/tmp/kandev-improve-abc/frontend.log",
  },
  github_login: "octocat",
  has_write_access: false,
  fork_status: "unknown",
};

describe("buildImproveKandevDescription", () => {
  beforeEach(() => {
    uploadFrontendLog.mockReset();
    uploadFrontendLog.mockResolvedValue({ path: bootstrap.bundle_files.frontend_log });
  });

  it("returns description unchanged when bootstrap is null", async () => {
    const out = await buildImproveKandevDescription("desc", null, true);
    expect(out).toBe("desc");
    expect(uploadFrontendLog).not.toHaveBeenCalled();
  });

  it("returns description unchanged when captureLogs is false", async () => {
    const out = await buildImproveKandevDescription("desc", bootstrap, false);
    expect(out).toBe("desc");
    expect(uploadFrontendLog).not.toHaveBeenCalled();
  });

  it("appends bundle file paths and uploads frontend log when captureLogs=true", async () => {
    const out = await buildImproveKandevDescription("Original prompt", bootstrap, true);
    expect(out).toContain("Original prompt");
    expect(out).toContain("Context bundle for the agent:");
    expect(out).toContain(bootstrap.bundle_files.metadata);
    expect(out).toContain(bootstrap.bundle_files.backend_log);
    expect(out).toContain(bootstrap.bundle_files.frontend_log);
    expect(uploadFrontendLog).toHaveBeenCalledWith(
      bootstrap.bundle_dir,
      expect.arrayContaining([expect.objectContaining({ message: "hi" })]),
    );
  });

  it("does not abort when frontend log upload fails", async () => {
    uploadFrontendLog.mockRejectedValueOnce(new Error("network down"));
    await expect(buildImproveKandevDescription("desc", bootstrap, true)).resolves.toContain(
      "Context bundle for the agent:",
    );
  });

  it("omits the frontend_log path when upload fails", async () => {
    uploadFrontendLog.mockRejectedValueOnce(new Error("network down"));
    const out = await buildImproveKandevDescription("desc", bootstrap, true);
    expect(out).toContain(bootstrap.bundle_files.metadata);
    expect(out).toContain(bootstrap.bundle_files.backend_log);
    expect(out).not.toContain(bootstrap.bundle_files.frontend_log);
  });
});
