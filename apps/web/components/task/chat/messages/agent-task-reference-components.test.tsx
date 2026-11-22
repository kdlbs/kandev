import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { taskId, sessionId, type Message } from "@/lib/types/http";
import { AgentMessageContent } from "./agent-message-content";
import { MemoizedMarkdown } from "@/components/shared/memoized-markdown";
import { agentTaskReferenceComponents } from "./agent-task-reference-components";
import { AgentPlanMessage } from "./agent-plan-message";

const copy = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
vi.mock("@/hooks/use-copy-to-clipboard", () => ({
  useCopyToClipboard: () => ({ copied: false, copy }),
}));

const SOURCE = "Tracked in [Fix \\*retry\\*](/t/exact-task).";
afterEach(cleanup);

function renderReply(content = SOURCE, raw = false, plan = false) {
  const comment: Message = {
    id: "reference-message",
    task_id: taskId("source-task"),
    session_id: sessionId("source-session"),
    author_type: "agent",
    content,
    type: "message",
    created_at: "2026-11-16T00:00:00Z",
  };
  return render(
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          {plan ? (
            <AgentPlanMessage comment={comment} />
          ) : (
            <AgentMessageContent
              comment={comment}
              showRaw={raw}
              onToggleRaw={() => {}}
              isTurnActive={false}
            />
          )}
        </TooltipProvider>
      </ToastProvider>
    </StateProvider>,
  );
}

// @covers AC-UI-AGENT-TASK-REFERENCE-001.1, AC-UI-AGENT-TASK-REFERENCE-001.5
// @covers AC-UI-AGENT-TASK-REFERENCE-001.6
describe("agent task reference composition", () => {
  it.each([false, true])("renders an authored task title in reply/plan %s", (plan) => {
    renderReply(plan ? `# Plan\n${SOURCE}` : SOURCE, false, plan);
    const link = screen.getByRole("link", { name: "Fix *retry*" });
    expect(link.getAttribute("href")).toBe("/t/exact-task");
    expect(link.querySelector("svg")).not.toBeNull();
    expect(link.textContent).toBe("Fix *retry*");
  });
  it("leaves raw source unchanged", () => {
    const { container } = renderReply(SOURCE, true);
    expect(container.querySelector("pre")?.textContent).toBe(SOURCE);
    expect(screen.queryByRole("link")).toBeNull();
  });
  it("does not decorate code, rich labels, unrelated links, or incomplete syntax", () => {
    renderReply(
      "`[code](/t/id)` [**rich**](/t/id) [external](https://example.com) [unfinished](/t/id",
    );
    expect(screen.queryByTestId("task-mention-chip")).toBeNull();
    expect(screen.getByRole("link", { name: "rich" }).querySelector("strong")).not.toBeNull();
    expect(screen.getByRole("link", { name: "external" }).getAttribute("href")).toBe(
      "https://example.com",
    );
  });
});

// @covers AC-UI-AGENT-TASK-REFERENCE-001.6
it("copies original authored Markdown instead of rendered chip text", () => {
  renderReply();
  fireEvent.click(screen.getByRole("button", { name: "Copy message to clipboard" }));
  expect(copy).toHaveBeenCalledWith(SOURCE);
});

// @covers AC-UI-AGENT-TASK-REFERENCE-001.6
it("keeps file links functional and recognizes a completed streamed reference", () => {
  const openFile = vi.fn();
  function Markdown({ content }: { content: string }) {
    return (
      <StateProvider>
        <TooltipProvider>
          <MemoizedMarkdown
            content={content}
            components={agentTaskReferenceComponents}
            animateText
            worktreePath="/workspace"
            onOpenFile={openFile}
          />
        </TooltipProvider>
      </StateProvider>
    );
  }
  const view = render(<Markdown content="See [Task](/t/id" />);
  expect(screen.queryByTestId("task-mention-chip")).toBeNull();
  view.rerender(<Markdown content="See [Task](/t/id) and [File](src/file.ts)." />);
  expect(screen.getByRole("link", { name: "Task" }).querySelector("svg")).not.toBeNull();
  fireEvent.click(screen.getByRole("link", { name: "File" }));
  expect(openFile).toHaveBeenCalledWith("src/file.ts");
  view.rerender(<Markdown content="See [Task](/t/id) and [File](src/file.ts). More text." />);
  expect(screen.getByRole("link", { name: "Task" }).getAttribute("href")).toBe("/t/id");
});
