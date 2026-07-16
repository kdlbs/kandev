import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QuickChatSetup } from "./quick-chat-setup";

let defaultAgentId = "";
const AGENT_SELECTOR_TEST_ID = "agent-profile-selector";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: { items: [{ id: "agent-a" }, { id: "agent-b" }] },
      workspaces: {
        items: [{ id: "workspace-1", default_agent_profile_id: defaultAgentId }],
      },
    }),
}));

vi.mock("@/components/task-create-dialog-options", () => ({
  useAgentProfileOptions: () => [],
}));

vi.mock("@/components/task-create-dialog-selectors", () => ({
  AgentSelector: ({
    value,
    onValueChange,
    triggerClassName,
  }: {
    value: string;
    onValueChange: (id: string) => void;
    triggerClassName?: string;
  }) => (
    <button
      type="button"
      data-testid={AGENT_SELECTOR_TEST_ID}
      className={triggerClassName}
      onClick={() => onValueChange("agent-b")}
    >
      {value || "Select agent"}
    </button>
  ),
}));

vi.mock("@/components/task-create-dialog-workspace-repo-chips", () => ({
  WorkspaceRepoChips: () => null,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("@/components/task-create-dialog-repositories-state", () => ({
  useRepositoriesState: () => ({
    repositories: [],
    addRepository: vi.fn(),
    removeRepository: vi.fn(),
    updateRepository: vi.fn(),
  }),
}));

vi.mock("@/hooks/domains/workspace/use-repositories", () => ({
  useRepositories: () => ({ repositories: [], isLoading: false }),
}));

const props = {
  workspaceId: "workspace-1",
  showIntroduction: true,
  pendingAgentId: null,
  onStart: vi.fn(),
  onCancel: vi.fn(),
  onKindChange: vi.fn(),
};

beforeEach(() => {
  defaultAgentId = "";
  vi.clearAllMocks();
});

afterEach(cleanup);

describe("QuickChatSetup default agent", () => {
  it("offers configuration chat as an explicit creation type", () => {
    render(<QuickChatSetup {...props} />);

    expect(screen.getByRole("radio", { name: "Quick chat" }).getAttribute("aria-checked")).toBe(
      "true",
    );
    fireEvent.click(screen.getByRole("radio", { name: "Configuration chat" }));

    expect(props.onKindChange).toHaveBeenCalledWith("config");
  });

  it("renders the agent selector with a visible field border", () => {
    render(<QuickChatSetup {...props} />);

    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).className).toContain("border-input");
  });

  it("uses a default agent that arrives after the setup mounts", () => {
    const { rerender } = render(<QuickChatSetup {...props} />);
    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain("Select agent");

    defaultAgentId = "agent-a";
    rerender(<QuickChatSetup {...props} />);

    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain("agent-a");
  });

  it("preserves an explicit selection when the workspace default changes", () => {
    const { rerender } = render(<QuickChatSetup {...props} />);
    fireEvent.click(screen.getByTestId(AGENT_SELECTOR_TEST_ID));
    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain("agent-b");

    defaultAgentId = "agent-a";
    rerender(<QuickChatSetup {...props} />);

    expect(screen.getByTestId(AGENT_SELECTOR_TEST_ID).textContent).toContain("agent-b");
  });
});
