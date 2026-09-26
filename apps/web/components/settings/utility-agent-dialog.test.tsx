import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UtilityAgentDialog } from "./utility-agent-dialog";

const mocks = vi.hoisted(() => ({
  promptEditor: vi.fn(),
  getTemplateVariables: vi.fn(),
  createUtilityAgent: vi.fn(),
  updateUtilityAgent: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: {
        items: [{ id: "profile-1", label: "Default", enabled: true, cli_passthrough: false }],
      },
    }),
}));
vi.mock("@/lib/api/domains/utility-api", () => ({
  getTemplateVariables: mocks.getTemplateVariables,
  createUtilityAgent: mocks.createUtilityAgent,
  updateUtilityAgent: mocks.updateUtilityAgent,
}));
vi.mock("./utility-agent-profile-picker", () => ({
  UtilityAgentProfilePicker: () => <div data-testid="utility-profile-picker" />,
}));
vi.mock("./settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    mocks.promptEditor(props);
    return <div data-testid={props.testId as string} />;
  },
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("UtilityAgentDialog", () => {
  it("keeps saved-prompt references disabled while exposing utility variables", async () => {
    mocks.getTemplateVariables.mockResolvedValue({
      variables: [{ name: "workspace", description: "Workspace name", example: "kandev" }],
    });

    render(<UtilityAgentDialog open onOpenChange={vi.fn()} agent={null} onSuccess={vi.fn()} />);

    await waitFor(() => {
      expect(mocks.promptEditor).toHaveBeenLastCalledWith(
        expect.objectContaining({
          language: "plaintext",
          placeholders: [expect.objectContaining({ key: "workspace" })],
          testId: "utility-agent-prompt-editor",
        }),
      );
    });
  });
});
