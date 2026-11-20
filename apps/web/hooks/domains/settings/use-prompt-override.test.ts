import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const mocks = vi.hoisted(() => ({
  toast: vi.fn(),
  override: vi.fn(),
  revert: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/lib/api/domains/prompt-sync-api", () => ({
  overridePrompt: (id: string) => mocks.override(id),
  revertPromptOverride: (id: string) => mocks.revert(id),
}));

import { usePromptOverride } from "./use-prompt-override";

describe("usePromptOverride", () => {
  beforeEach(() => vi.clearAllMocks());

  it("overrides and reverts through the API and reports the outcome", async () => {
    mocks.override.mockResolvedValue({ id: "p1", source: "override" });
    mocks.revert.mockRejectedValue(new Error("boom"));
    const { result } = renderHook(() => usePromptOverride());

    await act(async () => {
      await result.current.override("p1", "house-rules");
    });
    expect(mocks.override).toHaveBeenCalledWith("p1");
    expect(mocks.toast).toHaveBeenLastCalledWith(
      expect.objectContaining({
        variant: "success",
        description: expect.stringContaining("@house-rules"),
      }),
    );

    await act(async () => {
      await result.current.revert("p1", "house-rules");
    });
    expect(mocks.revert).toHaveBeenCalledWith("p1");
    expect(mocks.toast).toHaveBeenLastCalledWith(expect.objectContaining({ variant: "error" }));
    expect(result.current.pendingId).toBeNull();
  });
});
