import { describe, expect, it } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type MessageType,
} from "@/lib/types/http";
import { buildGroupedRenderItems } from "./use-processed-messages";

function message(
  id: string,
  type: MessageType,
  metadata: Record<string, unknown>,
  content = "",
): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content,
    type,
    metadata,
    created_at: "",
    turn_id: "turn-1",
  };
}

function toolActivity(id: string): Message {
  return message(
    id,
    "tool_execute",
    {
      status: "complete",
      normalized: { shell_exec: { command: "gh pr checks", output: { exit_code: 0 } } },
    },
    "gh pr checks",
  );
}

function richOutputCall(version: number): Message {
  const toolName = "kandev: show_rich_output_kandev";
  return message(
    "rich",
    "tool_call",
    {
      title: toolName,
      status: "complete",
      normalized: {
        kind: "generic",
        generic: {
          name: "other",
          input: {
            kind: "other",
            raw_input: {
              providerIdentifier: "kandev",
              toolName: "show_rich_output_kandev",
              args:
                version === 1
                  ? {
                      version: 1,
                      title: "Build health",
                      blocks: [{ type: "metrics", items: [{ label: "Passed", value: "38" }] }],
                    }
                  : {
                      version: 2,
                      catalog_version: 1,
                      title: "Build health",
                      spec: {
                        root: "counts",
                        elements: {
                          counts: {
                            type: "Metrics",
                            props: { items: [{ label: "Passed", value: "38" }] },
                            children: [],
                          },
                        },
                      },
                    },
            },
          },
          output: { success: true },
        },
      },
    },
    toolName,
  );
}

function copilotRichOutputCall(): Message {
  const title = "kandev-show_rich_output_kandev";
  return message(
    "copilot-rich-output",
    "tool_call",
    {
      title,
      status: "complete",
      normalized: {
        kind: "generic",
        generic: {
          name: "other",
          input: {
            kind: "other",
            raw_input: {
              version: 2,
              catalog_version: 1,
              title: "Synthetic report",
              spec: {
                root: "report",
                elements: {
                  report: {
                    type: "Callout",
                    props: {
                      kind: "info",
                      title: "Synthetic report",
                      text: "Synthetic check completed.",
                    },
                    children: [],
                  },
                },
              },
            },
          },
          output: { content: "Rich output accepted." },
        },
      },
    },
    title,
  );
}

describe("buildGroupedRenderItems rich-output hoisting", () => {
  it.each([1, 2])("keeps version %i standalone and groups ordinary activity", (version) => {
    const items = buildGroupedRenderItems(
      [
        toolActivity("t1"),
        toolActivity("t2"),
        richOutputCall(version),
        toolActivity("t3"),
        toolActivity("t4"),
      ],
      "s1",
      { canAnchorPrepareProgress: false },
    );

    expect(items.map((item) => item.type)).toEqual(["turn_group", "message", "turn_group"]);
    expect(items[1]).toMatchObject({ type: "message", message: { id: "rich" } });
  });

  it("keeps Copilot output between ordinary activity groups", () => {
    const items = buildGroupedRenderItems(
      [
        toolActivity("before-1"),
        toolActivity("before-2"),
        copilotRichOutputCall(),
        toolActivity("after-1"),
        toolActivity("after-2"),
      ],
      "s1",
      { canAnchorPrepareProgress: false },
    );

    expect(items.map((item) => item.type)).toEqual(["turn_group", "message", "turn_group"]);
    expect(items[0]).toMatchObject({
      type: "turn_group",
      messages: [{ id: "before-1" }, { id: "before-2" }],
    });
    expect(items[1]).toMatchObject({ type: "message", message: { id: "copilot-rich-output" } });
    expect(items[2]).toMatchObject({
      type: "turn_group",
      messages: [{ id: "after-1" }, { id: "after-2" }],
    });
  });
});
