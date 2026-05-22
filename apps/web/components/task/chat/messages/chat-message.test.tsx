import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ChatMessage } from "./chat-message";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

const SENDER_TASK_ID = "task-sender";
const SENDER_TITLE = "Fix login bug";
const SENDER_BADGE_SELECTOR = "[data-testid='sender-task-badge']";
const PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=";
const OPEN_ATTACHMENT_1_LABEL = "Open Attachment 1";
const FULL_SIZE_ATTACHMENT_1_ALT = "Full size Attachment 1";

afterEach(() => {
  vi.restoreAllMocks();
});

function userMessage(overrides: Partial<Message>): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("sess-1"),
    task_id: toTaskId("task-target"),
    author_type: "user",
    content: "hello",
    type: "message",
    created_at: "2026-05-04T00:00:00Z",
    ...overrides,
  };
}

function wrapper(tasks: Array<{ id: string; title: string }> = []) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{
          // Seed the kanban slice so useTaskById can resolve sender tasks for
          // live-title resolution; tests that exercise the deleted-sender
          // fallback simply omit the sender from this list.
          // The full Task shape isn't required by useTaskById — only id+title.
          kanban: {
            tasks: tasks.map((t) => ({
              id: t.id,
              title: t.title,
              workflow_step_id: "",
              priority: 0,
              parent_id: undefined,
            })),
          } as unknown as never,
        }}
      >
        {children}
      </StateProvider>
    );
  };
}

function renderWithSender(
  tasks: Array<{ id: string; title: string }>,
  metadata: Partial<Message["metadata"] & object>,
) {
  const Wrapper = wrapper(tasks);
  return render(
    <Wrapper>
      <ChatMessage comment={userMessage({ metadata })} label="Message" className="" />
    </Wrapper>,
  );
}

describe("ChatMessage sender badge", () => {
  it("renders the sender badge when sender_task_id is present in metadata", () => {
    const { container } = renderWithSender([{ id: SENDER_TASK_ID, title: SENDER_TITLE }], {
      sender_task_id: SENDER_TASK_ID,
      sender_task_title: SENDER_TITLE,
      sender_session_id: "sender-sess",
    });

    const badge = container.querySelector(SENDER_BADGE_SELECTOR);
    expect(badge).not.toBeNull();
    expect(badge?.getAttribute("data-sender-task-id")).toBe(SENDER_TASK_ID);
    expect(badge?.textContent).toContain(SENDER_TITLE);
  });

  it("links the badge to the source task when the sender is loaded", () => {
    const { container } = renderWithSender([{ id: SENDER_TASK_ID, title: SENDER_TITLE }], {
      sender_task_id: SENDER_TASK_ID,
      sender_task_title: SENDER_TITLE,
    });

    const link = container.querySelector(`a[href='/t/${SENDER_TASK_ID}']`);
    expect(link).not.toBeNull();
  });

  it("renders a non-clickable greyed badge when sender task is unknown", () => {
    // No tasks seeded — sender task is "deleted" or cross-workspace.
    const { container } = renderWithSender([], {
      sender_task_id: "task-deleted",
      sender_task_title: "Old title",
    });

    const badge = container.querySelector(SENDER_BADGE_SELECTOR);
    expect(badge).not.toBeNull();
    expect(container.querySelector("a[href='/t/task-deleted']")).toBeNull();
    // Falls back to the snapshotted title rather than blanking the badge.
    expect(badge?.textContent).toContain("Old title");
  });

  it("uses the live title when it differs from the snapshot", () => {
    // The badge re-resolves the title from the kanban store so renames are
    // reflected without re-sending the message.
    const { container } = renderWithSender([{ id: SENDER_TASK_ID, title: "Renamed task" }], {
      sender_task_id: SENDER_TASK_ID,
      sender_task_title: "Old name",
    });

    const badge = container.querySelector(SENDER_BADGE_SELECTOR);
    expect(badge?.textContent).toContain("Renamed task");
    expect(badge?.textContent).not.toContain("Old name");
  });

  it("truncates very long titles for display", () => {
    const longTitle = "This is a really long task title that should be truncated";
    const { container } = renderWithSender([{ id: SENDER_TASK_ID, title: longTitle }], {
      sender_task_id: SENDER_TASK_ID,
      sender_task_title: longTitle,
    });

    const badge = container.querySelector(SENDER_BADGE_SELECTOR);
    expect(badge).not.toBeNull();
    // The badge text must contain the ellipsis (truncated) and not the full title.
    expect(badge?.textContent).toContain("…");
    expect(badge?.textContent ?? "").not.toContain(longTitle);
  });

  it("does not render a sender badge when metadata has no sender_task_id", () => {
    const { container } = renderWithSender([], { plan_mode: true });

    expect(container.querySelector(SENDER_BADGE_SELECTOR)).toBeNull();
  });
});

describe("ChatMessage image attachments", () => {
  it("opens image attachments in an in-app preview dialog", () => {
    const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
    renderWithSender([], {
      attachments: [{ type: "image", data: PNG_BASE64, mime_type: "image/png" }],
    });

    fireEvent.click(screen.getByRole("button", { name: OPEN_ATTACHMENT_1_LABEL }));

    expect(openSpy).not.toHaveBeenCalled();
    const dialog = screen.getByRole("dialog");
    const preview = screen.getByAltText(FULL_SIZE_ATTACHMENT_1_ALT);
    expect(dialog.className).toContain("w-fit");
    expect(dialog.className).toContain("max-w-[calc(100vw-1rem)]");
    expect(preview.className).toContain("w-[min(92vw,1100px)]");
    expect(preview.className).toContain("max-h-[calc(100dvh-5rem)]");
    expect(preview.getAttribute("src")).toBe(`data:image/png;base64,${PNG_BASE64}`);
  });
});
