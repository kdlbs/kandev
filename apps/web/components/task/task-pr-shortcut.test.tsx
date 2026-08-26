import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TaskPR } from "@/lib/types/github";

const { mockOpenExternalLink, mockToast } = vi.hoisted(() => ({
  mockOpenExternalLink: vi.fn(),
  mockToast: vi.fn(),
}));
let mockPRs: TaskPR[] = [];

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: { userSettings: { keyboardShortcuts: Record<string, unknown> } }) => unknown,
  ) => selector({ userSettings: { keyboardShortcuts: {} } }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/hooks/domains/github/use-task-pr", () => ({
  useTaskPR: () => ({ prs: mockPRs }),
}));

vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useTaskMRs: () => [],
}));

vi.mock("@/lib/desktop/external-links", () => ({
  openExternalLink: mockOpenExternalLink,
}));

import { TaskPRShortcut } from "./task-pr-shortcut";

function makePR(id: string, number: number): TaskPR {
  return {
    id,
    workspace_id: "workspace-1",
    task_id: "task-1",
    owner: "acme",
    repo: "kandev",
    pr_number: number,
    pr_url: `https://github.com/acme/kandev/pull/${number}`,
    pr_title: `PR ${number}`,
    head_branch: `feature/${number}`,
    base_branch: "main",
    author_login: "alice",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "2026-07-31T00:00:00Z",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "2026-07-31T00:00:00Z",
  };
}

function dispatchKey(type: "keydown" | "keyup", key: string, init: KeyboardEventInit = {}) {
  act(() => {
    window.dispatchEvent(
      new KeyboardEvent(type, { key, bubbles: true, cancelable: true, ...init }),
    );
  });
}

describe("TaskPRShortcut", () => {
  beforeEach(() => {
    mockPRs = [makePR("pr-1", 1), makePR("pr-2", 2)];
    mockOpenExternalLink.mockReset().mockResolvedValue(undefined);
    mockToast.mockReset();
  });

  afterEach(() => cleanup());

  it("opens the cycled review when the primary modifier is released", async () => {
    render(<TaskPRShortcut taskId="task-1" />);

    dispatchKey("keydown", "g", { ctrlKey: true, shiftKey: true });
    dispatchKey("keydown", "g", { ctrlKey: true });

    await waitFor(() =>
      expect(screen.getByTestId("task-pr-picker-row-pr-2").getAttribute("data-selected")).toBe(
        "true",
      ),
    );

    dispatchKey("keyup", "Control");

    await waitFor(() => expect(mockOpenExternalLink).toHaveBeenCalledWith(mockPRs[1].pr_url));
  });
});
