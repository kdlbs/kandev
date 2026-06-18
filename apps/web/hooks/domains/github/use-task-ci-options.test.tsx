import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import type { TaskCIAutomationOptions } from "@/lib/types/github";

const apiMocks = vi.hoisted(() => ({
  getOptionsMock: vi.fn(),
  updateOptionsMock: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => ({
  getTaskCIAutomationOptions: apiMocks.getOptionsMock,
  updateTaskCIAutomationOptions: apiMocks.updateOptionsMock,
}));

import { useTaskCIAutomationOptions } from "./use-task-ci-options";

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function makeOptions(overrides: Partial<TaskCIAutomationOptions> = {}): TaskCIAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "Default CI prompt",
    using_default_prompt: true,
    updated_at: "2026-06-18T10:00:00Z",
    pr_states: [],
    ...overrides,
  };
}

beforeEach(() => {
  apiMocks.getOptionsMock.mockReset();
  apiMocks.updateOptionsMock.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("useTaskCIAutomationOptions", () => {
  it("loads options for the task and stores the response", async () => {
    apiMocks.getOptionsMock.mockResolvedValue(makeOptions({ auto_fix_enabled: true }));

    const { result } = renderHook(() => useTaskCIAutomationOptions("task-1"), { wrapper });

    await waitFor(() => expect(result.current.options?.auto_fix_enabled).toBe(true));
    expect(apiMocks.getOptionsMock).toHaveBeenCalledWith("task-1", { cache: "no-store" });
    expect(result.current.loading).toBe(false);
  });

  it("patches options and supports resetting the task prompt override", async () => {
    apiMocks.getOptionsMock.mockResolvedValue(
      makeOptions({ auto_fix_prompt_override: "Custom prompt" }),
    );
    apiMocks.updateOptionsMock.mockResolvedValue(makeOptions({ auto_fix_prompt_override: null }));

    const { result } = renderHook(() => useTaskCIAutomationOptions("task-1"), { wrapper });
    await waitFor(() => expect(result.current.options).not.toBeNull());

    await act(async () => {
      await result.current.resetPrompt();
    });

    expect(apiMocks.updateOptionsMock).toHaveBeenCalledWith(
      "task-1",
      { auto_fix_prompt_override: null },
      { cache: "no-store" },
    );
    expect(result.current.options?.auto_fix_prompt_override).toBeNull();
    expect(result.current.saving).toBe(false);
  });

  it("does not auto-retry after a load error", async () => {
    apiMocks.getOptionsMock.mockRejectedValue(new Error("backend unavailable"));

    const { result } = renderHook(() => useTaskCIAutomationOptions("task-1"), { wrapper });

    await waitFor(() => expect(result.current.error).toBe("backend unavailable"));
    expect(apiMocks.getOptionsMock).toHaveBeenCalledTimes(1);
    await act(async () => {
      await Promise.resolve();
    });
    expect(apiMocks.getOptionsMock).toHaveBeenCalledTimes(1);
  });

  it("ignores stale refresh responses", async () => {
    let resolveFirst: (value: TaskCIAutomationOptions) => void = () => {};
    let resolveSecond: (value: TaskCIAutomationOptions) => void = () => {};
    apiMocks.getOptionsMock
      .mockReturnValueOnce(new Promise((resolve) => (resolveFirst = resolve)))
      .mockReturnValueOnce(new Promise((resolve) => (resolveSecond = resolve)));

    const { result } = renderHook(() => useTaskCIAutomationOptions("task-1"), { wrapper });

    await waitFor(() => expect(apiMocks.getOptionsMock).toHaveBeenCalledTimes(1));
    let secondRefresh: Promise<TaskCIAutomationOptions | null>;
    await act(async () => {
      secondRefresh = result.current.refresh();
    });
    resolveSecond(makeOptions({ auto_fix_enabled: true }));
    await act(async () => {
      await secondRefresh!;
    });
    resolveFirst(makeOptions({ auto_fix_enabled: false }));
    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.options?.auto_fix_enabled).toBe(true);
  });

  it("ignores stale update responses", async () => {
    apiMocks.getOptionsMock.mockResolvedValue(makeOptions());
    let resolveFirst: (value: TaskCIAutomationOptions) => void = () => {};
    let resolveSecond: (value: TaskCIAutomationOptions) => void = () => {};
    apiMocks.updateOptionsMock
      .mockReturnValueOnce(new Promise((resolve) => (resolveFirst = resolve)))
      .mockReturnValueOnce(new Promise((resolve) => (resolveSecond = resolve)));

    const { result } = renderHook(() => useTaskCIAutomationOptions("task-1"), { wrapper });
    await waitFor(() => expect(result.current.options).not.toBeNull());

    let firstUpdate: Promise<TaskCIAutomationOptions | null>;
    let secondUpdate: Promise<TaskCIAutomationOptions | null>;
    await act(async () => {
      firstUpdate = result.current.update({ auto_fix_enabled: true });
      secondUpdate = result.current.update({ auto_merge_enabled: true });
    });
    resolveSecond(makeOptions({ auto_merge_enabled: true }));
    await act(async () => {
      await secondUpdate!;
    });
    resolveFirst(makeOptions({ auto_fix_enabled: true }));
    await act(async () => {
      await firstUpdate!;
    });

    expect(result.current.options?.auto_merge_enabled).toBe(true);
    expect(result.current.options?.auto_fix_enabled).toBe(false);
  });
});
