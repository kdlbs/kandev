import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { TaskLoadProgressState } from "./task-load-progress-state";

afterEach(cleanup);

function renderProgress(onRetry = vi.fn()) {
  render(
    <StateProvider initialState={{ workspaces: { items: [], activeId: "ws-1" } }}>
      <TaskLoadProgressState onRetry={onRetry} />
    </StateProvider>,
  );
  return onRetry;
}

describe("TaskLoadProgressState", () => {
  it("shows ordinary loading progress with a disabled Retry and overview access", () => {
    const onRetry = renderProgress();

    expect(screen.getByTestId("task-read-progress-state")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toContain("Loading task");
    const retry = screen.getByRole("button", { name: "Retry" });
    expect(retry.hasAttribute("disabled")).toBe(true);
    expect(retry.className).toContain("max-md:h-11");
    expect(screen.getByTestId("task-unavailable-overview-link").getAttribute("href")).toBe(
      "/?home=overview&workspaceId=ws-1",
    );
    expect(onRetry).not.toHaveBeenCalled();
  });

  it("announces progress once without a nested spinner status", () => {
    renderProgress();

    expect(screen.getAllByRole("status")).toHaveLength(1);
  });
});
