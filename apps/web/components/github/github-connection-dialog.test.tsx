import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { GitHubStatus } from "@/lib/types/github";
import { GitHubConnectionDialog } from "./github-connection-dialog";

afterEach(() => cleanup());

describe("GitHubConnectionDialog", () => {
  it("routes an unavailable deployment App to System Settings", async () => {
    const status: GitHubStatus = {
      workspace_id: "workspace-1",
      automation: {
        workspace_id: "workspace-1",
        source: "github_app_installation",
        github_host: "github.com",
        status: "invalid",
        credential_generation: 1,
      },
      personal: null,
      app_available: false,
      authenticated: false,
      username: "",
      auth_method: "github_app_installation",
      token_configured: false,
      required_scopes: [],
    };

    render(
      <GitHubConnectionDialog
        status={status}
        workspaceId="workspace-1"
        busy={false}
        onSaved={vi.fn()}
        onAppInstall={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Change connection" }));
    const setupLink = await waitFor(() => screen.getByTestId("github-app-system-setup-link"));

    expect(setupLink.getAttribute("href")).toBe("/settings/system/github-app");
    expect(screen.getByText(/does not have a GitHub App yet/)).toBeTruthy();
    expect(screen.getByText(/PAT and CLI act as people/)).toBeTruthy();
    expect(screen.queryByTestId("github-app-install-button")).toBeNull();
  });
});
