import { createElement, type ReactNode } from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { useForgejoActionPresets } from "./use-forgejo-action-presets";

const api = vi.hoisted(() => ({ list: vi.fn(), save: vi.fn(), remove: vi.fn() }));
vi.mock("@/lib/api/domains/forgejo-api", () => ({
  listForgejoActionPresets: api.list,
  saveForgejoActionPreset: api.save,
  deleteForgejoActionPreset: api.remove,
}));
function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}
const preset = {
  id: "preset-a",
  workspace_id: "ws-a",
  kind: "review",
  name: "Review",
  instructions: "Inspect tests",
  created_at: "",
  updated_at: "",
};

describe("useForgejoActionPresets", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({ presets: [preset] });
  });
  it("loads and refreshes saved presets after deletion", async () => {
    api.remove.mockResolvedValue({ deleted: true });
    const { result } = renderHook(() => useForgejoActionPresets("ws-a"), { wrapper });
    await waitFor(() => expect(result.current.presets).toEqual([preset]));
    await result.current.remove(preset.id);
    expect(api.remove).toHaveBeenCalledWith(preset.id, { workspaceId: "ws-a" });
    expect(api.list).toHaveBeenCalledTimes(2);
  });
});
