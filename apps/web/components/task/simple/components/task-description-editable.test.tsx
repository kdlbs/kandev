import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import {
  __resetOfficeTaskContentSyncForTests,
  isFieldGuarded,
} from "@/lib/state/office-task-content-sync";
import { TaskDescriptionEditable } from "./task-description-editable";
import type { Task, TaskSession } from "@/app/office/tasks/[id]/types";

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

const ORIGINAL_DESCRIPTION = "Original description";
const CHANGED_DESCRIPTION = "Changed description";
const READ_TESTID = "office-task-description";
const EMPTY_TESTID = "office-task-description-empty";
const TEXTAREA_TESTID = "office-task-description-textarea";
const SAVE_TESTID = "office-task-description-save";
const CANCEL_TESTID = "office-task-description-cancel";
const DISCARD_CONFIRM_TESTID = "office-task-description-discard-confirm";
const RUNNING_NOTE_TESTID = "office-task-description-running-note";
const UPDATED_AT = "2026-05-02T00:00:00Z";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  __resetOfficeTaskContentSyncForTests();
});

const task: Task = {
  id: "t-1",
  workspaceId: "ws-1",
  identifier: "TASK-1",
  title: "Task",
  description: ORIGINAL_DESCRIPTION,
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

function renderEditable(description: string | undefined, sessions: TaskSession[] = []) {
  return render(
    <Wrapper>
      <TaskDescriptionEditable taskId="t-1" description={description} sessions={sessions} />
    </Wrapper>,
  );
}

function openEditor(description = ORIGINAL_DESCRIPTION) {
  renderEditable(description);
  fireEvent.click(screen.getByTestId(READ_TESTID));
  return screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement;
}

function changeDraft(textarea: HTMLTextAreaElement, value: string) {
  fireEvent.change(textarea, { target: { value } });
}

describe("TaskDescriptionEditable read view", () => {
  it("renders the empty-state placeholder when there is no description", () => {
    renderEditable(undefined);
    expect(screen.getByTestId(EMPTY_TESTID)).toBeTruthy();
    expect(screen.queryByTestId(READ_TESTID)).toBeNull();
  });

  it("opens the editor with an empty draft when the empty-state placeholder is clicked (AC-13)", () => {
    renderEditable(undefined);
    fireEvent.click(screen.getByTestId(EMPTY_TESTID));
    const textarea = screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement;
    expect(textarea.value).toBe("");
  });

  it("renders the description text and opens the editor on click, focused (AC-12)", () => {
    const textarea = openEditor();
    expect(textarea.value).toBe(ORIGINAL_DESCRIPTION);
    expect(document.activeElement).toBe(textarea);
  });

  it("opens the editor when the description element has keyboard focus and Enter is pressed (AC-33)", () => {
    renderEditable(ORIGINAL_DESCRIPTION);
    fireEvent.keyDown(screen.getByTestId(READ_TESTID), { key: "Enter" });
    const textarea = screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement;
    expect(textarea.value).toBe(ORIGINAL_DESCRIPTION);
  });

  it("renders Markdown syntax as literal plain text rather than formatted output (AC-46)", () => {
    const markdown = "# Heading\n\n**bold** and _italic_ and [a link](https://example.com)";
    renderEditable(markdown);
    const view = screen.getByTestId(READ_TESTID);
    expect(view.textContent).toBe(markdown);
    expect(view.querySelector("strong")).toBeNull();
    expect(view.querySelector("em")).toBeNull();
    expect(view.querySelector("a")).toBeNull();
    expect(view.querySelector("h1")).toBeNull();
  });

  it("does not open the editor when clicking to select text", () => {
    renderEditable(ORIGINAL_DESCRIPTION);
    const view = screen.getByTestId(READ_TESTID);
    const getSelectionSpy = vi.spyOn(window, "getSelection").mockReturnValue({
      toString: () => "some selected text",
    } as Selection);
    fireEvent.click(view);
    expect(screen.queryByTestId(TEXTAREA_TESTID)).toBeNull();
    getSelectionSpy.mockRestore();
  });
});

describe("TaskDescriptionEditable save", () => {
  it("Save is disabled until the draft differs from the baseline", () => {
    const textarea = openEditor();
    const save = screen.getByTestId(SAVE_TESTID) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    changeDraft(textarea, CHANGED_DESCRIPTION);
    expect(save.disabled).toBe(false);
  });

  it("commits a trimmed draft on Save and closes the editor", async () => {
    mockedUpdateTask.mockResolvedValue({
      description: CHANGED_DESCRIPTION,
      updated_at: UPDATED_AT,
    } as never);
    const textarea = openEditor();
    changeDraft(textarea, `  ${CHANGED_DESCRIPTION}  `);
    await act(async () => {
      fireEvent.click(screen.getByTestId(SAVE_TESTID));
    });
    expect(mockedUpdateTask).toHaveBeenCalledWith("t-1", { description: CHANGED_DESCRIPTION });
    expect(screen.queryByTestId(TEXTAREA_TESTID)).toBeNull();
  });

  it("sends and persists a long description unmodified apart from whitespace trimming (AC-45)", async () => {
    const longDescription = "Lorem ipsum dolor sit amet. ".repeat(400).trim();
    mockedUpdateTask.mockResolvedValue({
      description: longDescription,
      updated_at: UPDATED_AT,
    } as never);
    const textarea = openEditor();
    changeDraft(textarea, `  ${longDescription}  `);
    await act(async () => {
      fireEvent.click(screen.getByTestId(SAVE_TESTID));
    });
    expect(mockedUpdateTask).toHaveBeenCalledWith("t-1", { description: longDescription });
    expect(screen.queryByTestId(TEXTAREA_TESTID)).toBeNull();
  });
});

describe("TaskDescriptionEditable clearing to empty (AC-24)", () => {
  it("clears the stored description and renders the empty-state placeholder after saving an empty draft", async () => {
    mockedUpdateTask.mockResolvedValue({
      description: "",
      updated_at: UPDATED_AT,
    } as never);
    const { rerender } = render(
      <Wrapper>
        <TaskDescriptionEditable taskId="t-1" description={ORIGINAL_DESCRIPTION} sessions={[]} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId(READ_TESTID));
    const textarea = screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement;
    changeDraft(textarea, "   ");
    await act(async () => {
      fireEvent.click(screen.getByTestId(SAVE_TESTID));
    });
    expect(mockedUpdateTask).toHaveBeenCalledWith("t-1", { description: "" });
    expect(screen.queryByTestId(TEXTAREA_TESTID)).toBeNull();

    // The parent re-renders with the newly cleared description once the
    // store/canonical value converges (simulated here via rerender).
    rerender(
      <Wrapper>
        <TaskDescriptionEditable taskId="t-1" description="" sessions={[]} />
      </Wrapper>,
    );
    expect(screen.getByTestId(EMPTY_TESTID)).toBeTruthy();
  });
});

describe("TaskDescriptionEditable whitespace-only draft on an empty description (AC-43)", () => {
  it("treats a whitespace-only draft as equal to an empty baseline, keeping Save disabled and sending no request", () => {
    renderEditable("");
    fireEvent.click(screen.getByTestId(EMPTY_TESTID));
    const textarea = screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement;
    changeDraft(textarea, "   ");
    expect((screen.getByTestId(SAVE_TESTID) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId(SAVE_TESTID));
    expect(mockedUpdateTask).not.toHaveBeenCalled();
  });
});

describe("TaskDescriptionEditable in-flight save state (AC-41)", () => {
  it("keeps the textarea editable, disables Save and Cancel, and takes no action on Escape while saving", async () => {
    let resolveUpdate: (value: never) => void = () => {};
    mockedUpdateTask.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveUpdate = resolve;
        }),
    );
    const textarea = openEditor();
    changeDraft(textarea, CHANGED_DESCRIPTION);

    await act(async () => {
      fireEvent.click(screen.getByTestId(SAVE_TESTID));
      // Yield one microtask so the `isSaving` state commits without waiting
      // for the still-pending mocked update to resolve.
      await Promise.resolve();
    });

    expect(textarea.disabled).toBe(false);
    expect((screen.getByTestId(SAVE_TESTID) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId(CANCEL_TESTID) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.keyDown(textarea, { key: "Escape" });
    expect(screen.getByTestId(TEXTAREA_TESTID)).toBeTruthy();

    await act(async () => {
      resolveUpdate({
        description: CHANGED_DESCRIPTION,
        updated_at: UPDATED_AT,
      } as never);
    });
  });
});

describe("TaskDescriptionEditable draft changed mid-save (AC-42)", () => {
  it("keeps the editor open with the newer draft, rebases the baseline to the saved value, and re-enables Save", async () => {
    let resolveUpdate: (value: never) => void = () => {};
    mockedUpdateTask.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveUpdate = resolve;
        }),
    );
    const textarea = openEditor();
    changeDraft(textarea, "First draft");

    await act(async () => {
      fireEvent.click(screen.getByTestId(SAVE_TESTID));
      await Promise.resolve();
    });

    // The user keeps typing while the save for "First draft" is in flight.
    changeDraft(textarea, "Second draft");

    await act(async () => {
      resolveUpdate({ description: "First draft", updated_at: UPDATED_AT } as never);
    });

    expect(screen.getByTestId(TEXTAREA_TESTID)).toBeTruthy();
    expect((screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement).value).toBe("Second draft");
    expect((screen.getByTestId(SAVE_TESTID) as HTMLButtonElement).disabled).toBe(false);
  });
});

describe("TaskDescriptionEditable blur (AC-21)", () => {
  it("keeps the editor open, keeps the draft, and sends no request when the textarea loses focus", () => {
    const textarea = openEditor();
    changeDraft(textarea, "Changed but not saved");
    fireEvent.blur(textarea);
    expect(screen.getByTestId(TEXTAREA_TESTID)).toBeTruthy();
    expect((screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement).value).toBe(
      "Changed but not saved",
    );
    expect(mockedUpdateTask).not.toHaveBeenCalled();
  });
});

describe("TaskDescriptionEditable escape and cancel", () => {
  it("closes the editor on Escape when the draft is clean", () => {
    const textarea = openEditor();
    fireEvent.keyDown(textarea, { key: "Escape" });
    expect(screen.queryByTestId(TEXTAREA_TESTID)).toBeNull();
  });

  it("does nothing on Escape when the draft is dirty", () => {
    const textarea = openEditor();
    changeDraft(textarea, "Changed");
    fireEvent.keyDown(textarea, { key: "Escape" });
    expect(screen.getByTestId(TEXTAREA_TESTID)).toBeTruthy();
  });

  it("closes immediately on Cancel when the draft is clean", () => {
    openEditor();
    fireEvent.click(screen.getByTestId(CANCEL_TESTID));
    expect(screen.queryByTestId(TEXTAREA_TESTID)).toBeNull();
  });
});

describe("TaskDescriptionEditable discard confirmation", () => {
  it("shows a discard-confirm dialog on Cancel when dirty, and discards on confirm", () => {
    const textarea = openEditor();
    changeDraft(textarea, "Changed");
    fireEvent.click(screen.getByTestId(CANCEL_TESTID));
    expect(screen.getByTestId(DISCARD_CONFIRM_TESTID)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(screen.queryByTestId(TEXTAREA_TESTID)).toBeNull();
  });

  it("keeps the draft when discard is dismissed via 'keep editing'", () => {
    const textarea = openEditor();
    changeDraft(textarea, "Changed");
    fireEvent.click(screen.getByTestId(CANCEL_TESTID));
    fireEvent.click(screen.getByRole("button", { name: "Keep editing" }));
    expect((screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement).value).toBe("Changed");
  });

  it("keeps the draft when discard is dismissed via Escape rather than the explicit discard choice (AC-36)", () => {
    const textarea = openEditor();
    changeDraft(textarea, "Changed");
    fireEvent.click(screen.getByTestId(CANCEL_TESTID));
    expect(screen.getByTestId(DISCARD_CONFIRM_TESTID)).toBeTruthy();
    fireEvent.keyDown(screen.getByTestId(DISCARD_CONFIRM_TESTID), { key: "Escape" });
    expect(screen.queryByTestId(DISCARD_CONFIRM_TESTID)).toBeNull();
    expect((screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement).value).toBe("Changed");
  });
});

describe("TaskDescriptionEditable frozen baseline (AC-40)", () => {
  it("keeps Save disabled and Escape closing when a refetch changes the stored description while open", () => {
    const { rerender } = render(
      <Wrapper>
        <TaskDescriptionEditable taskId="t-1" description={ORIGINAL_DESCRIPTION} sessions={[]} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId(READ_TESTID));
    const textarea = screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement;

    // A refetch changes the stored description while the editor is open.
    rerender(
      <Wrapper>
        <TaskDescriptionEditable taskId="t-1" description="Refetched description" sessions={[]} />
      </Wrapper>,
    );

    // The draft still equals the frozen (original) baseline, not the
    // refetched value, so Save stays disabled...
    const save = screen.getByTestId(SAVE_TESTID) as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    // ...and Escape closes immediately rather than being treated as dirty.
    fireEvent.keyDown(textarea, { key: "Escape" });
    expect(screen.queryByTestId(TEXTAREA_TESTID)).toBeNull();
  });
});

describe("TaskDescriptionEditable unmount cleanup (AC-68, COR-002)", () => {
  it("releases the field's guard when the component unmounts while the editor is still open", () => {
    const { unmount } = renderEditable(ORIGINAL_DESCRIPTION);
    fireEvent.click(screen.getByTestId(READ_TESTID));
    expect(isFieldGuarded("t-1", "description")).toBe(true);

    unmount();

    expect(isFieldGuarded("t-1", "description")).toBe(false);
  });

  it("is a no-op on unmount when the editor is already closed", () => {
    const { unmount } = renderEditable(ORIGINAL_DESCRIPTION);
    expect(isFieldGuarded("t-1", "description")).toBe(false);

    unmount();

    expect(isFieldGuarded("t-1", "description")).toBe(false);
  });
});

describe("TaskDescriptionEditable taskId change while editing (P1 review fix, COR-002)", () => {
  it("closes the editor and releases the old task's guard when taskId changes while open", () => {
    const { rerender } = render(
      <Wrapper>
        <TaskDescriptionEditable taskId="t-1" description={ORIGINAL_DESCRIPTION} sessions={[]} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId(READ_TESTID));
    const textarea = screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement;
    changeDraft(textarea, "Changed");
    expect(isFieldGuarded("t-1", "description")).toBe(true);

    // TaskBody swaps simple/advanced panes on a URL search param, which can
    // reuse this component instance for a different taskId without
    // unmounting it (no key={taskId} upstream). Description never closes on
    // blur (AC-21), so this is the only safety net for a still-open editor.
    rerender(
      <Wrapper>
        <TaskDescriptionEditable taskId="t-2" description="Other task description" sessions={[]} />
      </Wrapper>,
    );

    expect(isFieldGuarded("t-1", "description")).toBe(false);
    expect(isFieldGuarded("t-2", "description")).toBe(false);
    expect(screen.queryByTestId(TEXTAREA_TESTID)).toBeNull();
    expect(screen.getByTestId(READ_TESTID).textContent).toBe("Other task description");
  });

  it("ignores an old save result after the replacement task editor opens", async () => {
    let resolveUpdate: (value: never) => void = () => {};
    mockedUpdateTask.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveUpdate = resolve;
        }),
    );
    const { rerender } = render(
      <Wrapper>
        <TaskDescriptionEditable taskId="t-1" description={ORIGINAL_DESCRIPTION} sessions={[]} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId(READ_TESTID));
    const textarea = screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement;
    changeDraft(textarea, CHANGED_DESCRIPTION);

    await act(async () => {
      fireEvent.click(screen.getByTestId(SAVE_TESTID));
      await Promise.resolve();
    });

    rerender(
      <Wrapper>
        <TaskDescriptionEditable taskId="t-2" description="Other task description" sessions={[]} />
      </Wrapper>,
    );
    fireEvent.click(screen.getByTestId(READ_TESTID));
    expect((screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement).value).toBe(
      "Other task description",
    );

    await act(async () => {
      resolveUpdate({ description: CHANGED_DESCRIPTION, updated_at: UPDATED_AT } as never);
    });

    expect((screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement).value).toBe(
      "Other task description",
    );
    expect((screen.getByTestId(SAVE_TESTID) as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("TaskDescriptionEditable running-session note", () => {
  it("shows the note only while a session is RUNNING", () => {
    const sessions: TaskSession[] = [
      {
        id: "s-1",
        agentName: "agent",
        agentRole: "engineer",
        state: "RUNNING",
        isPrimary: true,
      },
    ];
    renderEditable(ORIGINAL_DESCRIPTION, sessions);
    fireEvent.click(screen.getByTestId(READ_TESTID));
    expect(screen.getByTestId(RUNNING_NOTE_TESTID)).toBeTruthy();
  });

  it("does not show the note when no session is RUNNING", () => {
    renderEditable(ORIGINAL_DESCRIPTION, []);
    fireEvent.click(screen.getByTestId(READ_TESTID));
    expect(screen.queryByTestId(RUNNING_NOTE_TESTID)).toBeNull();
  });

  it("keeps Save enabled while a session is RUNNING and the draft is dirty (AC-25)", () => {
    const sessions: TaskSession[] = [
      {
        id: "s-1",
        agentName: "agent",
        agentRole: "engineer",
        state: "RUNNING",
        isPrimary: true,
      },
    ];
    renderEditable(ORIGINAL_DESCRIPTION, sessions);
    fireEvent.click(screen.getByTestId(READ_TESTID));
    const textarea = screen.getByTestId(TEXTAREA_TESTID) as HTMLTextAreaElement;
    changeDraft(textarea, CHANGED_DESCRIPTION);
    expect(screen.getByTestId(RUNNING_NOTE_TESTID)).toBeTruthy();
    expect((screen.getByTestId(SAVE_TESTID) as HTMLButtonElement).disabled).toBe(false);
  });
});
