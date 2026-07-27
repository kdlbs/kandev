import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import type { GitHubAppRegistrationCatalogItem, GitHubStatus } from "@/lib/types/github";
import { GitHubAppConnectionPanel } from "./github-app-connection-panel";
import { GitHubConnectionDialog } from "./github-connection-dialog";

const mocks = vi.hoisted(() => ({
  mobile: false,
  registrations: [] as GitHubAppRegistrationCatalogItem[],
}));

const changeConnectionLabel = "Change connection";
const registrationDisplayName = "Work automation";
const githubAppLabel = "GitHub App";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.mobile }),
}));

vi.mock("@/hooks/domains/github/use-github-app-registrations", () => ({
  useGitHubAppRegistrations: (workspaceId: string) => ({
    workspaceId,
    catalog: { workspace_id: workspaceId, registrations: mocks.registrations },
    registrations: mocks.registrations,
    selected: mocks.registrations.find((item) => item.selected) ?? null,
    loaded: true,
    loading: false,
    error: null,
    mutating: false,
    refresh: vi.fn(),
    startManifest: vi.fn(),
    prepareImport: vi.fn(),
    importRegistration: vi.fn(),
    rename: vi.fn(),
    remove: vi.fn(),
    startInstall: vi.fn(),
  }),
}));

vi.mock("./github-app-import-form", () => ({
  GitHubAppImportForm: ({ onImported }: { onImported: (registrationId: string) => void }) => (
    <button onClick={() => onImported("registration-1")}>Complete test import</button>
  ),
}));

const status: GitHubStatus = {
  workspace_id: "workspace-1",
  automation: {
    workspace_id: "workspace-1",
    source: "pat",
    github_host: "github.com",
    login: "octocat",
    status: "active",
    credential_generation: 1,
  },
  personal: null,
  authenticated: true,
  username: "octocat",
  auth_method: "pat",
  token_configured: true,
  required_scopes: [],
};

const registration: GitHubAppRegistrationCatalogItem = {
  id: "registration-1",
  source: "managed",
  display_name: registrationDisplayName,
  github_host: "github.com",
  app_id: 42,
  client_id: "Iv1.test",
  slug: "work-automation",
  owner_login: "acme",
  owner_type: "Organization",
  visibility: "private",
  public_base_url: "https://kandev.example",
  credential_generation: 1,
  status: "active",
  webhook_status: "verified",
  created_at: "2026-07-21T00:00:00Z",
  updated_at: "2026-07-21T00:00:00Z",
  selected: true,
  binding_count: 3,
  workspace_binding_count: 2,
  shared: true,
  manifest_callback_url: "https://kandev.example/manifest/callback",
  install_callback_url: "https://kandev.example/install/callback",
  personal_callback_url: "https://kandev.example/personal/callback",
  webhook_url: "https://kandev.example/webhook",
};

function view(workspaceId = "workspace-1") {
  return render(
    <ToastProvider>
      <GitHubConnectionDialog
        status={{ ...status, workspace_id: workspaceId }}
        workspaceId={workspaceId}
        onSaved={vi.fn()}
      />
    </ToastProvider>,
  );
}

beforeEach(() => {
  mocks.mobile = false;
  mocks.registrations = [registration];
});
afterEach(() => cleanup());

describe("GitHubConnectionDialog", () => {
  it("uses a method list and shows workspace App choices and sharing impact", async () => {
    view();
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    expect(await screen.findByTestId("github-connection-desktop")).toBeTruthy();
    expect(screen.getByLabelText("Connection method")).toBeTruthy();
    expect(screen.getAllByText("Personal access token")).not.toHaveLength(0);
    expect(screen.getByText("GitHub CLI account")).toBeTruthy();

    fireEvent.click(screen.getByText(githubAppLabel, { selector: "span" }));
    expect(await screen.findByText(registrationDisplayName)).toBeTruthy();
    expect(screen.getByText(/Used by 2 workspaces/)).toBeTruthy();
    expect(screen.getByRole("button", { name: /Install for this workspace/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add existing App" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Create new App" })).toBeTruthy();
    expect(screen.queryByTestId("github-app-system-setup-link")).toBeNull();
  });

  it("renders one bounded mobile drawer with a single scroll body", async () => {
    mocks.mobile = true;
    view();
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    const drawer = await screen.findByTestId("github-connection-mobile");
    expect(drawer.className).toContain("100dvh");
    expect(drawer.querySelectorAll(".overflow-y-auto")).toHaveLength(1);
    expect(screen.queryByTestId("github-connection-desktop")).toBeNull();
  });

  it("returns to the registration catalog after importing an App", async () => {
    view();
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    fireEvent.click(await screen.findByText(githubAppLabel, { selector: "span" }));
    fireEvent.click(screen.getByRole("button", { name: "Add existing App" }));
    fireEvent.click(screen.getByRole("button", { name: "Complete test import" }));
    expect(await screen.findByText(registrationDisplayName)).toBeTruthy();
    const installButton = screen.getByRole("button", { name: /Install for this workspace/ });
    expect((installButton as HTMLButtonElement).disabled).toBe(false);
  });

  it("does not install invalid App registrations", async () => {
    mocks.registrations = [
      {
        ...registration,
        status: "invalid",
        last_error: "Credentials could not be restored.",
      },
    ];
    view();
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    fireEvent.click(await screen.findByText(githubAppLabel, { selector: "span" }));
    expect(await screen.findByText("Needs attention")).toBeTruthy();
    expect(screen.getByText("Credentials could not be restored.")).toBeTruthy();
    expect(
      (screen.getByRole("button", { name: /Install for this workspace/ }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });

  it("clears a selected App when it disappears from the catalog", async () => {
    const rendered = render(
      <ToastProvider>
        <GitHubAppConnectionPanel workspaceId="workspace-1" />
      </ToastProvider>,
    );
    expect(
      (screen.getByRole("button", { name: /Install for this workspace/ }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);

    mocks.registrations = [];
    rendered.rerender(
      <ToastProvider>
        <GitHubAppConnectionPanel workspaceId="workspace-1" />
      </ToastProvider>,
    );
    await waitFor(() =>
      expect(
        (screen.getByRole("button", { name: /Install for this workspace/ }) as HTMLButtonElement)
          .disabled,
      ).toBe(true),
    );
  });

  it("closes and resets local setup state when the workspace changes", async () => {
    const rendered = view();
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    fireEvent.click(await screen.findByText(githubAppLabel, { selector: "span" }));
    expect(await screen.findByText(registrationDisplayName)).toBeTruthy();

    rendered.rerender(
      <ToastProvider>
        <GitHubConnectionDialog
          status={{ ...status, workspace_id: "workspace-2" }}
          workspaceId="workspace-2"
          onSaved={vi.fn()}
        />
      </ToastProvider>,
    );
    await waitFor(() => expect(screen.queryByTestId("github-connection-desktop")).toBeNull());
  });
});
