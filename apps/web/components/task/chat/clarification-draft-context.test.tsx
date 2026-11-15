import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { createRef, useEffect, useState, type ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { TooltipProvider } from "@kandev/ui/tooltip";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type ClarificationAnswer,
  type ClarificationRequestMetadata,
  type Message,
} from "@/lib/types/http";
import { ClarificationInputOverlay } from "./clarification-input-overlay";
import {
  ClarificationDraftProvider,
  useClarificationDraft,
  useClarificationResizeState,
} from "./clarification-draft-context";

const TASK_ID = "task";
const CLARIFICATION_BUNDLE = "source:bundle";
const RESIZE_STATE_TEST_ID = "resize-state";
const DRAFT_STATE_TEST_ID = "draft";
const CLARIFICATION_OPTION_TEST_ID = "clarification-option";
const CLARIFICATION_INPUT_TEST_ID = "clarification-input";

function Draft({
  bundle = CLARIFICATION_BUNDLE,
  settled = false,
}: {
  bundle?: string;
  settled?: boolean;
}) {
  const [answers, setAnswers] = useState<Record<string, ClarificationAnswer>>({});
  const draft = useClarificationDraft(bundle, answers, settled);
  const restoredKey = JSON.stringify(draft.savedAnswers ?? []);
  useEffect(() => {
    if (!draft.savedAnswers?.length) return;
    setAnswers((current) => {
      const next = { ...current };
      for (const answer of draft.savedAnswers ?? []) next[answer.question_id] = answer;
      return next;
    });
  }, [restoredKey]);

  return (
    <>
      <button
        onClick={() => {
          draft.setCustomDrafts({ q2: "Keep this" });
          draft.setActiveIndex(1);
          setAnswers({ q1: { question_id: "q1", selected_options: ["yes"] } });
        }}
      >
        Edit
      </button>
      <button
        onClick={() => {
          draft.setCustomDrafts({});
          draft.setActiveIndex(0);
          setAnswers({});
        }}
      >
        Clear
      </button>
      <output data-testid={DRAFT_STATE_TEST_ID}>
        {JSON.stringify([draft.customDrafts, draft.rawActiveIndex, draft.savedAnswers])}
      </output>
    </>
  );
}

function TaskPage({ taskId, bundle }: { taskId: string; bundle?: string }) {
  return (
    <ClarificationDraftProvider taskId={taskId}>
      <Draft key={taskId} bundle={bundle} />
    </ClarificationDraftProvider>
  );
}

function Root({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

const RETAINED_DRAFT = JSON.stringify([
  { q2: "Keep this" },
  1,
  [{ question_id: "q1", selected_options: ["yes"] }],
]);
const EMPTY_DRAFT = JSON.stringify([{}, 0, undefined]);

function editDraft() {
  act(() => screen.getByRole("button", { name: "Edit" }).click());
}

function readDraft() {
  return screen.getByTestId(DRAFT_STATE_TEST_ID).textContent;
}

afterEach(cleanup);

it("retains unsubmitted answers across task navigation", () => {
  const { rerender } = render(
    <Root>
      <TaskPage taskId="task-a" />
    </Root>,
  );
  editDraft();

  rerender(
    <Root>
      <TaskPage taskId="task-b" />
    </Root>,
  );
  expect(readDraft()).toBe(EMPTY_DRAFT);

  rerender(
    <Root>
      <TaskPage taskId="task-a" />
    </Root>,
  );
  expect(readDraft()).toBe(RETAINED_DRAFT);
});

it("retains unsubmitted answers after task-page unmount", () => {
  const { rerender } = render(
    <Root>
      <TaskPage taskId="task-a" />
    </Root>,
  );
  editDraft();

  rerender(<Root>{null}</Root>);
  rerender(
    <Root>
      <TaskPage taskId="task-a" />
    </Root>,
  );

  expect(readDraft()).toBe(RETAINED_DRAFT);
});

it("retains text-only drafts, position, and keyed choices across a layout remount", () => {
  const { rerender } = render(
    <Root>
      <TaskPage taskId={TASK_ID} />
    </Root>,
  );
  editDraft();

  rerender(
    <Root>
      <ClarificationDraftProvider taskId={TASK_ID}>
        <Draft key="phone" />
      </ClarificationDraftProvider>
    </Root>,
  );
  expect(readDraft()).toBe(RETAINED_DRAFT);
});

it("does not revive a draft the user cleared after restoring it", () => {
  const { rerender } = render(
    <Root>
      <TaskPage taskId="task-cleared" />
    </Root>,
  );
  editDraft();
  rerender(<Root>{null}</Root>);
  rerender(
    <Root>
      <TaskPage taskId="task-cleared" />
    </Root>,
  );
  expect(readDraft()).toBe(RETAINED_DRAFT);

  act(() => screen.getByRole("button", { name: "Clear" }).click());
  rerender(<Root>{null}</Root>);
  rerender(
    <Root>
      <TaskPage taskId="task-cleared" />
    </Root>,
  );
  expect(readDraft()).toBe(EMPTY_DRAFT);
});

it("does not reuse a draft for another source or bundle", () => {
  const { rerender } = render(
    <Root>
      <TaskPage taskId={TASK_ID} />
    </Root>,
  );
  editDraft();

  rerender(
    <Root>
      <TaskPage taskId={TASK_ID} bundle="other:bundle" />
    </Root>,
  );
  expect(readDraft()).toBe(EMPTY_DRAFT);
});

it("clears a settled bundle and its cached answer state", () => {
  const { rerender } = render(
    <Root>
      <TaskPage taskId={TASK_ID} />
    </Root>,
  );
  editDraft();

  rerender(
    <Root>
      <ClarificationDraftProvider taskId={TASK_ID}>
        <Draft key="settled" settled />
      </ClarificationDraftProvider>
    </Root>,
  );
  rerender(
    <Root>
      <ClarificationDraftProvider taskId={TASK_ID}>
        <Draft key="revisit" />
      </ClarificationDraftProvider>
    </Root>,
  );
  expect(readDraft()).toBe(EMPTY_DRAFT);
});

it("keeps the task workbench mounted when the task changes", () => {
  const mounted = vi.fn();
  function Layout() {
    useEffect(mounted, []);
    return <Draft />;
  }
  const page = (taskId: string) => (
    <Root>
      <ClarificationDraftProvider taskId={taskId}>
        <Layout />
      </ClarificationDraftProvider>
    </Root>
  );
  const { rerender } = render(page("task-a"));
  rerender(page("task-b"));
  rerender(page("task-a"));
  expect(mounted).toHaveBeenCalledTimes(1);
});

const AUTO_SIZE = { mode: "auto", height: null, restore: null };
const MANUAL_DESKTOP_SIZE = { mode: "manual", height: 810, restore: null };
const MANUAL_PHONE_SIZE = { mode: "manual", height: 432, restore: null };
const EXPANDED_AUTO_SIZE = {
  mode: "expanded",
  height: null,
  restore: { mode: "auto", height: null },
};
const EXPANDED_MANUAL_SIZE = {
  mode: "expanded",
  height: null,
  restore: { mode: "manual", height: 810 },
};

function ResizeState({ bundle = CLARIFICATION_BUNDLE }: { bundle?: string }) {
  const sizing = useClarificationResizeState(bundle);
  return (
    <>
      <button onClick={() => sizing.setDesktopSize({ mode: "manual", height: 810, restore: null })}>
        Set desktop size
      </button>
      <button onClick={() => sizing.setPhoneSize({ mode: "manual", height: 432, restore: null })}>
        Set phone size
      </button>
      <button
        onClick={() =>
          sizing.setDesktopSize({
            mode: "expanded",
            height: null,
            restore: { mode: "auto", height: null },
          })
        }
      >
        Expand desktop from auto
      </button>
      <button
        onClick={() =>
          sizing.setDesktopSize({
            mode: "expanded",
            height: null,
            restore: { mode: "manual", height: 810 },
          })
        }
      >
        Expand desktop from manual
      </button>
      <output data-testid={RESIZE_STATE_TEST_ID}>
        {JSON.stringify([sizing.desktop, sizing.phone])}
      </output>
    </>
  );
}

function resizePage(mount: string, bundle = CLARIFICATION_BUNDLE, taskId = TASK_ID) {
  return (
    <ClarificationDraftProvider taskId={taskId}>
      <ResizeState key={mount} bundle={bundle} />
    </ClarificationDraftProvider>
  );
}

it("retains phone and desktop sizes across responsive remounts by bundle", () => {
  const { rerender } = render(resizePage("desktop"));
  act(() => {
    screen.getByRole("button", { name: "Set desktop size" }).click();
    screen.getByRole("button", { name: "Set phone size" }).click();
  });

  rerender(resizePage("phone"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([MANUAL_DESKTOP_SIZE, MANUAL_PHONE_SIZE]),
  );

  rerender(resizePage("new-phone", "other:bundle"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([AUTO_SIZE, AUTO_SIZE]),
  );

  rerender(resizePage("desktop-again"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([MANUAL_DESKTOP_SIZE, MANUAL_PHONE_SIZE]),
  );

  rerender(resizePage("task-switch", CLARIFICATION_BUNDLE, "other-task"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([AUTO_SIZE, AUTO_SIZE]),
  );
  rerender(resizePage("task-return"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([AUTO_SIZE, AUTO_SIZE]),
  );
});

it("retains an expanded auto target with no pixel height across responsive remounts", () => {
  const { rerender } = render(resizePage("desktop"));
  act(() => screen.getByRole("button", { name: "Expand desktop from auto" }).click());
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([EXPANDED_AUTO_SIZE, AUTO_SIZE]),
  );

  rerender(resizePage("phone"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([EXPANDED_AUTO_SIZE, AUTO_SIZE]),
  );

  rerender(resizePage("other-bundle", "other:bundle"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([AUTO_SIZE, AUTO_SIZE]),
  );

  rerender(resizePage("return"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([EXPANDED_AUTO_SIZE, AUTO_SIZE]),
  );
});

it("retains an expanded manual Restore height across responsive remounts", () => {
  const { rerender } = render(resizePage("desktop"));
  act(() => screen.getByRole("button", { name: "Expand desktop from manual" }).click());
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([EXPANDED_MANUAL_SIZE, AUTO_SIZE]),
  );

  rerender(resizePage("phone"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([EXPANDED_MANUAL_SIZE, AUTO_SIZE]),
  );
});

it("bounds retained clarification sizes to the most recently used bundles", () => {
  const { rerender } = render(resizePage("bundle-0", "bundle-0"));
  act(() => screen.getByRole("button", { name: "Set desktop size" }).click());

  for (let index = 1; index <= 8; index += 1) {
    rerender(resizePage(`bundle-${index}`, `bundle-${index}`));
    act(() => screen.getByRole("button", { name: "Set desktop size" }).click());
  }

  rerender(resizePage("old-bundle", "bundle-0"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([AUTO_SIZE, AUTO_SIZE]),
  );
  rerender(resizePage("recent-bundle", "bundle-8"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(
    JSON.stringify([MANUAL_DESKTOP_SIZE, AUTO_SIZE]),
  );
});

type AppStore = ReturnType<typeof useAppStoreApi>;

function StoreCapture({ onStore }: { onStore: (store: AppStore) => void }) {
  const store = useAppStoreApi();
  useEffect(() => onStore(store), [onStore, store]);
  return null;
}

function authorityMessages(): Message[] {
  return [0, 1].map((index) => ({
    id: `authority-${index}`,
    session_id: toSessionId("authority-session"),
    task_id: toTaskId("authority-task"),
    author_type: "agent" as const,
    content: "Question",
    type: "clarification_request" as const,
    created_at: "2026-11-03T00:00:00Z",
    metadata: {
      pending_id: "authority-pending",
      session_id: "authority-session",
      question_id: `aq${index}`,
      question_index: index,
      question_total: 2,
      status: "pending",
      question: {
        id: `aq${index}`,
        title: "",
        prompt: `Question ${index + 1}?`,
        options: [{ option_id: "ao1", label: "Option 1", description: "" }],
      },
    } satisfies ClarificationRequestMetadata,
  }));
}

it("admits fresh drafts after an authority generation change without unmounting", async () => {
  const messages = authorityMessages();
  const scopeRef = createRef<HTMLDivElement>();
  const onResolved = vi.fn();
  const onDismiss = vi.fn();
  let store: AppStore | null = null;
  const onStore = (value: AppStore) => {
    store = value;
  };
  const page = (mounted: boolean) => (
    <StateProvider>
      <StoreCapture onStore={onStore} />
      {mounted ? (
        <ClarificationDraftProvider taskId="authority-task">
          <TooltipProvider>
            <div ref={scopeRef} tabIndex={-1}>
              <ClarificationInputOverlay
                messages={messages}
                onResolved={onResolved}
                onDismiss={onDismiss}
                shortcutScopeRef={scopeRef}
              />
            </div>
          </TooltipProvider>
        </ClarificationDraftProvider>
      ) : null}
    </StateProvider>
  );

  const { rerender } = render(page(true));
  fireEvent.click(screen.getByTestId(CLARIFICATION_OPTION_TEST_ID));
  fireEvent.change(screen.getByTestId(CLARIFICATION_INPUT_TEST_ID), {
    target: { value: "old draft" },
  });

  act(() => {
    store!.getState().setAuthState({
      mode: "enabled",
      authenticated: true,
      user: {
        id: "user-1",
        email: "u@example.com",
        display_name: "User One",
        role: "admin",
        status: "active",
      },
    });
  });

  await vi.waitFor(() =>
    expect((screen.getByTestId(CLARIFICATION_INPUT_TEST_ID) as HTMLTextAreaElement).value).toBe(""),
  );
  expect(screen.getAllByTestId("clarification-step")[0].getAttribute("data-active")).toBe("true");
  expect(screen.getByTestId(CLARIFICATION_OPTION_TEST_ID).getAttribute("data-selected")).not.toBe(
    "true",
  );

  fireEvent.click(screen.getByTestId(CLARIFICATION_OPTION_TEST_ID));
  fireEvent.change(screen.getByTestId(CLARIFICATION_INPUT_TEST_ID), {
    target: { value: "fresh draft" },
  });

  rerender(page(false));
  rerender(page(true));

  await vi.waitFor(() =>
    expect(screen.getAllByTestId("clarification-step")[1].getAttribute("data-active")).toBe("true"),
  );
  expect((screen.getByTestId(CLARIFICATION_INPUT_TEST_ID) as HTMLTextAreaElement).value).toBe(
    "fresh draft",
  );
  fireEvent.click(screen.getAllByTestId("clarification-step")[0]);
  expect(screen.getByTestId(CLARIFICATION_OPTION_TEST_ID).getAttribute("data-selected")).toBe(
    "true",
  );
});
