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
