import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import TaskLink from "./task-link";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("TaskLink", () => {
  it("builds a canonical task href from raw task and query context", () => {
    const searchParams = new URLSearchParams([["view", "focused"]]);
    render(
      <TaskLink taskId="task/123" layout="plan" sessionId="session 123" searchParams={searchParams}>
        Open task
      </TaskLink>,
    );

    expect(screen.getByRole("link", { name: "Open task" }).getAttribute("href")).toBe(
      "/t/task%2F123?view=focused&layout=plan&sessionId=session+123",
    );
  });

  it("forwards navigation callbacks and anchor refs through AppLink", () => {
    const onNavigated = vi.fn();
    const ref = { current: null as HTMLAnchorElement | null };
    render(
      <TaskLink ref={ref} taskId="task-123" onNavigated={onNavigated}>
        Open task
      </TaskLink>,
    );

    fireEvent.click(screen.getByRole("link", { name: "Open task" }));

    expect(ref.current).toBe(screen.getByRole("link", { name: "Open task" }));
    expect(onNavigated).toHaveBeenCalledOnce();
  });
});
