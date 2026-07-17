// @vitest-environment happy-dom
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getPluginConfig = vi.fn();
const updatePluginConfig = vi.fn();
vi.mock("@/lib/api/domains/plugins-api", () => ({
  getPluginConfig: (...args: unknown[]) => getPluginConfig(...args),
  updatePluginConfig: (...args: unknown[]) => updatePluginConfig(...args),
}));

const toastError = vi.fn();
const toastSuccess = vi.fn();
const toastWarning = vi.fn();
vi.mock("sonner", () => ({
  toast: {
    error: (...args: unknown[]) => toastError(...args),
    success: (...args: unknown[]) => toastSuccess(...args),
    warning: (...args: unknown[]) => toastWarning(...args),
  },
}));

import { SECRET_MASK } from "@/lib/plugins/config-schema";
import { usePluginConfigForm } from "./use-plugin-config-form";
import type { PluginRecord } from "@/lib/types/plugins";

function testPlugin(): PluginRecord {
  return {
    id: "kandev-plugin-github",
    api_version: 1,
    version: "1.0.0",
    display_name: "GitHub",
    description: "",
    author: "kandev",
    categories: [],
    capabilities: {},
    status: "active",
    install_path: "/tmp/x",
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    config_schema: {
      type: "object",
      required: ["github_token"],
      properties: {
        github_token: { type: "string", secret: true },
        org: { type: "string" },
      },
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  getPluginConfig.mockResolvedValue({});
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("usePluginConfigForm", () => {
  it("loads the stored config into initial values", async () => {
    getPluginConfig.mockResolvedValue({ github_token: SECRET_MASK, org: "kdlbs" });

    const { result } = renderHook(() => usePluginConfigForm(testPlugin()));

    await waitFor(() => expect(result.current.configLoading).toBe(false));
    expect(getPluginConfig).toHaveBeenCalledWith("kandev-plugin-github", { cache: "no-store" });
    expect(result.current.values.github_token).toBe(SECRET_MASK);
    expect(result.current.values.org).toBe("kdlbs");
    expect(result.current.isDirty).toBe(false);
  });

  it("does not fetch for a null plugin or an empty schema", () => {
    renderHook(() => usePluginConfigForm(null));
    renderHook(() => usePluginConfigForm({ ...testPlugin(), config_schema: undefined }));
    expect(getPluginConfig).not.toHaveBeenCalled();
  });

  it("tracks dirtiness across change and save", async () => {
    getPluginConfig.mockResolvedValue({});
    updatePluginConfig.mockResolvedValue({ updated: true });

    const { result } = renderHook(() => usePluginConfigForm(testPlugin()));
    await waitFor(() => expect(result.current.configLoading).toBe(false));

    act(() => result.current.handleChange("github_token", "ghp_x"));
    expect(result.current.isDirty).toBe(true);

    getPluginConfig.mockResolvedValue({ github_token: SECRET_MASK });
    await act(() => result.current.handleSave());
    expect(result.current.isDirty).toBe(false);
    expect(result.current.saveStatus).toBe("success");
  });

  it("rejects a save with missing required fields without calling the API", async () => {
    const { result } = renderHook(() => usePluginConfigForm(testPlugin()));
    await waitFor(() => expect(result.current.configLoading).toBe(false));

    await act(() => result.current.handleSave());

    expect(updatePluginConfig).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith(expect.stringContaining("Required"));
  });

  it("saves, refetches the masked config, and reports success", async () => {
    updatePluginConfig.mockResolvedValue({ updated: true });
    const { result } = renderHook(() => usePluginConfigForm(testPlugin()));
    await waitFor(() => expect(result.current.configLoading).toBe(false));

    act(() => result.current.handleChange("github_token", "ghp_cleartext"));
    getPluginConfig.mockResolvedValue({ github_token: SECRET_MASK });
    await act(() => result.current.handleSave());

    expect(updatePluginConfig).toHaveBeenCalledWith("kandev-plugin-github", {
      github_token: "ghp_cleartext",
    });
    expect(result.current.values.github_token).toBe(SECRET_MASK);
    expect(result.current.saveStatus).toBe("success");
    expect(toastSuccess).toHaveBeenCalled();
  });

  it("reports a PATCH failure as an error and keeps saveStatus=error", async () => {
    updatePluginConfig.mockRejectedValue(new Error("boom"));
    const { result } = renderHook(() => usePluginConfigForm(testPlugin()));
    await waitFor(() => expect(result.current.configLoading).toBe(false));

    act(() => result.current.handleChange("github_token", "ghp_x"));
    await act(() => result.current.handleSave());

    expect(result.current.saveStatus).toBe("error");
    expect(toastError).toHaveBeenCalledWith("boom");
  });

  it("treats a refetch failure after a successful PATCH as saved — and masks typed secrets", async () => {
    updatePluginConfig.mockResolvedValue({ updated: true });
    const { result } = renderHook(() => usePluginConfigForm(testPlugin()));
    await waitFor(() => expect(result.current.configLoading).toBe(false));

    act(() => result.current.handleChange("github_token", "ghp_cleartext"));
    getPluginConfig.mockRejectedValue(new Error("plugin restarting"));
    await act(() => result.current.handleSave());

    // The save succeeded: never report it as a failure...
    expect(result.current.saveStatus).toBe("success");
    expect(toastWarning).toHaveBeenCalled();
    expect(toastError).not.toHaveBeenCalled();
    // ...and the cleartext secret must not linger in the form.
    expect(result.current.values.github_token).toBe(SECRET_MASK);
  });
});
