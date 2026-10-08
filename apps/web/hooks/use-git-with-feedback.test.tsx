import { act, cleanup, renderHook, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { useGitWithFeedback } from "./use-git-with-feedback";

vi.mock("@/lib/api/domains/frontend-error-log-api", () => ({
  scheduleFrontendErrorReport: vi.fn(),
}));

afterEach(cleanup);

// @covers AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.1, .2, .3
describe("Git operation acknowledgement and existing feedback", () => {
  it.each([
    [{ success: true, output: "committed" }, true, "Commit successful", "committed"],
    [{ success: false, output: "", error: "hook denied" }, false, "Commit failed", "hook denied"],
  ])(
    "returns acknowledgement %s after reporting outcome",
    async (outcome, expected, title, description) => {
      const { result } = renderHook(useGitWithFeedback, { wrapper: ToastProvider });
      let acknowledgement: unknown;
      await act(async () => {
        acknowledgement = await result.current(async () => outcome, "Commit");
      });
      expect(acknowledgement).toBe(expected);
      expect(screen.getByText(title)).toBeTruthy();
      expect(screen.getByText(description)).toBeTruthy();
    },
  );

  it.each([new Error("request denied"), "unknown rejection"])(
    "returns false after reporting a rejection",
    async (error) => {
      const { result } = renderHook(useGitWithFeedback, { wrapper: ToastProvider });
      let acknowledgement: unknown;
      await act(async () => {
        acknowledgement = await result.current(async () => {
          throw error;
        }, "Commit");
      });
      expect(acknowledgement).toBe(false);
      expect(screen.getByText("Commit failed")).toBeTruthy();
      expect(
        screen.getByText(error instanceof Error ? error.message : "An unexpected error occurred"),
      ).toBeTruthy();
    },
  );

  it("keeps loading feedback until acknowledgement and retains output truncation", async () => {
    let resolve!: (value: { success: boolean; output: string }) => void;
    const response = new Promise<{ success: boolean; output: string }>((res) => {
      resolve = res;
    });
    const { result } = renderHook(useGitWithFeedback, { wrapper: ToastProvider });
    let run!: Promise<unknown>;
    act(() => {
      run = result.current(() => response, "Push");
    });
    expect(screen.getByText("Push...")).toBeTruthy();
    await act(async () => {
      resolve({ success: true, output: "x".repeat(220) });
      await run;
    });
    expect(screen.getByText("Push successful")).toBeTruthy();
    expect(screen.getByText("x".repeat(200))).toBeTruthy();
  });
});
