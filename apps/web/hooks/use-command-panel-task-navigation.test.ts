import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Task } from "@/lib/types/http";

const mockPush = vi.hoisted(() => vi.fn());
const mockCancelSidebarTaskReveal = vi.hoisted(() => vi.fn());
const mockRevealSidebarTask = vi.hoisted(() => vi.fn());

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mockPush }),
}));
vi.mock("@/lib/sidebar/task-navigation", () => ({
  cancelSidebarTaskReveal: mockCancelSidebarTaskReveal,
  revealSidebarTask: mockRevealSidebarTask,
}));

import { useCommandPanelTaskNavigation } from "./use-command-panel-task-navigation";

const task = (id: string) => ({ id }) as Task;

describe("useCommandPanelTaskNavigation", () => {
  beforeEach(() => {
    mockPush.mockReset();
    mockCancelSidebarTaskReveal.mockReset();
    mockRevealSidebarTask.mockReset();
    mockRevealSidebarTask.mockReturnValue(new Promise<boolean>(() => undefined));
  });

  it("cancels an active reveal before queuing a newer task navigation", () => {
    const { result } = renderHook(() => useCommandPanelTaskNavigation("/t/task-a", "task-a"));

    act(() => result.current(task("task-a")));
    expect(mockRevealSidebarTask).toHaveBeenCalledWith("task-a");

    act(() => result.current(task("task-b")));

    expect(mockCancelSidebarTaskReveal).toHaveBeenCalledTimes(2);
    expect(mockPush).toHaveBeenNthCalledWith(2, "/t/task-b");
  });
});

it("opens the session that contains the conversation match", () => {
  const { result } = renderHook(() => useCommandPanelTaskNavigation("/", null));
  act(() =>
    result.current({
      ...task("found"),
      conversation_match: {
        session_id: "matching-session",
        message_id: "m1",
        author_type: "agent",
        excerpt: "needle",
        highlight_ranges: [[0, 6]],
      },
    }),
  );
  expect(mockPush).toHaveBeenLastCalledWith("/t/found?sessionId=matching-session");
});
