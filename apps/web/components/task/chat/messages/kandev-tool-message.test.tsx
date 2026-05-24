import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { KandevToolMessage, hasKandevRenderer } from "./kandev-tool-message";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";

// kandevToolCall constructs the kandev Message shape produced by the
// orchestrator for an MCP tool_call. The shape mirrors live production data
// observed on a real session:
//   - `metadata.tool_name`    — NOT set
//   - `metadata.title`        — raw tool name (`mcp__kandev__list_*_kandev`)
//   - `comment.content`       — same raw tool name
//   - `normalized.generic.name` — the ACP adapter's *category* ("other"),
//                                 NOT the tool name
// The matcher must therefore look at title/content, not generic.name —
// matching on the wrong field is exactly the bug that shipped initially.
function kandevToolCall(opts: {
  toolName: string;
  input?: Record<string, unknown>;
  resultJson?: unknown;
  status?: "pending" | "running" | "complete" | "error";
}): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: opts.toolName,
    type: "tool_call",
    created_at: "2026-05-21T10:00:00Z",
    metadata: {
      tool_call_id: "tc-1",
      title: opts.toolName,
      status: opts.status ?? "complete",
      normalized: {
        kind: "generic",
        generic: {
          name: "other",
          input: opts.input,
          output:
            opts.resultJson !== undefined
              ? [{ type: "text", text: JSON.stringify(opts.resultJson) }]
              : undefined,
        },
      },
    },
  };
}

describe("hasKandevRenderer", () => {
  it("matches a registered kandev tool", () => {
    expect(hasKandevRenderer(kandevToolCall({ toolName: "mcp__kandev__list_tasks_kandev" }))).toBe(
      true,
    );
  });

  it("does not match unrelated tools", () => {
    expect(hasKandevRenderer(kandevToolCall({ toolName: "Edit" }))).toBe(false);
    expect(hasKandevRenderer(kandevToolCall({ toolName: "mcp__github__list_issues" }))).toBe(false);
  });

  it("does not match kandev tools with no registered renderer", () => {
    expect(
      hasKandevRenderer(kandevToolCall({ toolName: "mcp__kandev__never_heard_of_it_kandev" })),
    ).toBe(false);
  });
});

// State labels and titles reused across multiple tool-DTOs. Pulled to
// constants to satisfy the "no duplicate string" lint rule.
const COMPLETED = "COMPLETED";
const FIX_LOGIN_BUG = "Fix login bug";

// The expandable row only renders its body when expanded. For tests that
// need to inspect the body, set status to "running" which auto-expands the
// row — the same trigger we rely on at runtime to keep an in-progress tool
// call visible to the user.

describe("KandevToolMessage list renderers", () => {
  it("renders the workflow-steps list with structured fields, not raw JSON", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "running",
          toolName: "mcp__kandev__list_workflow_steps_kandev",
          input: { workflow_id: "f058586c-8a32-474b-ac02-eef47c938b41" },
          resultJson: {
            steps: [
              { id: "4aad62c5", name: "Backlog", position: 0, color: "bg-neutral-400" },
              {
                id: "step-2",
                name: "Doing",
                position: 1,
                color: "bg-blue-500",
                is_start_step: true,
              },
            ],
            total: 2,
          },
        })}
      />,
    );
    expect(html).toContain("Kandev: List Workflow Steps");
    expect(html).toContain("Backlog");
    expect(html).toContain("Doing");
    expect(html).toContain("2 steps");
    // The raw "\n" escape sequences from the broken JSON dump must not be
    // present any more.
    expect(html).not.toMatch(/\\n/);
    expect(html).not.toContain('"steps":');
  });

  it("renders list_tasks with task state badges", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "running",
          toolName: "list_tasks_kandev",
          input: { workflow_id: "wf-1" },
          resultJson: {
            tasks: [
              { id: "t1", title: FIX_LOGIN_BUG, state: COMPLETED },
              { id: "t2", title: "Add dashboard", state: "RUNNING" },
            ],
            total: 2,
          },
        })}
      />,
    );
    expect(html).toContain("Kandev: List Tasks");
    expect(html).toContain(FIX_LOGIN_BUG);
    expect(html).toContain("Add dashboard");
    expect(html).toContain(COMPLETED);
    expect(html).toContain("RUNNING");
  });

  it("renders list_workspaces with the count and item names", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "running",
          toolName: "mcp__kandev__list_workspaces_kandev",
          resultJson: {
            workspaces: [
              { id: "w1", name: "Main" },
              { id: "w2", name: "Side project" },
            ],
            total: 2,
          },
        })}
      />,
    );
    expect(html).toContain("Kandev: List Workspaces");
    expect(html).toContain("2 workspaces");
    expect(html).toContain("Main");
    expect(html).toContain("Side project");
  });
});

describe("KandevToolMessage task renderers", () => {
  it("shows the title argument inline for create_task", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "running",
          toolName: "kandev/create_task_kandev",
          input: { title: FIX_LOGIN_BUG, description: "..." },
          resultJson: { id: "new-id", title: FIX_LOGIN_BUG, state: "CREATED" },
        })}
      />,
    );
    expect(html).toContain("Kandev: Create Task");
    expect(html).toContain(FIX_LOGIN_BUG);
    expect(html).toContain("CREATED");
  });

  it("shows updated fields inline for update_task", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          toolName: "mcp__kandev__update_task_kandev",
          input: { task_id: "t1", state: COMPLETED },
          resultJson: { id: "t1", title: "Old title", state: COMPLETED },
        })}
      />,
    );
    expect(html).toContain("Kandev: Update Task");
    expect(html).toContain(`state=${COMPLETED}`);
  });
});

describe("KandevToolMessage document & question renderers", () => {
  it("renders task plan markdown content for get_task_plan", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "running",
          toolName: "mcp__kandev__get_task_plan_kandev",
          input: { task_id: "t1" },
          resultJson: {
            id: "p1",
            task_id: "t1",
            title: "Migration plan",
            content: "# Heading\n\nSome **bold** content.",
          },
        })}
      />,
    );
    expect(html).toContain("Kandev: Get Task Plan");
    expect(html).toContain("Migration plan");
    // Markdown is rendered through ReactMarkdown so the heading and bold
    // mark up into proper HTML tags rather than raw text.
    expect(html).toContain("<h1");
    expect(html).toContain("<strong>bold</strong>");
  });

  it("renders ask_user_question with the prompt and options", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "running",
          toolName: "mcp__kandev__ask_user_question_kandev",
          input: {
            questions: [
              {
                id: "q1",
                prompt: "Which database should we use?",
                options: [{ label: "Postgres" }, { label: "SQLite" }],
              },
            ],
          },
          resultJson: {
            pending_id: "pend-1",
            responses: { q1: { question_id: "q1", selected: "Postgres" } },
          },
        })}
      />,
    );
    expect(html).toContain("Kandev: Ask User Question");
    expect(html).toContain("Which database should we use?");
    expect(html).toContain("Postgres");
    expect(html).toContain("SQLite");
  });

  // Regression guard for the array-shape response branch in
  // matchAnswerForQuestion. Positional arrays come from older backends or
  // single-question calls; the renderer must match the answer by index.
  it("renders ask_user_question with positional array responses", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "running",
          toolName: "mcp__kandev__ask_user_question_kandev",
          input: {
            questions: [
              { id: "q1", prompt: "Pick one", options: [{ label: "A" }, { label: "B" }] },
            ],
          },
          resultJson: { responses: [{ question_id: "q1", selected: "A" }] },
        })}
      />,
    );
    expect(html).toContain("Pick one");
    // Selected option gets the "default" Badge variant; unselected stays "outline".
    expect(html).toMatch(/data-variant="default"[^>]*>A<\/span>/);
  });

  it("does not render anything when the renderer is missing", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({ toolName: "mcp__kandev__unknown_tool_kandev" })}
      />,
    );
    expect(html).toBe("");
  });
});
