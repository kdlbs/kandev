import { render, screen, fireEvent, within, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MCPSessionSelectionBody } from "../../mcp-session-selector";
import type { MCPServerDefinition } from "@/lib/types/http-mcp";
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      workspaces: { activeId: "ws" },
      agentProfiles: { items: [] },
      repositories: { itemsByWorkspaceId: {} },
      kanban: { tasks: [] },
      workflows: { items: [] },
      taskSessions: { items: {} },
    }),
}));
afterEach(cleanup);
const definition: MCPServerDefinition = {
  id: "docs",
  workspace_id: "ws",
  runtime_name: "docs",
  normalized_runtime_name: "docs",
  display_name: "Docs",
  enabled: true,
  execution_mode: "remote",
  transport: "sse",
  configuration: { url: "https://example.test" },
  source: "custom",
  revision: 1,
  created_at: "now",
  updated_at: "now",
};
const props = {
  definitions: [definition],
  selectedIds: ["docs"],
  onSelectedIdsChange: vi.fn(),
  query: "",
  onQueryChange: vi.fn(),
  inherited: [
    {
      definition,
      origins: [
        { scope: "repository" as const, workspace_id: "ws", owner_id: "repo" },
        { scope: "profile" as const, workspace_id: "ws", owner_id: "profile" },
      ],
    },
  ],
};
it("excludes by definition once, retains origins and requires a committed result to show saved", () => {
  const change = vi.fn();
  const { rerender } = render(
    <MCPSessionSelectionBody {...props} excludedIds={[]} onExcludedIdsChange={change} />,
  );
  const row = screen.getByTestId("mcp-inherited-server-docs");
  expect(within(row).getByText(/Profile/)).toBeTruthy();
  fireEvent.click(within(row).getByRole("button", { name: "Exclude for this session" }));
  expect(change).toHaveBeenCalledExactlyOnceWith(["docs"]);
  expect(within(row).queryByRole("button", { name: "Restore inheritance" })).toBeNull();
  rerender(
    <MCPSessionSelectionBody {...props} excludedIds={["docs"]} onExcludedIdsChange={change} />,
  );
  fireEvent.click(within(row).getByRole("button", { name: "Restore inheritance" }));
  expect(change).toHaveBeenLastCalledWith([]);
  expect(props.onSelectedIdsChange).not.toHaveBeenCalled();
});
it("restores a dormant disabled exclusion without adding or enabling its definition", () => {
  const change = vi.fn();
  render(
    <MCPSessionSelectionBody
      {...props}
      inherited={[]}
      definitions={[{ ...definition, enabled: false }]}
      excludedIds={["docs"]}
      onExcludedIdsChange={change}
    />,
  );
  expect(screen.getByText("No longer inherited")).toBeTruthy();
  expect(screen.queryByRole("checkbox", { name: "Docs" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Restore inheritance" }));
  expect(change).toHaveBeenCalledWith([]);
  expect(props.onSelectedIdsChange).not.toHaveBeenCalled();
});
it("keeps creation additive and disables session actions while saving", () => {
  const { rerender } = render(<MCPSessionSelectionBody {...props} />);
  expect(screen.queryByRole("button", { name: "Exclude for this session" })).toBeNull();
  rerender(
    <MCPSessionSelectionBody {...props} excludedIds={[]} onExcludedIdsChange={vi.fn()} disabled />,
  );
  expect(
    screen.getByRole("button", { name: "Exclude for this session" }).hasAttribute("disabled"),
  ).toBe(true);
});

it("keeps the committed comparison visible when an addition draft was rejected", () => {
  render(
    <MCPSessionSelectionBody
      {...props}
      selectedIds={[]}
      savedSelectedIds={["docs"]}
      state={{
        desired_revision: 2,
        applied_revision: 1,
        apply_state: "pending_idle",
        comparison: {
          version: 1,
          status: "known",
          desired_revision: 2,
          applied_revision: 1,
          selection_ids: ["docs"],
          changes: [
            {
              definition_id: "docs",
              action: "add",
              after: {
                definition_id: "docs",
                revision: 1,
                enabled: true,
                display_name: "Docs",
                runtime_name: "docs",
              },
            },
          ],
        },
      }}
    />,
  );
  expect(screen.queryByTestId("mcp-pending-change-docs")).not.toBeNull();
});
