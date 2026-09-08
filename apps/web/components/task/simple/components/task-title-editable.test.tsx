import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import {
  __resetOfficeTaskContentSyncForTests,
  isFieldGuarded,
} from "@/lib/state/office-task-content-sync";
import { TaskTitleEditable } from "./task-title-editable";
import type { Task } from "@/app/office/tasks/[id]/types";

vi.mock("@/lib/api/domains/kanban-api", async () => {
  const actual = await vi.importActual<typeof import("@/lib/api/domains/kanban-api")>(
    "@/lib/api/domains/kanban-api",
  );
  return {
    ...actual,
    updateTask: vi.fn(),
  };
});

import { updateTask } from "@/lib/api/domains/kanban-api";

const mockedUpdateTask = vi.mocked(updateTask);

const ORIGINAL_TITLE = "Original title";
const HEADING_TESTID = "office-task-title";
const INPUT_TESTID = "office-task-title-input";
const ERROR_TESTID = "office-task-title-error";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  __resetOfficeTaskContentSyncForTests();
});

const task: Task = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: ORIGINAL_TITLE,
  status: "todo",
  priority: "medium",
  labels: [],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: [],
  decisions: [],
  createdBy: "user",
  createdAt: "2026-05-01T00:00:00Z",
  updatedAt: "2026-05-01T00:00:00Z",
};

function Wrapper({ children }: { children: ReactNode }) {
  const ctx = { task, applyPatch: vi.fn(), restore: vi.fn() };
  return (
    <StateProvider>
      <TaskOptimisticContextProvider value={ctx}>{children}</TaskOptimisticContextProvider>
    </StateProvider>
  );
}

function renderEditable() {
  return render(
    <Wrapper>
      <TaskTitleEditable taskId="t-1" title={ORIGINAL_TITLE} />
    </Wrapper>,
  );
}

function openEditor() {
  fireEvent.doubleClick(screen.getByTestId(HEADING_TESTID));
  return screen.getByTestId(INPUT_TESTID);
}

describe("TaskTitleEditable read/activate", () => {
  it("renders the title as a heading and enters edit mode on double-click, focused with its content selected (AC-1)", () => {
    renderEditable();
    const heading = screen.getByTestId(HEADING_TESTID);
    expect(heading.textContent).toBe(ORIGINAL_TITLE);
    fireEvent.doubleClick(heading);
    const input = screen.getByTestId(INPUT_TESTID) as HTMLInputElement;
    expect(input.value).toBe(ORIGINAL_TITLE);
    fireEvent.focus(input);
    expect(document.activeElement).toBe(input);
    expect(input.selectionStart).toBe(0);
    expect(input.selectionEnd).toBe(ORIGINAL_TITLE.length);
  });

  it("enters edit mode on Enter when the heading is focused, seeded with the current title (AC-2)", () => {
    renderEditable();
    fireEvent.keyDown(screen.getByTestId(HEADING_TESTID), { key: "Enter" });
    const input = screen.getByTestId(INPUT_TESTID) as HTMLInputElement;
    expect(input.value).toBe(ORIGINAL_TITLE);
  });
});

describe("TaskTitleEditable commit", () => {
  it("commits a trimmed, changed title on Enter", async () => {
    mockedUpdateTask.mockResolvedValue({
      title: "New title",
      updated_at: "2026-05-02T00:00:00Z",
    } as never);
    renderEditable();
    const input = openEditor();
    fireEvent.change(input, { target: { value: "  New title  " } });
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });
    expect(mockedUpdateTask).toHaveBeenCalledWith("t-1", { title: "New title" });
    expect(screen.queryByTestId(INPUT_TESTID)).toBeNull();
  });

  it("does not commit and closes without a request when Enter matches the baseline", async () => {
    renderEditable();
    const input = openEditor();
    fireEvent.change(input, { target: { value: ORIGINAL_TITLE } });
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });
    expect(mockedUpdateTask).not.toHaveBeenCalled();
    expect(screen.queryByTestId(INPUT_TESTID)).toBeNull();
  });

  it("shows an error and stays open when the draft is emptied, clearing once fixed (AC-37)", () => {
    renderEditable();
    const input = openEditor() as HTMLInputElement;
    fireEvent.change(input, { target: { value: "   " } });
    expect(input.getAttribute("aria-invalid")).toBe("true");
    fireEvent.keyDown(input, { key: "Enter" });
    expect(mockedUpdateTask).not.toHaveBeenCalled();
    expect(screen.getByTestId(ERROR_TESTID)).toBeTruthy();
    expect(screen.getByTestId(INPUT_TESTID)).toBeTruthy();

    fireEvent.change(input, { target: { value: "Fixed title" } });
    expect(screen.queryByTestId(ERROR_TESTID)).toBeNull();
    expect(input.getAttribute("aria-invalid")).toBe("false");
  });

  it("ignores Enter during IME composition", () => {
    renderEditable();
    const input = openEditor();
    fireEvent.change(input, { target: { value: "New title" } });
    fireEvent.keyDown(input, { key: "Enter", keyCode: 229 });
    expect(mockedUpdateTask).not.toHaveBeenCalled();
    expect(screen.getByTestId(INPUT_TESTID)).toBeTruthy();
  });
});

describe("TaskTitleEditable frozen baseline (AC-51)", () => {
  it("keeps the frozen baseline when a newer canonical title arrives while the editor is open", () => {
    const { rerender } = render(
      <Wrapper>
        <TaskTitleEditable taskId="t-1" title={ORIGINAL_TITLE} />
      </Wrapper>,
    );
    const input = openEditor();

    // A canonical update lands (e.g. via the deferred-apply store patch)
    // while the editor is still open, changing the prop the parent passes.
    rerender(
      <Wrapper>
        <TaskTitleEditable taskId="t-1" title="Canonical update" />
      </Wrapper>,
    );

    // Typing the ORIGINAL (frozen) baseline back in and pressing Enter must
    // be a no-op close, not a commit compared against the new canonical
    // value — proving the baseline never re-derived from the live prop.
    fireEvent.change(input, { target: { value: ORIGINAL_TITLE } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(mockedUpdateTask).not.toHaveBeenCalled();
    expect(screen.queryByTestId(INPUT_TESTID)).toBeNull();
  });
});

describe("TaskTitleEditable cancel", () => {
  it("closes the editor without committing on Escape", () => {
    renderEditable();
    const input = openEditor();
    fireEvent.change(input, { target: { value: "Changed but not saved" } });
    fireEvent.keyDown(input, { key: "Escape" });
    expect(mockedUpdateTask).not.toHaveBeenCalled();
    expect(screen.queryByTestId(INPUT_TESTID)).toBeNull();
  });

  it("closes the editor without committing on blur", () => {
    renderEditable();
    const input = openEditor();
    fireEvent.change(input, { target: { value: "Changed but not saved" } });
    fireEvent.blur(input);
    expect(mockedUpdateTask).not.toHaveBeenCalled();
    expect(screen.queryByTestId(INPUT_TESTID)).toBeNull();
  });
});

describe("TaskTitleEditable clamp (AC-6)", () => {
  it("retains only the first 60 code points when typed or pasted text would exceed the cap", () => {
    renderEditable();
    const input = openEditor() as HTMLInputElement;
    // fireEvent.change dispatches the same DOM event a paste produces, so
    // this covers both entry paths named in AC-6.
    fireEvent.change(input, { target: { value: "x".repeat(90) } });
    expect(input.value).toHaveLength(60);
    expect(input.value).toBe("x".repeat(60));
  });

  it("counts by code point, not UTF-16 code unit, so a surrogate-pair character is not split", () => {
    renderEditable();
    const input = openEditor() as HTMLInputElement;
    // Each emoji below is one code point (two UTF-16 code units): 65 emoji is
    // 65 code points, so the clamp must keep the first 60 emoji intact
    // rather than cutting mid-surrogate-pair at 60 code units.
    const emoji = "😀".repeat(65);
    fireEvent.change(input, { target: { value: emoji } });
    expect(Array.from(input.value)).toHaveLength(60);
    expect(input.value).toBe("😀".repeat(60));
  });

  it("does not clamp text at or under the 60 code point cap", () => {
    renderEditable();
    const input = openEditor() as HTMLInputElement;
    const atCap = "x".repeat(60);
    fireEvent.change(input, { target: { value: atCap } });
    expect(input.value).toBe(atCap);
  });
});

describe("TaskTitleEditable unmount cleanup (AC-68, COR-002)", () => {
  it("releases the field's guard when the component unmounts while the editor is still open", () => {
    const { unmount } = renderEditable();
    openEditor();
    expect(isFieldGuarded("t-1", "title")).toBe(true);

    unmount();

    expect(isFieldGuarded("t-1", "title")).toBe(false);
  });

  it("is a no-op on unmount when the editor is already closed", () => {
    const { unmount } = renderEditable();
    expect(isFieldGuarded("t-1", "title")).toBe(false);

    unmount();

    expect(isFieldGuarded("t-1", "title")).toBe(false);
  });
});

describe("TaskTitleEditable taskId change while editing (P1 review fix, COR-002)", () => {
  it("closes the editor and releases the old task's guard when taskId changes while open", () => {
    const { rerender } = render(
      <Wrapper>
        <TaskTitleEditable taskId="t-1" title={ORIGINAL_TITLE} />
      </Wrapper>,
    );
    openEditor();
    expect(isFieldGuarded("t-1", "title")).toBe(true);

    // TaskBody swaps simple/advanced panes on a URL search param, which can
    // reuse this component instance for a different taskId without
    // unmounting it (no key={taskId} upstream).
    rerender(
      <Wrapper>
        <TaskTitleEditable taskId="t-2" title="Other task title" />
      </Wrapper>,
    );

    expect(isFieldGuarded("t-1", "title")).toBe(false);
    expect(isFieldGuarded("t-2", "title")).toBe(false);
    expect(screen.queryByTestId(INPUT_TESTID)).toBeNull();
    expect(screen.getByTestId(HEADING_TESTID).textContent).toBe("Other task title");
  });
});
