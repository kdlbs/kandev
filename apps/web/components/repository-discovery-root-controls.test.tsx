import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/components/folder-picker", () => ({
  FolderPicker: ({ onChange }: { onChange: (path: string) => void }) => (
    <button type="button" onClick={() => onChange("/picked")}>
      Folder picker
    </button>
  ),
}));

import { RepositoryDiscoveryRootControls } from "./repository-discovery-root-controls";

const baseProps = {
  isLoading: false,
  discoveryRoots: [],
  homeConfirmationRequired: false,
  onConfirmHomeDiscovery: vi.fn(),
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

  it("confirms Home directly without using the folder picker", () => {
    render(
      <RepositoryDiscoveryRootControls
        {...baseProps}
        homeConfirmationRequired
        presentation="picker"
      />,
    );

    screen.getByRole("button", { name: "workspaces:continueHomeDiscovery" }).click();

    expect(baseProps.onConfirmHomeDiscovery).toHaveBeenCalledOnce();
    expect(baseProps.onChooseDiscoveryRoot).not.toHaveBeenCalled();
  });

  it("keeps the Home confirmation name stable and announces while saving", () => {
    render(
      <RepositoryDiscoveryRootControls
        {...baseProps}
        homeConfirmationRequired
        isConfirmingHomeDiscovery
      />,
    );

    const button = screen.getByRole("button", {
      name: "workspaces:continueHomeDiscovery",
    }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    expect(button.getAttribute("aria-busy")).toBe("true");
    expect(screen.getByRole("status").textContent).toBe("common:loading");
  });
});
