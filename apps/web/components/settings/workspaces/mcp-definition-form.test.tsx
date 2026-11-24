import { render, screen, fireEvent, act } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { MCPDefinitionForm } from "./mcp-definition-form";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: vi.fn(),
}));
vi.mock("@/hooks/domains/settings/use-workspace-secret-options", () => ({
  useWorkspaceSecretOptions: () => ({ items: [], loaded: true, loading: false }),
}));
it("editing description preserves argv with spaces", async () => {
  const save = vi.fn().mockResolvedValue(undefined);
  render(
    <MCPDefinitionForm
      workspaceId="ws"
      server={{
        id: "d",
        workspace_id: "ws",
        runtime_name: "docs",
        normalized_runtime_name: "docs",
        display_name: "Docs",
        enabled: true,
        execution_mode: "existing_executable",
        transport: "stdio",
        configuration: {
          command: "node",
          args: ["server.js", "--name", "Hello World", "", "  spaced  ", "C:\\Program Files\\MCP"],
        },
        source: "custom",
        revision: 1,
        created_at: "now",
        updated_at: "now",
      }}
      onSave={save}
      onClose={() => {}}
    />,
  );
  fireEvent.change(screen.getByLabelText("Description"), { target: { value: "updated" } });
  const calls = vi.mocked(useSettingsSaveContributor).mock.calls;
  await act(async () => {
    await calls[calls.length - 1][0].save(calls[calls.length - 1][0].revision);
  });
  expect(save.mock.calls[0][0].configuration.args).toEqual([
    "server.js",
    "--name",
    "Hello World",
    "",
    "  spaced  ",
    "C:\\Program Files\\MCP",
  ]);
});
