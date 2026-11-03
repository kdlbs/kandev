import { describe, expect, it } from "vitest";
import { buildTaskMentionsContext } from "@/hooks/use-message-handler";
import type { AppState } from "@/lib/state/store";
import { taskMentionReferences } from "./task-mention-references";

const state = {
  kanban: { steps: [{ id: "step", title: "In progress" }] },
  kanbanMulti: { snapshots: {} },
} as unknown as AppState;

function context(tasks: Array<{ title: string; taskId: string }>) {
  return buildTaskMentionsContext(
    tasks.map((task) => ({ ...task, workflowId: "workflow", workflowStepId: "step" })),
    state,
  );
}

// @covers AC-UI-CHAT-TASK-MENTION-001.1
// @covers AC-UI-CHAT-TASK-MENTION-001.2
// @covers AC-UI-CHAT-TASK-MENTION-001.3
describe("taskMentionReferences", () => {
  it("recovers recorded IDs and submitted Unicode titles from the real producer", () => {
    const tasks = [
      { title: "Investigate terminal retry", taskId: "task-1" },
      { title: "修复 (id: fake), step: ready", taskId: "task-2" },
    ];
    expect(taskMentionReferences(`check @Investigate terminal retry${context(tasks)}`)).toEqual(
      tasks,
    );
  });

  it("handles optional state and CRLF framing", () => {
    const source = context([{ title: "Retry", taskId: "task-1" }])
      .replace("step: In progress)", "step: In progress, state: RUNNING)")
      .replaceAll("\n", "\r\n");
    expect(taskMentionReferences(source)).toEqual([{ title: "Retry", taskId: "task-1" }]);
  });

  it("does not mistake record-shaped step text for the referenced task identity", () => {
    const source = buildTaskMentionsContext(
      [{ title: "Retry", taskId: "real-task", workflowId: "workflow", workflowStepId: "step" }],
      {
        ...state,
        kanban: {
          ...state.kanban,
          steps: [
            {
              ...state.kanban.steps[0],
              title: "Review (id: other-task, workflow_id: other, step: Done)",
            },
          ],
        },
      },
    );
    expect(taskMentionReferences(source)).toEqual([]);
  });

  it("does not guess between complete record delimiters embedded in a task title", () => {
    const source = context([
      { title: "Retry (id: other-task, workflow_id: other, step: Done)", taskId: "real-task" },
    ]);
    expect(taskMentionReferences(source)).toEqual([]);
  });

  it.each(["bad/id", "x".repeat(257)])(
    "does not count an invalid embedded ID as another task reference",
    (embeddedId) => {
      const task = {
        title: `Retry (id: ${embeddedId}, workflow_id: other, step: Done)`,
        taskId: "real-task",
      };
      expect(taskMentionReferences(context([task]))).toEqual([task]);
    },
  );

  it("deduplicates identical pairs and removes conflicting titles across blocks", () => {
    const first = context([
      { title: "Same", taskId: "one" },
      { title: "Unique", taskId: "three" },
      { title: "Unique", taskId: "three" },
    ]);
    expect(taskMentionReferences(first + context([{ title: "Same", taskId: "two" }]))).toEqual([
      { title: "Unique", taskId: "three" },
    ]);
  });

  it.each([
    (s: string) => s.replace("</kandev-system>", ""),
    (s: string) => s.replace("<kandev-system>", ""),
    (s: string) => s.replace("REFERENCED TASKS:", "OTHER CONTEXT:"),
    (s: string) => s.replace("(id: task-1,", "(id: ,"),
    (s: string) => s.replace("id: task-1", "id: ../task"),
    (s: string) => s.replace("id: task-1", "id: https://example.com"),
    (s: string) => s.replace("id: task-1", "id: task%2Fone"),
    (s: string) => s.replace("step: In progress)", "step: In progress"),
  ])("ignores incomplete or invalid reference context", (mutate) => {
    expect(taskMentionReferences(mutate(context([{ title: "Retry", taskId: "task-1" }])))).toEqual(
      [],
    );
  });

  it("keeps independently valid records when a neighboring record is malformed", () => {
    const source = context([
      { title: "Bad", taskId: "bad/id" },
      { title: "Good", taskId: "task-2" },
    ]);
    expect(taskMentionReferences(source)).toEqual([{ title: "Good", taskId: "task-2" }]);
  });
});

describe("task mention reference limits", () => {
  it("limits UTF-8 bytes while retaining references below the byte budget", () => {
    const task = { title: "Retry", taskId: "task-1" };
    const source = context([task]);
    expect(taskMentionReferences("中".repeat(80_000) + source)).toEqual([task]);
    expect(taskMentionReferences("中".repeat(88_000) + source)).toEqual([]);
  });
  it("fails closed on oversized sources or too many records", () => {
    const source = context([{ title: "Retry", taskId: "task-1" }]);
    expect(taskMentionReferences("x".repeat(262_144) + source)).toEqual([]);
    expect(
      taskMentionReferences(
        context(
          Array.from({ length: 201 }, (_, i) => ({
            title: `Task ${i}`,
            taskId: `task-${i}`,
          })),
        ),
      ),
    ).toEqual([]);
  });

  it("rejects oversized titles and IDs without dropping valid neighbors", () => {
    expect(
      taskMentionReferences(
        context([
          { title: "x".repeat(513), taskId: "one" },
          { title: "Retry", taskId: "x".repeat(257) },
          { title: "Good", taskId: "good" },
        ]),
      ),
    ).toEqual([{ title: "Good", taskId: "good" }]);
  });
});

describe("task mention parser scan limits", () => {
  it("rejects an overlong record before parsing while retaining a valid sibling", () => {
    const valid = context([{ title: "Retry", taskId: "task-1" }]);
    const oversized = context([{ title: "Oversized", taskId: "task-2" }]).replace(
      "step: In progress)",
      `step: ${"x".repeat(5_000)})`,
    );
    expect(taskMentionReferences(oversized + valid)).toEqual([
      { title: "Retry", taskId: "task-1" },
    ]);
  });

  it("handles a long comma-free record-shaped line within the source budget", () => {
    const valid = context([{ title: "Retry", taskId: "task-1" }]);
    const malformed = valid.replace(/- Retry.*\n/, `- Retry${" (id: a".repeat(30_000)}\n`);
    expect(taskMentionReferences(malformed + valid)).toEqual([
      { title: "Retry", taskId: "task-1" },
    ]);
  });

  it("stops after unmatched system openers without dropping earlier references", () => {
    const valid = context([{ title: "Retry", taskId: "task-1" }]);
    expect(taskMentionReferences(valid + "<kandev-system>".repeat(17_000))).toEqual([
      { title: "Retry", taskId: "task-1" },
    ]);
  });
});
