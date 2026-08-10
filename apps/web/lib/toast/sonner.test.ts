import fs from "node:fs";
import path from "node:path";

import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const order: string[] = [];
  return {
    order,
    upstreamError: vi.fn(() => {
      order.push("delegate");
      return "toast-id";
    }),
    report: vi.fn(() => order.push("report")),
  };
});

vi.mock("sonner", () => ({
  toast: Object.assign(vi.fn(), {
    error: mocks.upstreamError,
    success: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  }),
}));

vi.mock("@/lib/api/domains/frontend-error-log-api", () => ({
  scheduleFrontendErrorReport: mocks.report,
}));

import { createPluginToast, toast } from "./sonner";

describe("reporting Sonner seam", () => {
  beforeEach(() => {
    mocks.order.length = 0;
    vi.clearAllMocks();
  });

  it("delegates error rendering before scheduling exactly one report", () => {
    const result = toast.error("Failed to save", {
      description: "The backend rejected the update",
    });

    expect(result).toBe("toast-id");
    expect(mocks.order).toEqual(["delegate", "report"]);
    expect(mocks.report).toHaveBeenCalledOnce();
    expect(mocks.report).toHaveBeenCalledWith({
      source: "sonner",
      title: "Failed to save",
      description: "The backend rejected the update",
      error: undefined,
    });
  });

  // A plugin error toast is the plugin's problem, not a kandev application
  // error. Reporting it would file an Error-level backend log entry per
  // occurrence — every 60s for a polling plugin — with no way to tell it
  // apart from a real one. (The backend also rejects any source outside
  // "sonner" | "toast-provider" with 400, so attributing it there is not
  // available without a cross-stack change.)
  it("does not file a frontend error report for a plugin toast", () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const pluginToast = createPluginToast("kandev-plugin-github-status");

    const result = pluginToast.error("Poll failed");

    expect(result).toBe("toast-id");
    expect(mocks.upstreamError).toHaveBeenCalledOnce();
    expect(mocks.report).not.toHaveBeenCalled();
    expect(mocks.order).toEqual(["delegate"]);
    consoleError.mockRestore();
  });

  it("logs a plugin toast error to the console with plugin attribution", () => {
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const pluginToast = createPluginToast("kandev-plugin-github-status");

    pluginToast.error("Poll failed");

    expect(consoleError).toHaveBeenCalledWith(
      '[plugins] toast.error from "kandev-plugin-github-status":',
      "Poll failed",
    );
    consoleError.mockRestore();
  });

  it("leaves non-error plugin toast methods delegating straight to sonner", () => {
    const pluginToast = createPluginToast("kandev-plugin-github-status");

    pluginToast.success("Synced");

    expect(mocks.report).not.toHaveBeenCalled();
  });

  it("keeps reporting for first-party call sites", () => {
    toast.error("Real app failure");

    expect(mocks.report).toHaveBeenCalledOnce();
    expect(mocks.report).toHaveBeenCalledWith({
      source: "sonner",
      title: "Real app failure",
      description: undefined,
      error: undefined,
    });
  });

  it("prevents application call sites from bypassing the reporting seam", () => {
    const webRoot = path.resolve(process.cwd());
    const wrapper = path.join(webRoot, "lib", "toast", "sonner.ts");
    const bypasses = sourceFiles(webRoot).filter((file) => {
      if (file === wrapper) return false;
      if (/\.test\.(ts|tsx)$/.test(file)) return false;
      return /import\s+\{\s*toast\s*\}\s+from\s+["']sonner["']/.test(fs.readFileSync(file, "utf8"));
    });
    expect(bypasses).toEqual([]);
  });
});

function sourceFiles(root: string): string[] {
  const files: string[] = [];
  for (const entry of fs.readdirSync(root, { withFileTypes: true })) {
    if (entry.name === "node_modules" || entry.name === ".next") continue;
    const fullPath = path.join(root, entry.name);
    if (entry.isDirectory()) {
      files.push(...sourceFiles(fullPath));
    } else if (/\.(ts|tsx)$/.test(entry.name)) {
      files.push(fullPath);
    }
  }
  return files;
}
