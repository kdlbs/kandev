import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const discovery = vi.hoisted(() => ({
  desktopRuntime: true,
  isLoading: false,
  isRefreshing: false,
  rootStates: [
    { id: "root-1", path: "/projects", display_path: "~/projects", state: "connected" },
    { id: "", path: "/configured", display_path: "/configured", state: "connected" },
  ],
  homeConfirmationRequired: true,
}));
const actions = vi.hoisted(() => ({
  refreshDiscovery: vi.fn(),
  handleChooseDiscoveryRoot: vi.fn(),
  handleReconnectDiscoveryRoot: vi.fn(),
  handleRemoveDiscoveryRoot: vi.fn(),
}));

vi.mock("@/hooks/domains/workspace/use-repository-discovery", () => ({
  useRepositoryDiscovery: () => discovery,
}));
vi.mock("@/hooks/domains/workspace/use-discovery-root-actions", () => ({
  useDiscoveryRootActions: () => actions,
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/components/repository-discovery-root-controls", () => ({
  RepositoryDiscoveryRootControls: (props: {
    discoveryRoots: Array<{ id: string }>;
    onChooseDiscoveryRoot: (path: string) => void;
  }) => (
    <div data-testid="root-controls">
      <span data-testid="root-count">{props.discoveryRoots.length}</span>
      <button type="button" onClick={() => props.onChooseDiscoveryRoot("/picked")}>
        Choose
      </button>
    </div>
  ),
}));

import { RepositoryDiscoveryControls } from "./repository-discovery-controls";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("RepositoryDiscoveryControls", () => {
  it("renders the shared desktop consent surface and only exposes operator roots", () => {
    render(<RepositoryDiscoveryControls workspaceId="workspace-1" />);

    expect(screen.getByTestId("root-controls")).toBeTruthy();
    expect(screen.getByTestId("root-count").textContent).toBe("1");
    fireEvent.click(screen.getByRole("button", { name: "Choose" }));
    expect(actions.handleChooseDiscoveryRoot).toHaveBeenCalledWith("/picked");
  });

  it("does not lease or render a surface when disabled", () => {
    render(<RepositoryDiscoveryControls workspaceId="workspace-1" enabled={false} />);

    expect(screen.queryByTestId("root-controls")).toBeNull();
  });
});
