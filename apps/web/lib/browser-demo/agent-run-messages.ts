import { type Task } from "@/lib/types/http";
import { makeMessage } from "./scenario";

export function makeAgentRunMessages(task: Task, sessionId: string) {
  const turnId = `${sessionId}-implementation`;
  const worktree = `/demo/worktrees/${task.id}`;
  return [
    makeRunThinking(task, sessionId, worktree, turnId),
    makeRunRead(task, sessionId, worktree, turnId),
    makeRunEdit(task, sessionId, worktree, turnId),
    makeRunTest(task, sessionId, worktree, turnId),
    makeRunSummary(task, sessionId, worktree, turnId),
    makeRunMessage5(task, sessionId, worktree, turnId),
  ];
}

export function makeRunThinking(task: Task, sessionId: string, _worktree: string, turnId: string) {
  return makeMessage(
    `${sessionId}-thinking`,
    sessionId,
    task.id,
    "agent",
    "I will trace the user-facing path first, then make the smallest implementation change and verify it with a focused regression test.",
    {
      type: "thinking",
      turnId,
      metadata: {
        thinking:
          "The task touches an existing workflow, so I should understand its entry point and current coverage before editing.",
      },
    },
  );
}

export function makeRunRead(task: Task, sessionId: string, _worktree: string, turnId: string) {
  return makeMessage(
    `${sessionId}-search`,
    sessionId,
    task.id,
    "agent",
    "Searched the application for the relevant feature path",
    {
      type: "tool_search",
      turnId,
      metadata: {
        status: "complete",
        normalized: {
          code_search: {
            query: task.title,
            path: "src",
            output: {
              files: ["src/app.tsx", "src/pages/dashboard-page.tsx", "tests/dashboard.test.tsx"],
              file_count: 3,
            },
          },
        },
      },
    },
  );
}

export function makeRunEdit(task: Task, sessionId: string, worktree: string, turnId: string) {
  return makeMessage(
    `${sessionId}-read`,
    sessionId,
    task.id,
    "agent",
    "Read src/pages/dashboard-page.tsx and its focused tests",
    {
      type: "tool_read",
      turnId,
      metadata: {
        status: "complete",
        normalized: {
          read_file: {
            file_path: `${worktree}/src/pages/dashboard-page.tsx`,
            offset: 1,
            limit: 160,
            output: { line_count: 42, language: "tsx", truncated: false },
          },
        },
      },
    },
  );
}

export function makeRunTest(task: Task, sessionId: string, worktree: string, turnId: string) {
  return makeMessage(
    `${sessionId}-edit`,
    sessionId,
    task.id,
    "agent",
    "Updated the dashboard behavior and added regression coverage",
    {
      type: "tool_edit",
      turnId,
      metadata: {
        status: "complete",
        normalized: {
          modify_file: {
            file_path: `${worktree}/src/pages/dashboard-page.tsx`,
            mutations: [
              {
                type: "patch",
                old_content: "return <main><h1>Operations</h1></main>;",
                new_content: "return <main><h1>Operations</h1><ServiceHealthSummary /></main>;",
                diff: "@@ -8,1 +8,1 @@\n-return <main><h1>Operations</h1></main>;\n+return <main><h1>Operations</h1><ServiceHealthSummary /></main>;",
                start_line: 8,
                end_line: 8,
              },
            ],
          },
        },
      },
    },
  );
}

export function makeRunSummary(task: Task, sessionId: string, worktree: string, turnId: string) {
  return makeMessage(
    `${sessionId}-test`,
    sessionId,
    task.id,
    "agent",
    "pnpm test tests/dashboard.test.tsx --runInBand",
    {
      type: "tool_execute",
      turnId,
      metadata: {
        status: "complete",
        normalized: {
          shell_exec: {
            command: "pnpm test tests/dashboard.test.tsx --runInBand",
            work_dir: worktree,
            // i18n-exempt: demo fixture content retains its seeded English text
            description: "Run focused dashboard regression tests",
            output: { exit_code: 0, has_output: true, stdout_bytes: 986, stderr_bytes: 0 },
          },
        },
      },
    },
  );
}

export function makeRunMessage5(task: Task, sessionId: string, _worktree: string, turnId: string) {
  return makeMessage(
    `${sessionId}-summary`,
    sessionId,
    task.id,
    "agent",
    `Implemented **${task.title}** and added focused coverage for the updated behavior.\n\n\`\`\`text\nPASS tests/dashboard.test.tsx\nTests: 4 passed, 4 total\n\`\`\`\n\nThe task is ready for review.`,
    { turnId },
  );
}
