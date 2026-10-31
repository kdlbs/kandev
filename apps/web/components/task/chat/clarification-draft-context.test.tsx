import { act, cleanup, render, screen } from "@testing-library/react";
import { useEffect } from "react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ClarificationDraftProvider,
  useClarificationDraft,
  useClarificationResizeState,
} from "./clarification-draft-context";

const answers = { q1: { question_id: "q1", selected_options: ["yes"] } };
const TASK_ID = "task";
const CLARIFICATION_BUNDLE = "source:bundle";
const RESIZE_STATE_TEST_ID = "resize-state";
function Draft({ bundle = CLARIFICATION_BUNDLE, settled = false }) {
  const draft = useClarificationDraft(bundle, answers, settled);
  return (
    <>
      <button
        onClick={() => {
          draft.setCustomDrafts({ q2: "Keep this" });
          draft.setActiveIndex(1);
        }}
      >
        Edit
      </button>
      <output data-testid="draft">
        {JSON.stringify([draft.customDrafts, draft.rawActiveIndex, draft.savedAnswers])}
      </output>
    </>
  );
}
function Page({
  mount,
  bundle,
  settled = false,
}: {
  mount: string;
  bundle?: string;
  settled?: boolean;
}) {
  return (
    <ClarificationDraftProvider taskId={TASK_ID}>
      <Draft key={mount} bundle={bundle} settled={settled} />
    </ClarificationDraftProvider>
  );
}
function ResizeState({ bundle = CLARIFICATION_BUNDLE }: { bundle?: string }) {
  const sizing = useClarificationResizeState(bundle);
  return (
    <>
      <button onClick={() => sizing.setDesktopHeight(810)}>Set desktop size</button>
      <button onClick={() => sizing.setPhoneHeight(432)}>Set phone size</button>
      <output data-testid={RESIZE_STATE_TEST_ID}>
        {JSON.stringify([sizing.desktopHeight, sizing.phoneHeight])}
      </output>
    </>
  );
}
afterEach(cleanup);

it("retains text-only drafts, position, and keyed choices across a layout remount", () => {
  const { rerender } = render(<Page mount="desktop" />);
  act(() => screen.getByRole("button").click());
  rerender(<Page mount="phone" />);
  expect(screen.getByTestId("draft").textContent).toBe(
    JSON.stringify([{ q2: "Keep this" }, 1, [answers.q1]]),
  );
});

it("does not reuse a draft for another source or bundle", () => {
  const { rerender } = render(<Page mount="desktop" />);
  act(() => screen.getByRole("button").click());
  rerender(<Page mount="phone" bundle="other:bundle" />);
  expect(screen.getByTestId("draft").textContent).toBe(JSON.stringify([{}, 0, undefined]));
});

it("clears a settled bundle and its cached answer state", () => {
  const { rerender } = render(<Page mount="desktop" />);
  act(() => screen.getByRole("button").click());
  rerender(<Page mount="desktop" settled />);
  rerender(<Page mount="phone" settled />);
  expect(screen.getByTestId("draft").textContent).toBe(JSON.stringify([{}, 0, undefined]));
});

it("clears task-owned drafts without remounting the layout", () => {
  const mounted = vi.fn();
  function Layout() {
    useEffect(mounted, []);
    return <Draft />;
  }
  const page = (taskId: string) => (
    <ClarificationDraftProvider taskId={taskId}>
      <Layout />
    </ClarificationDraftProvider>
  );
  const { rerender } = render(page("task-a"));
  act(() => screen.getByRole("button").click());
  rerender(page("task-b"));
  expect(screen.getByTestId("draft").textContent).toBe(JSON.stringify([{}, 0, undefined]));
  rerender(page("task-a"));
  expect(screen.getByTestId("draft").textContent).toBe(JSON.stringify([{}, 0, undefined]));
  expect(mounted).toHaveBeenCalledTimes(1);
});

it("retains phone and desktop sizes across responsive remounts by bundle", () => {
  const page = (mount: string, bundle = CLARIFICATION_BUNDLE, taskId = TASK_ID) => (
    <ClarificationDraftProvider taskId={taskId}>
      <ResizeState key={mount} bundle={bundle} />
    </ClarificationDraftProvider>
  );
  const { rerender } = render(page("desktop"));
  act(() => {
    screen.getByRole("button", { name: "Set desktop size" }).click();
    screen.getByRole("button", { name: "Set phone size" }).click();
  });

  rerender(page("phone"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(JSON.stringify([810, 432]));

  rerender(page("new-phone", "other:bundle"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(JSON.stringify([null, null]));

  rerender(page("desktop-again"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(JSON.stringify([810, 432]));

  rerender(page("task-switch", CLARIFICATION_BUNDLE, "other-task"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(JSON.stringify([null, null]));
  rerender(page("task-return"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(JSON.stringify([null, null]));
});

it("bounds retained clarification sizes to the most recently used bundles", () => {
  const page = (mount: string, bundle: string) => (
    <ClarificationDraftProvider taskId={TASK_ID}>
      <ResizeState key={mount} bundle={bundle} />
    </ClarificationDraftProvider>
  );
  const { rerender } = render(page("bundle-0", "bundle-0"));
  act(() => screen.getByRole("button", { name: "Set desktop size" }).click());

  for (let index = 1; index <= 8; index += 1) {
    rerender(page(`bundle-${index}`, `bundle-${index}`));
    act(() => screen.getByRole("button", { name: "Set desktop size" }).click());
  }

  rerender(page("old-bundle", "bundle-0"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(JSON.stringify([null, null]));
  rerender(page("recent-bundle", "bundle-8"));
  expect(screen.getByTestId(RESIZE_STATE_TEST_ID).textContent).toBe(JSON.stringify([810, null]));
});
