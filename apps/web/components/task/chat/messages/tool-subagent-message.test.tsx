import { describe, it, expect, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { ToolSubagentMessage } from "@/components/task/chat/messages/tool-subagent-message";
import type { Message } from "@/lib/types/http";

// Guards the subagent card's expand/collapse behavior. Regression coverage for
// "silent" subagents (a result_text with no child tool calls) rendering
// permanently expanded: auto-expand must key on the running state alone, not on
// result_text, so completed cards collapse to their header + metadata row like
// subagents that stream child tool calls.

function subagentMessage(opts: {
  status: string;
  result_text?: string;
  description?: string;
}): Message {
  return {
    id: "m1",
    content: "Subagent",
    metadata: {
      status: opts.status,
      normalized: {
        kind: "subagent_task",
        subagent_task: {
          description: opts.description ?? "Summarize the repo",
          subagent_type: "general-purpose",
          status: opts.status,
          result_text: opts.result_text,
        },
      },
    },
  } as unknown as Message;
}

const noopRenderChild = () => null;
const SILENT_RESULT = "Found 42 files across 7 packages.";
const RESULT_TEXT = "subagent-result-text";

afterEach(cleanup);

describe("ToolSubagentMessage expansion", () => {
  it("collapses a completed subagent with child tool calls to its header", () => {
    const child = { id: "c1", content: "sleep 30", metadata: {} } as unknown as Message;
    render(
      <ToolSubagentMessage
        comment={subagentMessage({ status: "complete" })}
        childMessages={[child]}
        renderChild={() => <div data-testid="child-body">sleep 30</div>}
      />,
    );
    expect(screen.queryByTestId("child-body")).toBeNull();
  });

  it("collapses a completed silent subagent (result_text, no children)", () => {
    render(
      <ToolSubagentMessage
        comment={subagentMessage({
          status: "complete",
          result_text: SILENT_RESULT,
        })}
        childMessages={[]}
        renderChild={noopRenderChild}
      />,
    );
    // Completed and no longer running -> collapsed: result text stays hidden
    // until the user opens the card. The metadata row remains visible.
    expect(screen.queryByTestId(RESULT_TEXT)).toBeNull();
    expect(screen.getByTestId("subagent-type")).toBeTruthy();
  });

  it("reveals the silent subagent's result text when expanded", () => {
    render(
      <ToolSubagentMessage
        comment={subagentMessage({
          status: "complete",
          result_text: SILENT_RESULT,
        })}
        childMessages={[]}
        renderChild={noopRenderChild}
      />,
    );
    fireEvent.click(screen.getByRole("button"));
    expect(screen.getByTestId(RESULT_TEXT).textContent).toContain(SILENT_RESULT);
  });

  it("auto-expands while the subagent is running, then auto-collapses on completion", () => {
    const running = subagentMessage({ status: "running" });
    const { rerender } = render(
      <ToolSubagentMessage comment={running} childMessages={[]} renderChild={noopRenderChild} />,
    );
    // Running -> auto-expanded working indicator is shown.
    expect(screen.queryByText("Subagent working...")).not.toBeNull();

    rerender(
      <ToolSubagentMessage
        comment={subagentMessage({ status: "complete", result_text: "Done." })}
        childMessages={[]}
        renderChild={noopRenderChild}
      />,
    );
    // Completed without a manual override -> auto-collapsed.
    expect(screen.queryByTestId(RESULT_TEXT)).toBeNull();
  });

  it("keeps a manual collapse after the subagent completes", () => {
    const { rerender } = render(
      <ToolSubagentMessage
        comment={subagentMessage({ status: "running" })}
        childMessages={[]}
        renderChild={noopRenderChild}
      />,
    );
    // User collapses the running card.
    fireEvent.click(screen.getByRole("button"));
    expect(screen.queryByText("Subagent working...")).toBeNull();

    // Completion emits result_text; the manual collapse must survive (it used
    // to be wiped, forcing the card back open).
    rerender(
      <ToolSubagentMessage
        comment={subagentMessage({ status: "complete", result_text: "Final summary." })}
        childMessages={[]}
        renderChild={noopRenderChild}
      />,
    );
    expect(screen.queryByTestId(RESULT_TEXT)).toBeNull();
  });
});
