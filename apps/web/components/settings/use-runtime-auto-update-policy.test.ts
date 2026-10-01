import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentUpdateStatus } from "@/lib/api";
import type { SettingsSaveContributor } from "./settings-save-provider";
const api = vi.fn();
const refresh = vi.fn();
let contributor: SettingsSaveContributor;
let authoritative = true;
vi.mock("@/lib/api", () => ({ setAgentAutomaticUpdates: (...args: unknown[]) => api(...args) }));
vi.mock("@/lib/agents/runtime-update-statuses", () => ({
  refreshRuntimeUpdateStatuses: (...args: unknown[]) => refresh(...args),
}));
vi.mock("./settings-save-provider", () => ({
  useSettingsSaveContributor: (value: SettingsSaveContributor) => {
    contributor = value;
  },
}));
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: () => store }));
const store = {
  getState: () => ({
    agentRuntimeUpdates: { byAgent: { gemini: { auto_update: authoritative } } },
  }),
};
import { useRuntimeAutoUpdatePolicy } from "./use-runtime-auto-update-policy";
const status = {
  agent_name: "gemini",
  runtime_id: "npm:gemini",
  auto_update: false,
} as AgentUpdateStatus;
beforeEach(() => {
  vi.clearAllMocks();
  api.mockResolvedValue(undefined);
  refresh.mockResolvedValue(true);
  authoritative = true;
});
describe("saved automatic runtime consent", () => {
  // @covers AC-AGENTS-RUNTIME-NOTIFY-002.1
  it("keeps consent in a draft until Save and retains a newer edit during the request", async () => {
    let finish!: () => void;
    api.mockReturnValueOnce(
      new Promise<void>((done) => {
        finish = done;
      }),
    );
    const { result } = renderHook(() => useRuntimeAutoUpdatePolicy(status));
    act(() => result.current.setDraft(true));
    expect(result.current.isDirty).toBe(true);
    expect(api).not.toHaveBeenCalled();
    let saving!: Promise<void>;
    act(() => {
      saving = Promise.resolve(contributor.save(1));
    });
    act(() => result.current.setDraft(false));
    await act(async () => {
      finish();
      await saving;
    });
    expect(api).toHaveBeenCalledWith("gemini", true);
    expect(result.current.draft).toBe(false);
    expect(result.current.isDirty).toBe(true);
  });
  it("keeps failed consent unsaved and discards to the authoritative value", async () => {
    api.mockRejectedValueOnce(new Error("sensitive internal detail"));
    const { result } = renderHook(() => useRuntimeAutoUpdatePolicy(status));
    act(() => result.current.setDraft(true));
    await act(async () => {
      await expect(contributor.save(1)).rejects.toThrow("Unable to save automatic updates");
    });
    expect(result.current.isDirty).toBe(true);
    act(() => {
      contributor.discard();
    });
    expect(result.current.draft).toBe(false);
  });
  it("does not carry draft consent to a different runtime identity", () => {
    const { result, rerender } = renderHook(({ value }) => useRuntimeAutoUpdatePolicy(value), {
      initialProps: { value: status },
    });
    act(() => result.current.setDraft(true));
    rerender({ value: { ...status, runtime_id: "native:gemini" } });
    expect(result.current.draft).toBe(false);
    expect(result.current.isDirty).toBe(false);
  });
});
