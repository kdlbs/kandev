import { render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const replaceMock = vi.hoisted(() => vi.fn());

vi.mock("@/components/kanban-with-preview", () => ({
  KanbanWithPreview: () => null,
}));
vi.mock("@/components/onboarding-dialog", () => ({
  OnboardingDialog: () => null,
}));
vi.mock("@/hooks/use-task-listing-view", () => ({
  useTaskListingView: () => ({ preferredView: "list" }),
}));
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ replace: replaceMock }),
}));

import { PageClient } from "./page-client";

afterEach(() => {
  replaceMock.mockReset();
});

describe("PageClient", () => {
  it("restores List in the resolved workspace", async () => {
    render(<PageClient workspaceId="workspace-1" />);

    await waitFor(() => {
      expect(replaceMock).toHaveBeenCalledWith("/tasks?workspace=workspace-1");
    });
  });

  it("does not restore List while opening a task", async () => {
    render(<PageClient workspaceId="workspace-1" initialTaskId="task-1" />);

    await waitFor(() => {
      expect(replaceMock).not.toHaveBeenCalled();
    });
  });

  it("does not restore List while opening a session", async () => {
    render(<PageClient workspaceId="workspace-1" initialSessionId="session-1" />);

    await waitFor(() => {
      expect(replaceMock).not.toHaveBeenCalled();
    });
  });
});
