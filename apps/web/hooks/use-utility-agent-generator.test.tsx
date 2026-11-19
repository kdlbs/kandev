import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockExecuteUtilityPrompt = vi.fn();
const mockToast = vi.fn();
const mockUseSessionGitStatus = vi.fn();

vi.mock("@/lib/api/domains/utility-api", () => ({
  executeUtilityPrompt: (...args: unknown[]) => mockExecuteUtilityPrompt(...args),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));
vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: (...args: unknown[]) => mockUseSessionGitStatus(...args),
}));

import { useUtilityAgentGenerator } from "./use-utility-agent-generator";

beforeEach(() => {
  vi.clearAllMocks();
});

function utf8Length(value: string): number {
  return new TextEncoder().encode(value).length;
}

async function sendCommitMessageDiff(diff: string): Promise<{ git_diff: string }> {
  mockUseSessionGitStatus.mockReturnValue({ files: { "src/a.ts": { diff } } });
  mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "feat: x" });
  const { result } = renderHook(() => useUtilityAgentGenerator({ sessionId: "session-1" }));
  await act(async () => {
    await result.current.generateCommitMessage(() => undefined);
  });
  return mockExecuteUtilityPrompt.mock.calls[0][0] as { git_diff: string };
}

describe("useUtilityAgentGenerator enhancePrompt", () => {
  it("keeps hidden session composers off rich git-status subscriptions", () => {
    renderHook(() => useUtilityAgentGenerator({ sessionId: "session-1", detailActive: false }));

    expect(mockUseSessionGitStatus).toHaveBeenCalledWith("session-1", { detailActive: false });
  });

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

describe("useUtilityAgentGenerator request context", () => {
  it("does not send the diff when enhancing a prompt", async () => {
    mockUseSessionGitStatus.mockReturnValue({
      files: { "src/a.ts": { diff: "@@ -1 +1 @@\n-old\n+new" } },
    });
    mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "improved" });
    const { result } = renderHook(() => useUtilityAgentGenerator({ sessionId: "session-1" }));

    await act(async () => {
      await result.current.enhancePrompt("original", () => true);
    });

    expect(mockExecuteUtilityPrompt).toHaveBeenCalledWith(
      expect.objectContaining({ git_diff: undefined, changed_files: "src/a.ts" }),
    );
  });

  it("does not read changed-file diffs when enhancing a prompt", async () => {
    const file = Object.defineProperty({}, "diff", {
      get: () => {
        throw new Error("Enhance prompt must not assemble file diffs");
      },
    });
    mockUseSessionGitStatus.mockReturnValue({ files: { "src/a.ts": file } });
    mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "improved" });
    const { result } = renderHook(() => useUtilityAgentGenerator({ sessionId: "session-1" }));

    await act(async () => {
      await result.current.enhancePrompt("original", () => true);
    });

    expect(mockExecuteUtilityPrompt).toHaveBeenCalledWith(
      expect.objectContaining({ git_diff: undefined, changed_files: "src/a.ts" }),
    );
    expect(mockToast).not.toHaveBeenCalled();
  });

  it("still sends the diff for generators that need it", async () => {
    mockUseSessionGitStatus.mockReturnValue({
      files: { "src/a.ts": { diff: "@@ -1 +1 @@\n-old\n+new" } },
    });
    mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "feat: x" });
    const { result } = renderHook(() => useUtilityAgentGenerator({ sessionId: "session-1" }));

    await act(async () => {
      await result.current.generateCommitMessage(() => undefined);
    });

    expect(mockExecuteUtilityPrompt).toHaveBeenCalledWith(
      expect.objectContaining({ git_diff: "@@ -1 +1 @@\n-old\n+new" }),
    );
  });

  it("cuts an oversized diff on a UTF-8 boundary and names the dropped bytes", async () => {
    // One ASCII byte, then two-byte characters: the cut point (the 100,000-byte
    // cap minus the 64-byte marker reserve) lands on the second byte of a
    // character and must back off one byte.
    const request = await sendCommitMessageDiff(`a${"é".repeat(50_000)}`);

    const [kept, marker] = request.git_diff.split("\n[... truncated ");
    expect(marker).toBe("66 bytes ...]");
    expect(utf8Length(kept)).toBe(99_935);
    expect(kept).toBe(`a${"é".repeat(49_967)}`);
  });

  it("keeps a cut diff and its marker under the backend cap", async () => {
    const diff = "x".repeat(3_000_000);

    const request = await sendCommitMessageDiff(diff);

    // The backend cuts anything above MaxGitDiffBytes (100,000) again and would
    // replace this marker with one that counts only the marker itself.
    expect(utf8Length(request.git_diff)).toBeLessThanOrEqual(100_000);
    expect(request.git_diff.endsWith("\n[... truncated 2900064 bytes ...]")).toBe(true);
  });

  it("sends a diff at the cap unchanged", async () => {
    const diff = "x".repeat(100_000);

    const request = await sendCommitMessageDiff(diff);

    expect(request.git_diff).toBe(diff);
  });

  it("limits the changed-file list it sends", async () => {
    const files = Object.fromEntries(
      Array.from({ length: 205 }, (_, index) => [`src/file-${index}.ts`, { diff: "" }] as const),
    );
    mockUseSessionGitStatus.mockReturnValue({ files });
    mockExecuteUtilityPrompt.mockResolvedValue({ success: true, response: "improved" });
    const { result } = renderHook(() => useUtilityAgentGenerator({ sessionId: "session-1" }));

    await act(async () => {
      await result.current.enhancePrompt("original", () => true);
    });

    const request = mockExecuteUtilityPrompt.mock.calls[0][0] as { changed_files: string };
    const lines = request.changed_files.split("\n");
    expect(lines).toHaveLength(201);
    expect(lines[200]).toBe("... and 5 more files");
  });
});
