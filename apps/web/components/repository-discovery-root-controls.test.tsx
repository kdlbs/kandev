import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/components/folder-picker", () => ({
  FolderPicker: () => <button type="button">Folder picker</button>,
}));

import { RepositoryDiscoveryRootControls } from "./repository-discovery-root-controls";

const baseProps = {
  isLoading: false,
  discoveryRoots: [],
  failedRoots: ["/Users/example/Library/Photo Booth Library"],
  homeConfirmationRequired: false,
  onChooseDiscoveryRoot: vi.fn(),
  onRefreshDiscovery: vi.fn(),
  onReconnectDiscoveryRoot: vi.fn(),
  onRemoveDiscoveryRoot: vi.fn(),
};

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("RepositoryDiscoveryRootControls", () => {
  it("shows a failed server root and a touch-sized refresh action without folder controls", () => {
    render(
      <RepositoryDiscoveryRootControls
        {...baseProps}
        showRootActions={false}
        presentation="picker"
      />,
    );

    expect(screen.getByTestId("discovery-failure")).toBeTruthy();
    expect(screen.getByText("/Users/example/Library/Photo Booth Library")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "workspaces:refreshRepositories" }).className,
    ).toContain("[@media(pointer:coarse)]:h-11");
    expect(screen.queryByRole("button", { name: "Folder picker" })).toBeNull();
  });

  it("leaves saved root recovery controls in place without a duplicate warning", () => {
    render(
      <RepositoryDiscoveryRootControls
        {...baseProps}
        discoveryRoots={[
          {
            id: "root-1",
            path: "/Users/example/Library/Photo Booth Library",
            display_path: "~/Library/Photo Booth Library",
            state: "reconnect_required",
          },
        ]}
      />,
    );

    expect(screen.queryByTestId("discovery-failure")).toBeNull();
    expect(screen.getByRole("button", { name: "workspaces:refreshRepositories" })).toBeTruthy();
    expect(screen.getByText("workspaces:removeDiscoveryRoot")).toBeTruthy();
  });

  it("disables the server recovery action while discovery is refreshing", () => {
    render(<RepositoryDiscoveryRootControls {...baseProps} showRootActions={false} isLoading />);

    expect(
      (screen.getByRole("button", { name: "workspaces:refreshRepositories" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });
});
