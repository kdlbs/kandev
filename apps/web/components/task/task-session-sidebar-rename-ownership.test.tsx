import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { TaskRenameDialog } from "./task-rename-dialog";
import { useTaskRowActions } from "./task-session-sidebar";

const TOAST_CONTAINER = "toast-container";
const SUBMITTED_TITLE = "Submitted A";
const NEWER_DRAFT = "  exact newer draft  ";

// Same Rename wiring as SidebarDialogs; all composed hooks and API are real.
function PickerRenameFlow() {
  const rename = useTaskRowActions(useAppStoreApi(), []);
  return (
    <>
      <button onClick={() => rename.handleRenameTask("task-a", "Task A")}>Rename A</button>
      <button onClick={() => rename.handleRenameTask("task-b", "Task B")}>Rename B</button>
      <TaskRenameDialog
        open={rename.renamingTask !== null}
        currentTitle={rename.renamingTask?.title ?? ""}
        onSubmit={rename.handleRenameSubmit}
        onOpenChange={(open) => {
          if (!open) rename.setRenamingTask(null);
        }}
      />
    </>
  );
}

let pendingRequests: ReturnType<typeof setup> = [];

function setup() {
  vi.spyOn(console, "error").mockImplementation(() => {});
  const pending: Array<{ taskId: string; title: string; resolve: (response: Response) => void }> =
    [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: string, init?: RequestInit) => {
      expect(init?.method).toBe("PATCH");
      const taskId = new URL(input, "http://localhost").pathname.split("/").at(-1)!;
      return new Promise<Response>((resolve) => {
        pending.push({ taskId, title: JSON.parse(init?.body as string).title, resolve });
      });
    }),
  );
  render(
    <StateProvider>
      <ToastProvider>
        <PickerRenameFlow />
      </ToastProvider>
    </StateProvider>,
  );
  pendingRequests = pending;
  return pending;
}

function openDraft(target: "A" | "B", value: string) {
  fireEvent.click(screen.getByRole("button", { name: `Rename ${target}` }));
  const input = screen.getByRole("textbox") as HTMLInputElement;
  fireEvent.change(input, { target: { value } });
  expect(input.value).toBe(value);
  return input;
}

async function save() {
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(screen.queryByRole("textbox")).toBeNull());
}

async function settle(request: ReturnType<typeof setup>[number], failure = false) {
  await act(async () => {
    request.resolve(
      new Response(
        JSON.stringify(
          failure
            ? { error: "Rename unavailable" }
            : {
                id: request.taskId,
                title: request.title,
              },
        ),
        { status: failure ? 503 : 200, headers: { "Content-Type": "application/json" } },
      ),
    );
  });
}

afterEach(async () => {
  // Settle every admitted transport even when a regression assertion fails.
  await act(async () => {
    for (const request of pendingRequests) {
      request.resolve(new Response(JSON.stringify({ id: request.taskId, title: request.title })));
    }
  });
  pendingRequests = [];
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// @covers AC-UI-EDIT-FOCUS-001.7
it.each(["A", "B"] as const)(
  "preserves a newer %s draft through old success and explicit Save",
  async (target) => {
    const pending = setup();
    openDraft("A", "  Submitted A  ");
    await save();
    expect(pending).toHaveLength(1);
    expect(pending[0]).toMatchObject({ taskId: "task-a", title: SUBMITTED_TITLE });
    const input = openDraft(target, NEWER_DRAFT);
    input.setSelectionRange(5, 5);
    await settle(pending[0]);
    expect(screen.getByRole("textbox")).toBe(input);
    expect(input.value).toBe(NEWER_DRAFT);
    expect(input.selectionStart).toBe(5);
    expect(screen.getByTestId(TOAST_CONTAINER).textContent).toBe("");
    await save();
    expect(pending).toHaveLength(2);
    expect(pending[1]).toMatchObject({
      taskId: target === "A" ? "task-a" : "task-b",
      title: "exact newer draft",
    });
    await settle(pending[1]);
    expect(screen.queryByRole("textbox")).toBeNull();
  },
);

it.each(["A", "B"] as const)(
  "preserves a newer %s draft and legitimate old error feedback, then Cancel",
  async (target) => {
    const pending = setup();
    openDraft("A", SUBMITTED_TITLE);
    await save();
    const input = openDraft(target, "  untouched draft  ");
    await settle(pending[0], true);
    expect(screen.getByRole("textbox")).toBe(input);
    expect(input.value).toBe("  untouched draft  ");
    expect(screen.getByTestId(TOAST_CONTAINER).textContent).toBe("");
    expect(console.error).toHaveBeenCalledWith("Failed to rename task:", expect.any(Error));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(pending).toHaveLength(1);
  },
);

it.each([false, true])(
  "keeps ordinary immediate dismissal after settlement (failure=%s)",
  async (failure) => {
    const pending = setup();
    openDraft("A", SUBMITTED_TITLE);
    await save();
    await settle(pending[0], failure);
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(screen.getByTestId(TOAST_CONTAINER).textContent).toBe("");
    expect(console.error).toHaveBeenCalledTimes(failure ? 1 : 0);
  },
);

it.each([false, true])(
  "preserves a third same-task edit through reverse request completion (older failure=%s)",
  async (failure) => {
    const pending = setup();
    openDraft("A", "First A");
    await save();
    openDraft("B", "Second B");
    await save();
    const input = openDraft("A", "Third unsaved A");
    await settle(pending[1]);
    expect(screen.getByRole("textbox")).toBe(input);
    expect(input.value).toBe("Third unsaved A");
    await settle(pending[0], failure);
    expect(screen.getByRole("textbox")).toBe(input);
    expect(input.value).toBe("Third unsaved A");
    expect(pending.map(({ taskId, title }) => ({ taskId, title }))).toEqual([
      { taskId: "task-a", title: "First A" },
      { taskId: "task-b", title: "Second B" },
    ]);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("textbox")).toBeNull();
  },
);

// The real dialog closes immediately; direct producer admission covers a still-current owner.
it.each([false, true])(
  "clears its own current edit after settlement (failure=%s)",
  async (failure) => {
    let resolve!: (response: Response) => void;
    vi.spyOn(console, "error").mockImplementation(() => {});
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((done) => {
            resolve = done;
          }),
      ),
    );
    const { result } = renderHook(() => useTaskRowActions(useAppStoreApi(), []), {
      wrapper: ({ children }) => (
        <StateProvider>
          <ToastProvider>{children}</ToastProvider>
        </StateProvider>
      ),
    });
    act(() => result.current.handleRenameTask("task-a", "Task A"));
    let submitted!: Promise<void>;
    act(() => {
      submitted = result.current.handleRenameSubmit(SUBMITTED_TITLE);
    });
    expect(result.current.renamingTask?.id).toBe("task-a");
    await act(async () => {
      resolve(
        new Response(
          JSON.stringify(
            failure ? { error: "Rename unavailable" } : { id: "task-a", title: SUBMITTED_TITLE },
          ),
          { status: failure ? 503 : 200 },
        ),
      );
      await submitted;
    });
    expect(result.current.renamingTask).toBeNull();
    expect(console.error).toHaveBeenCalledTimes(failure ? 1 : 0);
  },
);
