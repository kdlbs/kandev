import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { officeTaskCommentScope, useCommentsStore } from "@/lib/state/slices/comments";
import { ActiveSessionRefProvider } from "./components/active-session-ref-context";
import { TaskChat } from "./task-chat";

const TASK_ID = "extract-task";
const bodies: unknown[] = [];

function seedExtract(id: string, authorName: string, quote: string, text: string) {
  useCommentsStore.getState().addComment({
    id,
    sessionId: officeTaskCommentScope(TASK_ID),
    source: "agent-message",
    messageId: `c-${id}`,
    authorName,
    selectedText: quote,
    anchor: {
      messageId: `c-${id}`,
      start: 0,
      end: quote.length,
      selectedText: quote,
      prefix: "",
      suffix: "",
    },
    text,
    createdAt: "",
    status: "pending",
  });
}

beforeEach(() => {
  bodies.length = 0;
  window.sessionStorage.clear();
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
  vi.stubGlobal(
    "fetch",
    vi.fn((_input: RequestInfo | URL, init: RequestInit = {}) => {
      bodies.push(JSON.parse(String(init.body)));
      return Promise.resolve(
        new Response(JSON.stringify({ comment: { id: "x", task_id: TASK_ID } }), { status: 201 }),
      );
    }),
  );
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount() {
  render(
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          <ActiveSessionRefProvider>
            <TaskChat taskId={TASK_ID} comments={[]} sessions={[]} />
          </ActiveSessionRefProvider>
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
  return within(screen.getByTestId("task-chat-root"));
}

describe("TaskChat thread extracts", () => {
  it("shows one chip per extract with the author and posts the formatted block", async () => {
    seedExtract("a", "CEO", "ship the brief", "Cut it down.");
    seedExtract("b", "Worker", "ten days", "Why ten?");
    const root = mount();

    expect(root.getByText("CEO: “ship the brief”")).toBeTruthy();
    expect(root.getByText("Worker: “ten days”")).toBeTruthy();

    fireEvent.change(root.getByRole("textbox"), { target: { value: "Please revise." } });
    await act(async () => {
      fireEvent.keyDown(root.getByRole("textbox"), { key: "Enter" });
    });

    const { body } = bodies[0] as { body: string };
    expect(body).toContain("Selected comment from CEO:\n> ship the brief");
    expect(body).toContain("Selected comment from Worker:\n> ten days");
    expect(body.endsWith("Please revise.")).toBe(true);
    expect(useCommentsStore.getState().pendingForChat).toEqual([]);
  });

  it("can post extracts without extra text", async () => {
    seedExtract("a", "CEO", "ship the brief", "Cut it down.");
    const root = mount();
    await act(async () => {
      fireEvent.keyDown(root.getByRole("textbox"), { key: "Enter" });
    });
    expect((bodies[0] as { body: string }).body).toContain("> Cut it down.");
  });
});
