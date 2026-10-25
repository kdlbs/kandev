import { render, screen, fireEvent } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { MCPSelectionContent } from "./mcp-selection-picker";
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
it("can remove an existing direct selection after the definition is disabled", () => {
  const onChange = vi.fn();
  render(
    <MCPSelectionContent
      workspaceId="ws"
      definitions={[
        {
          id: "d",
          workspace_id: "ws",
          runtime_name: "docs",
          normalized_runtime_name: "docs",
          display_name: "Docs",
          enabled: false,
          execution_mode: "remote",
          transport: "sse",
          configuration: { url: "https://example.test" },
          source: "custom",
          revision: 1,
          created_at: "now",
          updated_at: "now",
        },
      ]}
      selectedIds={["d"]}
      onSelectedIdsChange={onChange}
      query=""
      onQueryChange={() => {}}
    />,
  );
  expect(screen.getByRole("checkbox", { name: "Docs" }).getAttribute("aria-checked")).toBe("true");
  expect(screen.getByRole("checkbox", { name: "Docs" }).hasAttribute("disabled")).toBe(false);
  fireEvent.click(screen.getByRole("checkbox", { name: "Docs" }));
  expect(onChange).toHaveBeenCalledWith([]);
});
