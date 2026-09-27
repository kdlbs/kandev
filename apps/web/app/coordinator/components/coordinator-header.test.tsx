import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { CoordinatorHeader } from "./coordinator-header";

afterEach(cleanup);

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "co-1",
    workspace_id: "ws-1",
    name: "Planner",
    agent_profile_id: "a-1",
    executor_profile_id: "e-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  };
}

describe("CoordinatorHeader", () => {
  it("shows the coordinator's name as plain text with a single coordinator", () => {
    render(
      <CoordinatorHeader
        coordinator={coordinator()}
        coordinators={[coordinator()]}
        workspaceId="ws-1"
        view="needs-you"
        canManage={false}
      />,
    );
    expect(screen.getByText("Planner")).not.toBeNull();
    expect(screen.queryByTestId("coordinator-selector")).toBeNull();
  });

  it("shows a selector with several coordinators", () => {
    const other = coordinator({ id: "co-2", name: "Reviewer" });
    render(
      <CoordinatorHeader
        coordinator={coordinator()}
        coordinators={[coordinator(), other]}
        workspaceId="ws-1"
        view="needs-you"
        canManage={false}
      />,
    );
    expect(screen.getByTestId("coordinator-selector")).not.toBeNull();
  });

  it("shows Configure only when canManage is true", () => {
    const { rerender } = render(
      <CoordinatorHeader
        coordinator={coordinator()}
        coordinators={[coordinator()]}
        workspaceId="ws-1"
        view="needs-you"
        canManage={false}
      />,
    );
    expect(screen.queryByText("Configure")).toBeNull();

    rerender(
      <CoordinatorHeader
        coordinator={coordinator()}
        coordinators={[coordinator()]}
        workspaceId="ws-1"
        view="needs-you"
        canManage
      />,
    );
    expect(screen.getByRole("link", { name: "Configure" }).getAttribute("href")).toBe(
      "/settings/workspaces/ws-1/coordinators/co-1",
    );
  });
});
