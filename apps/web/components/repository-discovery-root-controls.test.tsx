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
  it("keeps the folder and refresh actions available", () => {
    render(<RepositoryDiscoveryRootControls {...baseProps} presentation="picker" />);

    expect(screen.getByRole("button", { name: "Folder picker" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "workspaces:refreshRepositories" })).toBeTruthy();
  });

  it("leaves saved root recovery controls in place", () => {
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

    expect(screen.getByRole("button", { name: "workspaces:refreshRepositories" })).toBeTruthy();
    expect(screen.getByText("workspaces:removeDiscoveryRoot")).toBeTruthy();
  });

  it("disables refresh while discovery is refreshing", () => {
    render(<RepositoryDiscoveryRootControls {...baseProps} isLoading />);

    expect(
      (screen.getByRole("button", { name: "workspaces:refreshRepositories" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });
});
