import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { filterVisibleMessages } from "./processed-message-filtering";

describe("workflow script visibility", () => {
  it("keeps workflow lifecycle scripts in the normal transcript", () => {
    const workflowScript: Message = {
      id: "workflow-script",
      session_id: toSessionId("s1"),
      task_id: toTaskId("t1"),
      author_type: "agent",
      content: "",
      type: "script_execution",
      created_at: "",
      metadata: {
        script_type: "workflow_step",
        workflow_step_name: "Review",
        status: "exited",
        exit_code: 0,
      },
    };

    expect(filterVisibleMessages([workflowScript], new Set<string>(), new Set<string>())).toEqual([
      workflowScript,
    ]);
  });
});
