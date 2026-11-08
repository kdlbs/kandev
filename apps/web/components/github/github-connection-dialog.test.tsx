const ENTERPRISE_HOST = "git.company.example";
import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import type { TaskGitCredentialsState } from "@/hooks/domains/github/use-task-git-credentials";
import type { GitHubAppRegistrationCatalogItem, GitHubStatus } from "@/lib/types/github";
import { GitHubAppConnectionPanel } from "./github-app-connection-panel";
import { GitHubConnectionDialog } from "./github-connection-dialog";

const mocks = vi.hoisted(() => ({
  mobile: false,
  registrations: [] as GitHubAppRegistrationCatalogItem[],
  setConnection: vi.fn(),
}));

const changeConnectionLabel = "Change connection";
const registrationDisplayName = "Work automation";
const githubAppLabel = "GitHub App";
const saveChangesLabel = "Save changes";
const personalAccessTokenLabel = "Personal access token";
const WORKSPACE_ID = "workspace-1";
const DESKTOP_CONNECTION_TEST_ID = "github-connection-desktop";
const taskAccess: TaskGitCredentialsState = {
  mode: "managed",
  loading: false,
  error: false,
  save: vi.fn(),
};

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

vi.mock("@/lib/api/domains/github-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/github-api")>()),
  setGitHubWorkspaceConnection: mocks.setConnection,
}));

const status: GitHubStatus = {
  workspace_id: WORKSPACE_ID,
  automation: {
    workspace_id: WORKSPACE_ID,
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

function view(workspaceId = WORKSPACE_ID, currentTaskAccess = taskAccess) {
  const onSaved = vi.fn();
  return render(
    <ToastProvider>
      <GitHubConnectionDialog
        status={{ ...status, workspace_id: workspaceId }}
        workspaceId={workspaceId}
        onSaved={onSaved}
        taskAccess={currentTaskAccess}
      />
    </ToastProvider>,
  );
}

function PartialSaveHarness() {
  const [mode, setMode] = useState<TaskGitCredentialsState["mode"]>("managed");
  const save = vi.fn(async (nextMode: TaskGitCredentialsState["mode"]) => {
    setMode(nextMode);
    return true;
  });
  return (
    <ToastProvider>
      <GitHubConnectionDialog
        status={status}
        workspaceId={WORKSPACE_ID}
        onSaved={vi.fn()}
        taskAccess={{ mode, loading: false, error: false, save }}
      />
    </ToastProvider>
  );
}

beforeEach(() => {
  mocks.mobile = false;
  mocks.registrations = [registration];
  mocks.setConnection.mockReset();
  mocks.setConnection.mockResolvedValue(status.automation);
});

describe("GitHubConnectionDialog task access", () => {
  it("saves connection and task access drafts with one action", async () => {
    const save = vi.fn().mockResolvedValue(true);
    const rendered = view(WORKSPACE_ID, { ...taskAccess, save });
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    const dialog = await screen.findByTestId(DESKTOP_CONNECTION_TEST_ID);
    const executor = screen
      .getByTestId("github-task-access-option-executor")
      .querySelector('[role="radio"]')!;
    fireEvent.click(executor);
    fireEvent.change(screen.getByLabelText(personalAccessTokenLabel), {
      target: { value: "ghp_replacement" },
    });

    expect(screen.queryByRole("button", { name: "Connect token" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Save task access" })).toBeNull();
    expect(screen.getAllByRole("button", { name: saveChangesLabel })).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));

    await waitFor(() =>
      expect(mocks.setConnection).toHaveBeenCalledWith(WORKSPACE_ID, {
        source: "pat",
        token: "ghp_replacement",
        host: "github.com",
      }),
    );
    expect(save).toHaveBeenCalledWith("executor");
    await waitFor(() => expect(dialog.getAttribute("data-state")).toBe("closed"));
    expect(rendered.baseElement.textContent).toContain("GitHub access settings saved");
  });

  it("preserves host and credential drafts across responsive surfaces", async () => {
    const rendered = view();
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    fireEvent.change(screen.getByLabelText("GitHub host"), {
      target: { value: ENTERPRISE_HOST },
    });
    fireEvent.change(screen.getByLabelText(personalAccessTokenLabel), {
      target: { value: "ghp_enterprise" },
    });
    mocks.mobile = true;
    rendered.rerender(
      <ToastProvider>
        <GitHubConnectionDialog
          status={status}
          workspaceId={WORKSPACE_ID}
          onSaved={vi.fn()}
          taskAccess={taskAccess}
        />
      </ToastProvider>,
    );
    expect(await screen.findByTestId("github-connection-mobile")).toBeTruthy();
    expect((screen.getByLabelText("GitHub host") as HTMLInputElement).value).toBe(ENTERPRISE_HOST);
    expect((screen.getByLabelText(personalAccessTokenLabel) as HTMLInputElement).value).toBe(
      "ghp_enterprise",
    );
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
    await waitFor(() =>
      expect(mocks.setConnection).toHaveBeenCalledWith(WORKSPACE_ID, {
        source: "pat",
        host: ENTERPRISE_HOST,
        token: "ghp_enterprise",
      }),
    );
  });
});

describe("GitHubConnectionDialog status refresh during save", () => {
  it.each([
    [false, false],
    [false, true],
    [true, false],
    [true, true],
  ])(
    "settles a pending save after status refresh (mobile=%s, success=%s)",
    async (mobile, success) => {
      mocks.mobile = mobile;
      let resolveSave: (value: unknown) => void = () => {};
      let rejectSave: (reason: Error) => void = () => {};
      mocks.setConnection.mockImplementation(
        () =>
          new Promise((resolve, reject) => {
            resolveSave = resolve;
            rejectSave = reject;
          }),
      );
      const onSaved = vi.fn();
      const renderDialog = (current: GitHubStatus) => (
        <ToastProvider>
          <GitHubConnectionDialog
            status={current}
            workspaceId={WORKSPACE_ID}
            onSaved={onSaved}
            taskAccess={taskAccess}
          />
        </ToastProvider>
      );
      const rendered = render(renderDialog(status));
      const trigger = screen.getByRole("button", { name: changeConnectionLabel });
      fireEvent.click(trigger);
      fireEvent.change(screen.getByLabelText(personalAccessTokenLabel), {
        target: { value: "ghp_pending" },
      });
      fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
      rendered.rerender(
        renderDialog({ ...status, automation: { ...status.automation!, source: "gh_cli" } }),
      );
      await act(async () => {
        if (success) resolveSave(status.automation);
        else rejectSave(new Error("Token rejected"));
      });
      if (success) {
        await waitFor(() => expect(onSaved).toHaveBeenCalledTimes(1));
        expect(trigger.getAttribute("aria-expanded")).toBe("false");
      } else {
        await screen.findByText("Token rejected");
        expect(screen.getByLabelText(personalAccessTokenLabel)).toHaveProperty("disabled", false);
        expect(screen.getByLabelText(personalAccessTokenLabel)).toHaveProperty(
          "value",
          "ghp_pending",
        );
        expect(screen.getByRole("button", { name: saveChangesLabel })).toHaveProperty(
          "disabled",
          false,
        );
        expect(onSaved).not.toHaveBeenCalled();
      }
    },
  );
});

describe("GitHubConnectionDialog stale save results", () => {
  it("ignores a save result after closing and reopening the same connection draft", async () => {
    let finish: (value: unknown) => void = () => {};
    mocks.setConnection.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    view();
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    fireEvent.change(screen.getByLabelText(personalAccessTokenLabel), {
      target: { value: "ghp_pending" },
    });
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
    expect(screen.getByRole("radio", { name: /^GitHub App/ })).toHaveProperty("disabled", true);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    fireEvent.change(screen.getByLabelText(personalAccessTokenLabel), {
      target: { value: "ghp_new_draft" },
    });
    await act(async () => finish(status.automation));
    await waitFor(() => expect(screen.queryByText("GitHub access settings saved")).toBeNull());
    expect((screen.getByLabelText(personalAccessTokenLabel) as HTMLInputElement).value).toBe(
      "ghp_new_draft",
    );
  });

  it("does not report a discarded task access save as successful", async () => {
    const save = vi.fn().mockResolvedValue(false);
    view(WORKSPACE_ID, { ...taskAccess, save });
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    const dialog = await screen.findByTestId(DESKTOP_CONNECTION_TEST_ID);
    const executor = screen
      .getByTestId("github-task-access-option-executor")
      .querySelector('[role="radio"]')!;
    fireEvent.click(executor);

    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));

    await waitFor(() => expect(save).toHaveBeenCalledWith("executor"));
    expect(screen.queryByText("GitHub access settings saved")).toBeNull();
    expect(dialog.getAttribute("data-state")).toBe("open");
  });

  it("preserves a failed PAT draft when task access saves successfully", async () => {
    mocks.setConnection.mockRejectedValue(new Error("Token rejected"));
    render(<PartialSaveHarness />);
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    const token = screen.getByLabelText(personalAccessTokenLabel) as HTMLInputElement;
    fireEvent.change(token, { target: { value: "ghp_retry_me" } });
    fireEvent.click(
      screen.getByTestId("github-task-access-option-executor").querySelector('[role="radio"]')!,
    );

    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));

    await screen.findByText(/Some settings were saved/);
    expect(token.value).toBe("ghp_retry_me");
  });

  it("keeps an invalid PAT draft in the open connection dialog", async () => {
    const onSaved = vi.fn();
    mocks.setConnection.mockRejectedValue(new Error("GitHub token is invalid"));
    render(
      <ToastProvider>
        <GitHubConnectionDialog
          status={status}
          workspaceId={WORKSPACE_ID}
          onSaved={onSaved}
          taskAccess={taskAccess}
        />
      </ToastProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    const dialog = await screen.findByTestId(DESKTOP_CONNECTION_TEST_ID);
    const token = screen.getByLabelText(personalAccessTokenLabel) as HTMLInputElement;
    fireEvent.change(token, { target: { value: "ghp_invalid" } });
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));

    await screen.findByText("GitHub token is invalid");
    expect(token.value).toBe("ghp_invalid");
    expect(dialog.getAttribute("data-state")).toBe("open");
    expect(onSaved).not.toHaveBeenCalled();
  });
});
afterEach(() => cleanup());

describe("GitHubConnectionDialog", () => {
  it("uses a method list and shows workspace App choices and sharing impact", async () => {
    view();
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    expect(await screen.findByTestId(DESKTOP_CONNECTION_TEST_ID)).toBeTruthy();
    expect(screen.getByLabelText("Connection method")).toBeTruthy();
    expect(screen.getAllByText(personalAccessTokenLabel)).not.toHaveLength(0);
    expect(screen.getByText("GitHub CLI account")).toBeTruthy();

    fireEvent.click(screen.getByText(githubAppLabel, { selector: "span" }));
    expect(await screen.findByText(registrationDisplayName)).toBeTruthy();
    expect(screen.getByText(/Used by 2 workspaces/)).toBeTruthy();
    expect(screen.getByRole("button", { name: /Install for this workspace/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add existing App" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Create new App" })).toBeTruthy();
    expect(screen.queryByTestId("github-app-system-setup-link")).toBeNull();
  });

  it("filters registered Apps to the proposed host and clears PAT secrets on host changes", async () => {
    mocks.registrations = [
      registration,
      {
        ...registration,
        id: "enterprise-app",
        github_host: ENTERPRISE_HOST,
        display_name: "Enterprise automation",
      },
    ];
    view();
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    fireEvent.change(screen.getByLabelText(personalAccessTokenLabel), {
      target: { value: "ghp_old" },
    });
    fireEvent.change(screen.getByLabelText("GitHub host"), {
      target: { value: ENTERPRISE_HOST },
    });
    expect((screen.getByLabelText(personalAccessTokenLabel) as HTMLInputElement).value).toBe("");
    fireEvent.click(screen.getByText(githubAppLabel, { selector: "span" }));
    expect(await screen.findByText("Enterprise automation")).toBeTruthy();
    expect(screen.queryByText(registrationDisplayName)).toBeNull();
  });
});

describe("GitHubConnectionDialog responsive App setup", () => {
  it("renders one bounded mobile drawer with a single scroll body", async () => {
    mocks.mobile = true;
    view();
    fireEvent.click(screen.getByRole("button", { name: changeConnectionLabel }));
    const drawer = await screen.findByTestId("github-connection-mobile");
    expect(drawer.className).toContain("100dvh");
    expect(drawer.querySelectorAll(".overflow-y-auto")).toHaveLength(1);
    expect(screen.queryByTestId(DESKTOP_CONNECTION_TEST_ID)).toBeNull();
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
          taskAccess={taskAccess}
        />
      </ToastProvider>,
    );
    await waitFor(() => expect(screen.queryByTestId(DESKTOP_CONNECTION_TEST_ID)).toBeNull());
  });
});
