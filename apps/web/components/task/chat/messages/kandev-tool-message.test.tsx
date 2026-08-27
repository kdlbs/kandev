import { describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { KandevToolMessage, hasKandevRenderer } from "./kandev-tool-message";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { KANDEV_RENDERERS } from "./kandev/registry";

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
  status?: "pending" | "running" | "in_progress" | "complete" | "error";
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
              ? {
                  _meta: null,
                  content: [{ type: "text", text: JSON.stringify(opts.resultJson) }],
                  structuredContent: null,
                }
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
    expect(
      hasKandevRenderer(kandevToolCall({ toolName: "mcp__kandev__show_walkthrough_kandev" })),
    ).toBe(true);
    expect(
      hasKandevRenderer(kandevToolCall({ toolName: "mcp__kandev__show_rich_output_kandev" })),
    ).toBe(true);
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

describe("KandevToolMessage host context", () => {
  it("passes session and repository-aware file opening to a renderer", () => {
    const original = KANDEV_RENDERERS.list_tasks;
    const renderer = vi.fn(() => <div>Context probe</div>);
    const onOpenFile = vi.fn();
    KANDEV_RENDERERS.list_tasks = renderer;
    try {
      renderToStaticMarkup(
        <KandevToolMessage
          comment={kandevToolCall({ toolName: "mcp__kandev__list_tasks_kandev" })}
          sessionId="session-1"
          onOpenFile={onOpenFile}
        />,
      );
    } finally {
      KANDEV_RENDERERS.list_tasks = original;
    }

    expect(renderer).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: "session-1", onOpenFile }),
    );
  });

  it("unwraps the real Codex ACP MCP arguments before rendering rich output", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          toolName: "mcp.kandev.show_rich_output_kandev",
          input: {
            raw_input: {
              server: "kandev",
              tool: "show_rich_output_kandev",
              arguments: {
                version: 1,
                title: "Nested presentation",
                blocks: [{ type: "metrics", items: [{ label: "Passed", value: "38" }] }],
              },
            },
          },
        })}
      />,
    );

    expect(html).toContain("Nested presentation");
    expect(html).not.toContain("This presentation is unavailable.");
  });

  it("renders a CSV chart from the persisted tool-result snapshot", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          toolName: "mcp__kandev__show_rich_output_kandev",
          input: {
            version: 1,
            title: "CSV presentation",
            blocks: [
              {
                type: "chart",
                chart_type: "bar",
                title: "Requests by route",
                summary: "Request volume from the workspace CSV.",
                csv: {
                  path: "reports/routes.csv",
                  x_column: "route",
                  series: [{ column: "requests" }],
                },
              },
            ],
          },
          resultJson: {
            version: 1,
            resolved_charts: [
              {
                block_index: 0,
                labels: ["/api", "/health"],
                series: [{ label: "requests", values: [2400, 800] }],
              },
            ],
          },
        })}
      />,
    );

    expect(html).toContain("CSV presentation");
    expect(html).toContain('data-testid="rich-output-chart-bar"');
    expect(html).not.toContain("This presentation is unavailable.");
  });
});

// State labels and titles reused across multiple tool-DTOs. Pulled to
// constants to satisfy the "no duplicate string" lint rule.
const COMPLETED = "COMPLETED";
const FIX_LOGIN_BUG = "Fix login bug";
const CHANGE_TOUR = "Change tour";
const PERMISSION_ACTION_ROW = 'data-testid="permission-action-row"';

// The expandable row only renders its body when expanded. For tests that
// need to inspect the body, set status to "running" which auto-expands the
// row — the same trigger we rely on at runtime to keep an in-progress tool
// call visible to the user.

describe("KandevToolMessage list renderers", () => {
  it("normalizes ACP in_progress status to running", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "in_progress",
          toolName: "mcp__kandev__list_workspaces_kandev",
          resultJson: { workspaces: [{ id: "w1", name: "Live workspace" }], total: 1 },
        })}
      />,
    );

    expect(html).toContain("Live workspace");
  });

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
    const root = document.createElement("div");
    root.innerHTML = html;
    expect(root.querySelector('[data-variant="default"]')?.textContent).toBe("A");
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

describe("KandevToolMessage walkthrough renderer", () => {
  const step = {
    title: "Review handler",
    file: "apps/backend/internal/mcp/handlers.go",
    line: 42,
    line_end: 45,
    text: "This handler persists the walkthrough.",
  };

  it("renders show_walkthrough with a walkthrough icon and title", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "running",
          toolName: "mcp__kandev__show_walkthrough_kandev",
          input: {
            task_id: "task-1",
            title: CHANGE_TOUR,
            steps: [step],
          },
        })}
      />,
    );

    expect(html).toContain("Walkthrough: Change tour");
    expect(html).toContain("tabler-icon-route");
    expect(html).toContain("1 step");
    expect(html).toContain("apps/backend/internal/mcp/handlers.go:42-45");
    expect(html).toContain("Review handler");
  });

  it("renders show_walkthrough from an MCP text result when args are absent", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "running",
          toolName: "show_walkthrough_kandev",
          resultJson: {
            result: `Walkthrough saved:\n${JSON.stringify({
              title: CHANGE_TOUR,
              steps: [step],
            })}`,
          },
        })}
      />,
    );

    expect(html).toContain("Walkthrough: Change tour");
    expect(html).toContain("1 step");
    expect(html).toContain("Review handler");
  });

  it("includes repo anchors in walkthrough step locations", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "running",
          toolName: "show_walkthrough_kandev",
          input: {
            title: CHANGE_TOUR,
            steps: [{ ...step, repo: "backend" }],
          },
        })}
      />,
    );

    expect(html).toContain("backend:apps/backend/internal/mcp/handlers.go:42-45");
  });
});

// Unset status is treated as pending by parsePermission.
function pendingPermissionMessage(toolCallId: string): Message {
  return {
    id: "perm-1",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "permission",
    type: "permission_request",
    created_at: "2026-05-21T10:00:01Z",
    metadata: {
      request_id: "req-1",
      pending_id: "pend-1",
      tool_call_id: toolCallId,
      action_type: "mcp_tool",
      action_details: {},
      options: [
        { option_id: "allow", name: "Allow", kind: "allow_once" },
        { option_id: "reject", name: "Reject", kind: "reject_once" },
      ],
    },
  };
}

describe("KandevToolMessage permission UI", () => {
  it("renders Approve / Deny buttons when permissionMessage is pending", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "pending",
          toolName: "mcp__kandev__list_workspaces_kandev",
        })}
        permissionMessage={pendingPermissionMessage("tc-1")}
      />,
    );
    expect(html).toContain(PERMISSION_ACTION_ROW);
    expect(html).toContain('data-testid="permission-approve"');
    expect(html).toContain('data-testid="permission-reject"');
  });

  it("does not render permission row when permissionMessage is absent", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "complete",
          toolName: "mcp__kandev__list_workspaces_kandev",
        })}
      />,
    );
    expect(html).not.toContain(PERMISSION_ACTION_ROW);
    expect(html).not.toContain('data-testid="permission-approve"');
  });

  it("does not render active actions for a legacy permission without request identity", () => {
    const permissionMessage = pendingPermissionMessage("tc-1");
    delete (permissionMessage.metadata as Record<string, unknown>).request_id;
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "pending",
          toolName: "mcp__kandev__list_workspaces_kandev",
        })}
        permissionMessage={permissionMessage}
      />,
    );
    expect(html).not.toContain(PERMISSION_ACTION_ROW);
    expect(html).not.toContain('data-testid="permission-approve"');
    expect(html).not.toContain('data-testid="permission-reject"');
  });

  it("renders an amber pending clock icon while waiting for approval", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "pending",
          toolName: "mcp__kandev__list_workspaces_kandev",
        })}
        permissionMessage={pendingPermissionMessage("tc-1")}
      />,
    );
    expect(html).toContain("tabler-icon-clock");
    expect(html).toContain("text-amber");
  });

  it("hides the result-count summary while pending (no misleading '0 workspaces')", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "pending",
          toolName: "mcp__kandev__list_workspaces_kandev",
        })}
        permissionMessage={pendingPermissionMessage("tc-1")}
      />,
    );
    expect(html).not.toContain("0 workspaces");
  });
});

describe("KandevToolMessage resolved-permission overlay", () => {
  function resolvedPermissionMessage(toolCallId: string, status: "approved" | "rejected"): Message {
    const msg = pendingPermissionMessage(toolCallId);
    return {
      ...msg,
      metadata: { ...(msg.metadata as object), status },
    } as Message;
  }

  it("drops the amber pending clock once the permission resolves to approved", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "pending",
          toolName: "mcp__kandev__list_workspaces_kandev",
        })}
        permissionMessage={resolvedPermissionMessage("tc-1", "approved")}
      />,
    );
    expect(html).not.toContain("tabler-icon-clock");
    expect(html).not.toContain(PERMISSION_ACTION_ROW);
  });

  it("does NOT mark the tool complete just because the permission was approved", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "pending",
          toolName: "mcp__kandev__list_workspaces_kandev",
        })}
        permissionMessage={resolvedPermissionMessage("tc-1", "approved")}
      />,
    );
    expect(html).not.toContain("tabler-icon-check");
  });

  it("still renders an error when the tool_call errors after an approved permission", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "error",
          toolName: "mcp__kandev__list_workspaces_kandev",
        })}
        permissionMessage={resolvedPermissionMessage("tc-1", "approved")}
      />,
    );
    expect(html).toContain("tabler-icon-x");
  });

  it("shows the result summary once permission resolves even if meta.status still pending", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "pending",
          toolName: "mcp__kandev__list_workspaces_kandev",
          resultJson: { workspaces: [{ id: "w1", name: "Main" }], total: 1 },
        })}
        permissionMessage={resolvedPermissionMessage("tc-1", "approved")}
      />,
    );
    expect(html).toContain("1 workspace");
  });

  it("renders a red X overlay when the permission is rejected", () => {
    const html = renderToStaticMarkup(
      <KandevToolMessage
        comment={kandevToolCall({
          status: "pending",
          toolName: "mcp__kandev__list_workspaces_kandev",
        })}
        permissionMessage={resolvedPermissionMessage("tc-1", "rejected")}
      />,
    );
    expect(html).toContain("tabler-icon-x");
    expect(html).toContain("text-red-500");
    expect(html).not.toContain("tabler-icon-clock");
  });
});
