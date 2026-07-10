import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ChatMessage } from "./chat-message";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type CustomPrompt,
  type Message,
  type TaskSession,
} from "@/lib/types/http";

const SENDER_TASK_ID = "task-sender";
const SENDER_TITLE = "Fix login bug";
const SENDER_BADGE_SELECTOR = "[data-testid='sender-task-badge']";
const MESSAGE_TIMESTAMP = "2026-05-04T00:00:00Z";
const PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=";
const OPEN_ATTACHMENT_1_LABEL = "Open Attachment 1";
const FULL_SIZE_ATTACHMENT_1_ALT = "Full size Attachment 1";

afterEach(() => {
  cleanup();
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
    created_at: MESSAGE_TIMESTAMP,
    ...overrides,
  };
}

function customPrompt(name: string): CustomPrompt {
  return {
    id: `prompt-${name}`,
    name,
    content: `${name} content`,
    builtin: false,
    created_at: MESSAGE_TIMESTAMP,
    updated_at: MESSAGE_TIMESTAMP,
  };
}

function wrapper(tasks: Array<{ id: string; title: string }> = [], prompts: CustomPrompt[] = []) {
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
          prompts: { items: prompts, loaded: true, loading: false },
        }}
      >
        {children}
      </StateProvider>
    );
  };
}

describe("ChatMessage prompt mentions", () => {
  it("renders saved prompt mentions as visual chips", () => {
    const Wrapper = wrapper([], [customPrompt("hello")]);

    render(
      <Wrapper>
        <ChatMessage
          comment={userMessage({ content: "@hello and @missing" })}
          label="Message"
          className=""
        />
      </Wrapper>,
    );

    const chips = screen.getAllByTestId("custom-prompt-mention");
    expect(chips).toHaveLength(1);
    const [chip] = chips;
    expect(chip.textContent).toBe("@hello");
    expect(screen.getByText(/and @missing/)).not.toBeNull();
  });

  it("preserves GFM attributes while highlighting prompt mentions", () => {
    const Wrapper = wrapper([], [customPrompt("hello")]);

    render(
      <Wrapper>
        <ChatMessage
          comment={userMessage({
            content: "- [x] @hello\n\n| Name |\n| :---: |\n| @hello |",
          })}
          label="Message"
          className=""
        />
      </Wrapper>,
    );

    const checkbox = screen.getByRole("checkbox") as HTMLInputElement;
    expect(screen.getAllByTestId("custom-prompt-mention")).toHaveLength(2);
    expect(checkbox.checked).toBe(true);
    expect(checkbox.closest("li")?.className).toContain("task-list-item");
    expect(screen.getByRole("cell").getAttribute("style")).toContain("text-align: center");
  });
});

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

function renderAgentMessageWithSession(session: Partial<TaskSession>, metadata = {}) {
  const taskSession: TaskSession = {
    id: toSessionId("sess-1"),
    task_id: toTaskId("task-target"),
    state: "COMPLETED",
    started_at: MESSAGE_TIMESTAMP,
    updated_at: MESSAGE_TIMESTAMP,
    ...session,
  };
  const Wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider
      initialState={{
        taskSessions: { items: { "sess-1": taskSession } },
      }}
    >
      {children}
    </StateProvider>
  );

  return render(
    <Wrapper>
      <ChatMessage
        comment={userMessage({ author_type: "agent", metadata })}
        label="Message"
        className=""
      />
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

  it("renders the workflow step badge when workflow metadata is present", () => {
    const { container } = renderWithSender([], {
      workflow_message: true,
      workflow_step_name: "Review",
      workflow_step_color: "bg-emerald-500",
    });

    const badge = container.querySelector("[data-testid='workflow-message-badge']");
    expect(badge).not.toBeNull();
    expect(badge?.textContent).toContain("Review");
    expect(container.querySelector("[data-testid='workflow-message-dot']")?.className).toContain(
      "bg-emerald-500",
    );
  });

  it("falls back when workflow metadata has an unknown color class", () => {
    const { container } = renderWithSender([], {
      workflow_message: true,
      workflow_step_name: "Review",
      workflow_step_color: "unknown-step-color",
    });

    expect(container.querySelector("[data-testid='workflow-message-dot']")?.className).toContain(
      "bg-neutral-400",
    );
  });
});

describe("ChatMessage raw view", () => {
  it("shows user raw_content with hidden kandev-system blocks when raw view is enabled", () => {
    const raw = `<kandev-system>This message was sent by an agent working in task "Sender" (${SENDER_TASK_ID}).</kandev-system>

@improve-task

<kandev-system>EXPANDED PROMPT REFERENCES: The message above references saved prompts by @name.

### @improve-task
Review this task for durable improvements.</kandev-system>`;
    const Wrapper = wrapper([{ id: SENDER_TASK_ID, title: SENDER_TITLE }]);

    render(
      <Wrapper>
        <ChatMessage
          comment={userMessage({
            content: "@improve-task",
            raw_content: raw,
            metadata: {
              sender_task_id: SENDER_TASK_ID,
              sender_task_title: SENDER_TITLE,
              has_hidden_prompts: true,
            },
          })}
          label="Message"
          className=""
        />
      </Wrapper>,
    );

    expect(screen.queryByText(/EXPANDED PROMPT REFERENCES/)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Show raw text" }));

    expect(screen.getByText(/This message was sent by an agent/)).not.toBeNull();
    expect(screen.getByText(/EXPANDED PROMPT REFERENCES/)).not.toBeNull();
    expect(screen.getByText(/### @improve-task/)).not.toBeNull();
    expect(screen.getByText(/Review this task for durable improvements/)).not.toBeNull();
  });

  it("shows agent raw_content with hidden kandev-system blocks when raw view is enabled", () => {
    const raw = `<kandev-system>Hidden agent context.</kandev-system>

Visible agent response.`;
    const Wrapper = wrapper([]);

    render(
      <Wrapper>
        <ChatMessage
          comment={userMessage({
            author_type: "agent",
            content: "Visible agent response.",
            raw_content: raw,
          })}
          label="Message"
          className=""
        />
      </Wrapper>,
    );

    expect(screen.queryByText(/Hidden agent context/)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Show raw text" }));

    expect(screen.getByText(/Hidden agent context/)).not.toBeNull();
    expect(screen.getByText(/Visible agent response/)).not.toBeNull();
  });
});

describe("ChatMessage agent session config metadata", () => {
  it("opens markdown file links with the provided chat file opener", () => {
    const onOpenFile = vi.fn();
    const Wrapper = wrapper([]);

    render(
      <Wrapper>
        <ChatMessage
          comment={userMessage({
            author_type: "agent",
            content: "[spec.md](/root/.kandev/tasks/example/kandev/docs/specs/native/spec.md)",
          })}
          label="Message"
          className=""
          worktreePath="/root/.kandev/tasks/example/kandev"
          onOpenFile={onOpenFile}
        />
      </Wrapper>,
    );

    fireEvent.click(screen.getByRole("link", { name: "spec.md" }));

    expect(onOpenFile).toHaveBeenCalledWith("docs/specs/native/spec.md");
  });

  it("shows session config options next to the model", () => {
    renderAgentMessageWithSession({
      agent_profile_snapshot: {
        model: "gpt-5.5",
        config_options: {
          reasoning_effort: "high",
          verbosity: "low",
        },
      },
    });

    expect(screen.getByText("gpt-5.5 · Reasoning effort: high · Verbosity: low")).not.toBeNull();
  });

  it("prefers live runtime config over the profile snapshot", () => {
    renderAgentMessageWithSession({
      metadata: {
        runtime_config: {
          model: "gpt-5.6",
          mode: "accept-edits",
          config_options: {
            reasoning_effort: "medium",
          },
        },
      },
      agent_profile_snapshot: {
        model: "gpt-5.5",
        mode: "default",
        config_options: {
          reasoning_effort: "high",
        },
      },
    });

    expect(
      screen.getByText("gpt-5.6 · Mode: accept-edits · Reasoning effort: medium"),
    ).not.toBeNull();
  });

  it("merges runtime config options over snapshot options per option", () => {
    renderAgentMessageWithSession({
      metadata: {
        runtime_config: {
          config_options: {
            reasoning_effort: "medium",
          },
        },
      },
      agent_profile_snapshot: {
        model: "gpt-5.5",
        config_options: {
          reasoning_effort: "high",
          verbosity: "low",
        },
      },
    });

    expect(
      screen.getAllByText("gpt-5.5 · Reasoning effort: medium · Verbosity: low").length,
    ).toBeGreaterThan(0);
  });

  it("keeps merged runtime-only config options sorted", () => {
    renderAgentMessageWithSession({
      metadata: {
        runtime_config: {
          config_options: {
            reasoning_effort: "medium",
          },
        },
      },
      agent_profile_snapshot: {
        model: "gpt-5.5",
        config_options: {
          verbosity: "low",
        },
      },
    });

    expect(
      screen.getAllByText("gpt-5.5 · Reasoning effort: medium · Verbosity: low").length,
    ).toBeGreaterThan(0);
  });
});

describe("ChatMessage agent session config metadata overrides", () => {
  it("does not fall back to snapshot options when runtime options are explicitly empty", () => {
    const { container } = renderAgentMessageWithSession({
      metadata: {
        runtime_config: {
          config_options: {},
        },
      },
      agent_profile_snapshot: {
        model: "gpt-5.5",
        config_options: {
          reasoning_effort: "high",
        },
      },
    });

    expect(screen.getByText("gpt-5.5")).not.toBeNull();
    expect(container.textContent).not.toContain("Reasoning effort");
  });

  it("keeps message-level model attribution while showing session options", () => {
    renderAgentMessageWithSession(
      {
        agent_profile_snapshot: {
          model: "gpt-5.5",
          config_options: {
            reasoning_effort: "high",
          },
        },
      },
      { model: "gpt-5.5-mini" },
    );

    expect(screen.getByText("gpt-5.5-mini · Reasoning effort: high")).not.toBeNull();
  });

  it("uses message-level config options when message metadata provides them", () => {
    const { container } = renderAgentMessageWithSession(
      {
        agent_profile_snapshot: {
          model: "gpt-5.5",
          config_options: {
            reasoning_effort: "high",
          },
        },
      },
      {
        model: "gpt-5.5-mini",
        config_options: {
          reasoning_effort: "low",
        },
      },
    );

    expect(screen.getByText("gpt-5.5-mini · Reasoning effort: low")).not.toBeNull();
    expect(container.textContent).not.toContain("Reasoning effort: high");
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
