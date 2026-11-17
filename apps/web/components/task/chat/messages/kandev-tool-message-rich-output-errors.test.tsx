import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { richOutputExamples } from "@/lib/rich-output/examples";
import { KandevToolMessage } from "./kandev-tool-message";
import { kandevToolCall } from "./kandev-tool-message.test-helpers";
import { KandevPermissionUIProvider, KandevRow } from "./kandev/shared";
import { IconChartBar } from "@tabler/icons-react";

const ERROR_TRIGGER = "Show visual output error";
const ERROR_TITLE = "Visual output error";
const RETAINED_DIAGNOSTIC = 'StatusList item "build" has unsupported status "success".';
const PRIVATE_ARGUMENT = "private submitted argument";

afterEach(() => cleanup());

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-001.13, AC-AGENTS-AGENT-RICH-OUTPUT-001.21
describe("KandevToolMessage rich output errors", () => {
  it("opens the retained failed-call error without changing a successful retry", () => {
    render(
      <TooltipProvider>
        <>
          <KandevToolMessage
            comment={kandevToolCall({
              toolName: "mcp__kandev__show_rich_output_kandev",
              input: { kind: "other", raw_input: { secret: PRIVATE_ARGUMENT } },
              resultJson: { message: RETAINED_DIAGNOSTIC },
              status: "error",
            })}
          />
          <KandevToolMessage
            comment={kandevToolCall({
              toolName: "mcp__kandev__show_rich_output_kandev",
              input: { kind: "other", raw_input: richOutputExamples[2] },
              output: { content: "Rich output accepted." },
              status: "complete",
            })}
          />
        </>
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: ERROR_TRIGGER }));

    expect(screen.getByText(ERROR_TITLE)).toBeTruthy();
    expect(screen.getByText(RETAINED_DIAGNOSTIC)).toBeTruthy();
    expect(screen.getByTestId("rich-output")).toBeTruthy();
    expect(document.body.textContent).not.toContain(PRIVATE_ARGUMENT);
  });

  it("shows a localized fallback when the retained result has no supported error text", () => {
    render(
      <TooltipProvider>
        <KandevToolMessage
          comment={kandevToolCall({
            toolName: "mcp__kandev__show_rich_output_kandev",
            input: { kind: "other", raw_input: { secret: PRIVATE_ARGUMENT } },
            resultJson: { details: PRIVATE_ARGUMENT },
            status: "error",
          })}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: ERROR_TRIGGER }));

    expect(screen.getByText("No error details are available.")).toBeTruthy();
    expect(document.body.textContent).not.toContain(PRIVATE_ARGUMENT);
  });

  it.each(["pending", "running"] as const)("does not disclose %s rich output calls", (status) => {
    render(
      <TooltipProvider>
        <KandevToolMessage
          comment={kandevToolCall({
            toolName: "mcp__kandev__show_rich_output_kandev",
            resultJson: { message: RETAINED_DIAGNOSTIC },
            status,
          })}
        />
      </TooltipProvider>,
    );

    expect(screen.queryByRole("button", { name: ERROR_TRIGGER })).toBeNull();
  });

  it("keeps permission icons ahead of an optional row status control", () => {
    const row = (permission: "pending" | "rejected") => (
      <KandevPermissionUIProvider value={permission}>
        <KandevRow
          Icon={IconChartBar}
          title="Kandev: visual output"
          status="error"
          statusControl={<button type="button">Custom status</button>}
          hasExpandableContent={false}
        />
      </KandevPermissionUIProvider>
    );

    const { rerender } = render(row("pending"));
    expect(screen.queryByRole("button", { name: "Custom status" })).toBeNull();
    rerender(row("rejected"));
    expect(screen.queryByRole("button", { name: "Custom status" })).toBeNull();
  });
});
