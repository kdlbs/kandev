import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LspStatusItem } from "./lsp-status-item";

vi.mock("@/components/lsp/task-lsp-control", () => ({
  TaskLspControl: ({ taskId, placement }: { taskId: string; placement: string }) => (
    <div data-testid="mock-task-lsp-control" data-task-id={taskId} data-placement={placement} />
  ),
  TaskLspDisclosure: ({
    taskId,
    touch,
    focusLanguage,
  }: {
    taskId: string;
    touch: boolean;
    focusLanguage?: string | null;
  }) => (
    <div
      data-testid="mock-task-lsp-disclosure"
      data-task-id={taskId}
      data-touch={String(touch)}
      data-language={focusLanguage}
    />
  ),
}));

afterEach(cleanup);

describe("LspStatusItem", () => {
  it("renders the task-scoped compact controller in the application status bar", () => {
    render(<LspStatusItem taskId="task-1" presentation="bar" />);

    const control = screen.getByTestId("mock-task-lsp-control");
    expect(control.getAttribute("data-task-id")).toBe("task-1");
    expect(control.getAttribute("data-placement")).toBe("status-bar");
  });

  it("embeds the shared touch disclosure in the existing status drawer", () => {
    render(<LspStatusItem taskId="task-1" presentation="mobile-drawer" focusLanguage="kotlin" />);

    const disclosure = screen.getByTestId("mock-task-lsp-disclosure");
    expect(disclosure.getAttribute("data-task-id")).toBe("task-1");
    expect(disclosure.getAttribute("data-touch")).toBe("true");
    expect(disclosure.getAttribute("data-language")).toBe("kotlin");
  });
});
