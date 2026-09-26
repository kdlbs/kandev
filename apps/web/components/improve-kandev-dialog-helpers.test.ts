import { describe, it, expect, vi, beforeEach } from "vitest";

import { buildImproveKandevDescription } from "./improve-kandev-dialog-helpers";
import type { ImproveKandevBootstrapResponse } from "@/lib/api/domains/improve-kandev-api";

const leaseDiagnosticBundle = vi.fn();
const createDiagnosticBundle = vi.fn();
const fetchDiagnosticBundle = vi.fn();
vi.mock("@/lib/api/domains/improve-kandev-api", async (orig) => {
  const actual = await orig<typeof import("@/lib/api/domains/improve-kandev-api")>();
  return {
    ...actual,
    leaseDiagnosticBundle: (...args: unknown[]) => leaseDiagnosticBundle(...args),
  };
});

vi.mock("@/lib/api/domains/system-api", () => ({
  createDiagnosticBundle: (...args: unknown[]) => createDiagnosticBundle(...args),
  fetchDiagnosticBundle: (...args: unknown[]) => fetchDiagnosticBundle(...args),
}));

const bootstrap: ImproveKandevBootstrapResponse = {
  workspace_id: "ws-1",
  repository_id: "r1",
  workflow_id: "w1",
  issue_workflow_id: "w2",
  branch: "main",
  bundle_dir: "/tmp/kandev-improve-abc",
  bundle_file: "/tmp/kandev-improve-abc/diagnostic-bundle.zip",
  github_login: "octocat",
  has_write_access: false,
  fork_status: "unknown",
};

describe("buildImproveKandevDescription", () => {
  beforeEach(() => {
    leaseDiagnosticBundle.mockReset();
    createDiagnosticBundle.mockReset();
    fetchDiagnosticBundle.mockReset();
    createDiagnosticBundle.mockResolvedValue({ id: "bundle-1", status: "ready" });
    leaseDiagnosticBundle.mockResolvedValue({
      path: bootstrap.bundle_file,
      status: "ready",
      sources: ["backend", "frontend"],
    });
  });

  it("returns description unchanged when bootstrap is null", async () => {
    const out = await buildImproveKandevDescription("desc", null, true);
    expect(out).toBe("desc");
    expect(createDiagnosticBundle).not.toHaveBeenCalled();
  });

  it("returns description unchanged when captureLogs is false", async () => {
    const out = await buildImproveKandevDescription("desc", bootstrap, false);
    expect(out).toBe("desc");
    expect(createDiagnosticBundle).not.toHaveBeenCalled();
  });

  it("creates and leases an all-source diagnostic ZIP when captureLogs=true", async () => {
    const out = await buildImproveKandevDescription("Original prompt", bootstrap, true);
    expect(out).toContain("Original prompt");
    expect(out).toContain("frontend + backend logs");
    expect(out).toContain(bootstrap.bundle_file);
    expect(createDiagnosticBundle).toHaveBeenCalledWith(["backend", "frontend"]);
    expect(leaseDiagnosticBundle).toHaveBeenCalledWith(bootstrap.bundle_dir, "bundle-1");
  });

  it("does not abort or reference a missing bundle when collection fails", async () => {
    createDiagnosticBundle.mockRejectedValueOnce(new Error("network down"));
    const out = await buildImproveKandevDescription("desc", bootstrap, true);
    expect(out).toBe("desc");
    expect(out).not.toContain(bootstrap.bundle_file);
  });
});
