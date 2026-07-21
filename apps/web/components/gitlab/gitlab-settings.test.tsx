import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GitLabIntegrationPage } from "./gitlab-settings";

const fetchGitLabStatusMock = vi.fn();

vi.mock("@/components/integrations/workspace-scoped-section", () => ({
  WorkspaceScopedSection: ({ children }: { children: (workspaceId: string) => React.ReactNode }) =>
    children("workspace-1"),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: vi.fn(),
}));

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  clearGitLabToken: vi.fn(),
  configureGitLabHost: vi.fn(),
  configureGitLabToken: vi.fn(),
  fetchGitLabStatus: (...args: unknown[]) => fetchGitLabStatusMock(...args),
}));

afterEach(() => {
  cleanup();
  fetchGitLabStatusMock.mockReset();
});

describe("GitLabIntegrationPage", () => {
  it("marks the changed host and its owning card dirty", async () => {
    fetchGitLabStatusMock.mockResolvedValue({
      authenticated: false,
      auth_method: "none",
      connection_error: "",
      glab_version: "",
      host: "https://gitlab.com",
      token_configured: false,
      username: "",
    });

    render(<GitLabIntegrationPage workspaceId="workspace-1" />);

    const host = await screen.findByDisplayValue("https://gitlab.com");
    const card = host.closest('[data-slot="card"]');
    expect(card?.getAttribute("data-settings-dirty")).toBe("false");

    fireEvent.change(host, { target: { value: "https://gitlab.example.com" } });

    expect(host.getAttribute("data-settings-dirty")).toBe("true");
    await waitFor(() => expect(card?.getAttribute("data-settings-dirty")).toBe("true"));
  });
});
