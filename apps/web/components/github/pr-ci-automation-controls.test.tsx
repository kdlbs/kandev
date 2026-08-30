import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";

const hookMocks = vi.hoisted(() => ({
  error: null as string | null,
  options: null as TaskCIAutomationOptions | null,
  refresh: vi.fn(),
  retryMerge: vi.fn(),
}));

vi.mock("@/hooks/domains/github/use-task-ci-options", () => ({
  useTaskCIAutomationOptions: () => ({
    options: hookMocks.options,
    loading: false,
    saving: false,
    error: hookMocks.error,
    refresh: hookMocks.refresh,
    retryMerge: hookMocks.retryMerge,
    update: vi.fn(),
    resetPrompt: vi.fn(),
  }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/components/github/pr-ci-automation-prompt-dialog", () => ({
  CIAutomationPromptDialog: () => null,
}));

vi.mock("@/components/github/pr-ci-automation-rows", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/github/pr-ci-automation-rows")>();
  return {
    ...actual,
    CIAutomationHeader: () => null,
    CIAutomationOptionRows: () => null,
  };
});

import { PRCIAutomationControls } from "./pr-ci-automation-controls";

const pr = {
  task_id: "task-1",
  repository_id: "repo-1",
  pr_number: 42,
} as TaskPR;

function makeOptions(errorKind: string): TaskCIAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: true,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "Default prompt",
    using_default_prompt: true,
    updated_at: "2026-08-28T10:00:00Z",
    pr_options: [],
    pr_states: [
      {
        task_id: "task-1",
        repository_id: "repo-1",
        pr_number: 42,
        last_error: "merge PR: provider unavailable",
        last_error_kind: errorKind,
      } as TaskCIAutomationOptions["pr_states"][number],
    ],
  };
}

beforeEach(() => {
  hookMocks.error = null;
  hookMocks.options = makeOptions("auto_merge");
  hookMocks.refresh.mockReset().mockResolvedValue(null);
  hookMocks.retryMerge.mockReset().mockResolvedValue({ accepted: true });
});

afterEach(cleanup);

describe("PRCIAutomationControls retry actions", () => {
  it("rearms only a typed auto-merge error for the exact linked PR", () => {
    render(<PRCIAutomationControls pr={pr} />);

    const action = screen.getByRole("button", { name: "Retry" });
    expect(action.className).toContain("h-11");
    fireEvent.click(action);

    expect(hookMocks.retryMerge).toHaveBeenCalledWith("repo-1", 42);
    expect(hookMocks.refresh).not.toHaveBeenCalled();
  });

  it("refreshes state for a stored error that is not an auto-merge failure", () => {
    hookMocks.options = makeOptions("auto_fix");
    render(<PRCIAutomationControls pr={pr} />);

    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));

    expect(hookMocks.refresh).toHaveBeenCalledTimes(1);
    expect(hookMocks.retryMerge).not.toHaveBeenCalled();
  });

  it("refreshes state after a load error and never rearms merge", () => {
    hookMocks.options = null;
    hookMocks.error = "backend unavailable";
    render(<PRCIAutomationControls pr={pr} />);

    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));

    expect(hookMocks.refresh).toHaveBeenCalledTimes(1);
    expect(hookMocks.retryMerge).not.toHaveBeenCalled();
  });
});
