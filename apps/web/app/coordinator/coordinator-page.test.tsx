import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import { CoordinatorChat } from "./coordinator-chat";
import { CoordinatorPage, resolveCoordinatorSelection } from "./coordinator-page";

vi.mock("@/components/page-shell", () => ({
  PageShell: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
const calls = vi.hoisted(() => ({ workspace: vi.fn(), conversation: vi.fn() }));
vi.mock("@/hooks/domains/orchestration/use-coordinator-workspace", () => ({
  useCoordinatorWorkspace: calls.workspace,
}));
vi.mock("@/hooks/domains/orchestration/use-coordinator-conversation", () => ({
  useCoordinatorConversation: calls.conversation,
}));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it("withholds the page and its reads while orchestration is off", () => {
  render(
    <StateProvider>
      <CoordinatorPage workspaceId="ws" />
    </StateProvider>,
  );
  expect(screen.getByText("Workspace orchestration is disabled.")).not.toBeNull();
  expect(calls.workspace).not.toHaveBeenCalled();
  expect(calls.conversation).not.toHaveBeenCalled();
});
it("does not fall back to another account for a missing or ambiguous assignment", () => {
  expect(resolveCoordinatorSelection([{ id: "first" }], null)).toBe("first");
  expect(resolveCoordinatorSelection([{ id: "first" }], "foreign")).toBe("");
  expect(resolveCoordinatorSelection([{ id: "first" }, { id: "second" }], null)).toBe("");
  expect(resolveCoordinatorSelection([{ id: "first" }, { id: "second" }], "second")).toBe("second");
});
it("offers configuration without opening a conversation when its profile is unavailable", () => {
  const catalog = {
    workspace: { id: "ws" },
    assignments: [{ id: "chief", profile_id: "deleted", executor_preference: "{}" }],
    profiles: [],
    executors: [],
  } as unknown as CoordinatorWorkspace;
  render(
    <StateProvider>
      <CoordinatorChat catalog={catalog} selected="chief" />
    </StateProvider>,
  );
  expect(screen.getByRole("link", { name: "Configure orchestrator" }).getAttribute("href")).toBe(
    "/settings/workspaces/ws/orchestration/chief",
  );
  expect(calls.conversation).not.toHaveBeenCalled();
});

it("uses a still-valid remembered selection but honors explicit clearing and invalid links", () => {
  const assignments = [{ id: "one" }, { id: "two" }];
  expect(resolveCoordinatorSelection(assignments, null, "two")).toBe("two");
  expect(resolveCoordinatorSelection(assignments, null, "removed")).toBe("");
  expect(resolveCoordinatorSelection(assignments, "", "two")).toBe("");
  expect(resolveCoordinatorSelection(assignments, "foreign", "two")).toBe("");
});
