import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockExecuteUtilityPrompt = vi.fn();
const mockToast = vi.fn();

vi.mock("@/lib/api/domains/utility-api", () => ({
  executeUtilityPrompt: (...args: unknown[]) => mockExecuteUtilityPrompt(...args),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));
vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => undefined,
}));

import { useUtilityAgentGenerator } from "./use-utility-agent-generator";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("useUtilityAgentGenerator enhancePrompt", () => {
  it("keeps enhance loading until the successful result is delivered", async () => {
    mockExecuteUtilityPrompt.mockResolvedValue({
      success: true,
      response: "improved prompt",
      call_id: "call-123",
      duration_ms: 80_000,
    });
    let releaseDelivery!: () => void;
    const delivered = new Promise<boolean>((resolve) => {
      releaseDelivery = () => resolve(true);
    });
    const { result } = renderHook(() => useUtilityAgentGenerator({ sessionId: null }));

    let pending!: Promise<void>;
    act(() => {
      pending = result.current.enhancePrompt("original", async (value) => {
        expect(value).toEqual({
          content: "improved prompt",
          callId: "call-123",
          durationMs: 80_000,
        });
        expect(result.current.isEnhancingPrompt).toBe(true);
        return delivered;
      });
    });
    expect(result.current.isEnhancingPrompt).toBe(true);
    releaseDelivery();
    await act(async () => {
      await pending;
    });

    expect(result.current.isEnhancingPrompt).toBe(false);
  });

  it("does not toast when delivery declines the result", async () => {
    mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "improved" });
    const { result } = renderHook(() => useUtilityAgentGenerator({ sessionId: null }));

    await act(async () => {
      await result.current.enhancePrompt("original", () => false);
    });

    expect(mockToast).not.toHaveBeenCalled();
  });

  it("retains API rejection toast behavior", async () => {
    mockExecuteUtilityPrompt.mockRejectedValue(new Error("network down"));
    const { result } = renderHook(() => useUtilityAgentGenerator({ sessionId: null }));

    await act(async () => {
      await result.current.enhancePrompt("original", () => true);
    });

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Generation failed", description: "network down" }),
    );
    expect(result.current.isEnhancingPrompt).toBe(false);
  });

  it("retains unsuccessful response toast behavior", async () => {
    mockExecuteUtilityPrompt.mockResolvedValue({ success: false, error: "bad response" });
    const { result } = renderHook(() => useUtilityAgentGenerator({ sessionId: null }));

    await act(async () => {
      await result.current.enhancePrompt("original", () => true);
    });

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Generation failed", description: "bad response" }),
    );
    expect(result.current.isEnhancingPrompt).toBe(false);
  });
});
