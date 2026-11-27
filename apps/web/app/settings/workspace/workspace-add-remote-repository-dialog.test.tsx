import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { AddRemoteRepositoryDialog } from "./workspace-add-remote-repository-dialog";
import type { Repository } from "@/lib/types/http";
import type { RepositoryInspection } from "@/lib/plugins/types";

const PLUGIN_HOST = "https://git.example.test";
const PLUGIN_URL = `${PLUGIN_HOST}/curtis/tooling`;
const PLUGIN_CLONE_URL = `${PLUGIN_URL}.git`;
const WORKSPACE_ID = "workspace-1";
const PLUGIN_FULL_NAME = "curtis/tooling";

const mocks = vi.hoisted(() => ({
  registerRemoteRepositoryAction: vi.fn(),
  useRemoteRepositoriesCalls: 0,
  inspections: new Map<string, RepositoryInspection>(),
  settledUrls: new Set<string>(),
  deferInspection: false,
  branchError: undefined as Error | undefined,
  inspectionError: undefined as Error | undefined,
}));

vi.mock("@/app/actions/workspaces", () => ({
  registerRemoteRepositoryAction: mocks.registerRemoteRepositoryAction,
}));
vi.mock("@/hooks/domains/integrations/use-remote-repositories", async () => {
  const support = await import("@/components/task-create-dialog-remote-repo-chip-test-support");
  return {
    useRemoteRepositories: () => {
      mocks.useRemoteRepositoriesCalls += 1;
      const accessible = support.makeAccessible({ repos: [support.githubSite()] });
      return {
        ...accessible,
        matchesURL: (url: string) => url.startsWith(`${PLUGIN_HOST}/`),
        availableProviders: ["github", "forgejo"],
        repos: [
          ...accessible.repos,
          {
            provider: "forgejo",
            id: "42",
            owner: "curtis",
            name: "tooling",
            fullName: PLUGIN_FULL_NAME,
            url: PLUGIN_URL,
            providerHost: PLUGIN_HOST,
            defaultBranch: "dev",
            private: true,
          },
        ],
      };
    },
  };
});
vi.mock("@/hooks/domains/github/use-branches-by-url", () => ({
  useBranchesByURL: () => ({
    branches: () => [{ name: "main", type: "remote" }],
    loading: () => false,
    error: () => mocks.branchError,
    ensure: () => undefined,
    clear: () => undefined,
  }),
}));
vi.mock("@/hooks/domains/github/use-pr-info-by-url", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/domains/github/use-pr-info-by-url")>()),
  usePRInfoByURL: () => ({
    ensure: (url: string) => {
      if (!mocks.deferInspection) mocks.settledUrls.add(url);
    },
    info: () => undefined,
    loading: () => false,
    settled: (url: string) => mocks.settledUrls.has(url),
    error: () => mocks.inspectionError,
    inspection: (url: string) => mocks.inspections.get(url),
    clear: () => undefined,
  }),
}));

const CONFIRM_LABEL = "Add to workspace";
const registered = {
  id: "repo-1",
  workspace_id: "workspace-1",
  name: "acme/site",
  source_type: "provider",
  local_path: "",
  provider: "github",
  provider_repo_id: "",
  provider_owner: "acme",
  provider_name: "site",
  remote_url: "https://github.com/acme/site.git",
  default_branch: "main",
  worktree_branch_prefix: "feature/",
  pull_before_worktree: true,
  setup_script: "",
  cleanup_script: "",
  dev_script: "",
  copy_files: "",
  created_at: "",
  updated_at: "",
} as unknown as Repository;

function renderDialog(open = true) {
  const onOpenChange = vi.fn();
  const onRegistered = vi.fn();
  const view = render(
    <TooltipProvider>
      <AddRemoteRepositoryDialog
        open={open}
        onOpenChange={onOpenChange}
        workspaceId={WORKSPACE_ID}
        onRegistered={onRegistered}
      />
    </TooltipProvider>,
  );
  const rerender = () =>
    view.rerender(
      <TooltipProvider>
        <AddRemoteRepositoryDialog
          open={open}
          onOpenChange={onOpenChange}
          workspaceId={WORKSPACE_ID}
          onRegistered={onRegistered}
        />
      </TooltipProvider>,
    );
  return { onOpenChange, onRegistered, view, rerender };
}

function pickOption(fullName: string, providerTab?: string) {
  fireEvent.click(screen.getByTestId("remote-repo-chip-trigger"));
  if (providerTab) {
    // Two providers are listed, so the picker shows provider tabs and only
    // the active provider's repositories; Radix activates a tab on mouse down.
    const tab = screen.getByRole("tab", { name: providerTab });
    fireEvent.mouseDown(tab);
    fireEvent.click(tab);
  }
  const option = screen
    .getAllByTestId("remote-repo-option")
    .find((element) => element.textContent?.includes(fullName));
  if (!option) throw new Error(`option ${fullName} not rendered`);
  fireEvent.click(option);
}

function pastePluginURL() {
  fireEvent.click(screen.getByTestId("remote-repo-chip-trigger"));
  const input = screen.getByTestId("remote-repo-input");
  fireEvent.change(input, { target: { value: PLUGIN_URL } });
  fireEvent.keyDown(input, { key: "Enter" });
}

function confirmButton() {
  return screen.getByRole("button", { name: CONFIRM_LABEL });
}

afterEach(() => {
  cleanup();
  mocks.registerRemoteRepositoryAction.mockReset();
  mocks.useRemoteRepositoriesCalls = 0;
  mocks.inspections.clear();
  mocks.settledUrls.clear();
  mocks.deferInspection = false;
  mocks.branchError = undefined;
  mocks.inspectionError = undefined;
});

describe("AddRemoteRepositoryDialog resolution", () => {
  it("uses the inspected default when branches arrive before plugin inspection", async () => {
    mocks.deferInspection = true;
    mocks.registerRemoteRepositoryAction.mockResolvedValue(registered);
    const { rerender } = renderDialog();
    pastePluginURL();
    expect(confirmButton().hasAttribute("disabled")).toBe(true);
    mocks.inspections.set(PLUGIN_URL, {
      providerId: "forgejo",
      providerHost: PLUGIN_HOST,
      repositoryId: "42",
      ownerOrProject: "curtis",
      repositoryName: "tooling",
      cloneUrl: PLUGIN_CLONE_URL,
      defaultBranch: "dev",
    } as RepositoryInspection);
    mocks.settledUrls.add(PLUGIN_URL);
    rerender();
    await waitFor(() => expect(confirmButton().hasAttribute("disabled")).toBe(false));
    fireEvent.click(confirmButton());
    await waitFor(() =>
      expect(mocks.registerRemoteRepositoryAction).toHaveBeenCalledWith(
        WORKSPACE_ID,
        expect.objectContaining({ default_branch: "dev" }),
      ),
    );
  });

  it.each([
    ["acme/site", undefined],
    [PLUGIN_FULL_NAME, "Forgejo"],
  ])("saves picked %s when optional branch lookup fails", async (fullName, providerTab) => {
    mocks.branchError = new Error("branch service unavailable");
    mocks.inspectionError = new Error("optional inspection unavailable");
    mocks.registerRemoteRepositoryAction.mockResolvedValue(registered);
    const { onRegistered } = renderDialog();
    pickOption(fullName, providerTab);
    await waitFor(() => expect(confirmButton().hasAttribute("disabled")).toBe(false));
    fireEvent.click(confirmButton());
    await waitFor(() => expect(onRegistered).toHaveBeenCalledWith(registered));
  });

  it("blocks pasted plugin registration when required inspection fails", async () => {
    mocks.inspectionError = new Error("provider inspection unavailable");
    renderDialog();
    pastePluginURL();
    expect(confirmButton().hasAttribute("disabled")).toBe(true);
    fireEvent.click(confirmButton());
    expect(mocks.registerRemoteRepositoryAction).not.toHaveBeenCalled();
  });

  it("preserves a chosen branch after inspected metadata refresh", async () => {
    mocks.registerRemoteRepositoryAction.mockResolvedValue(registered);
    const { rerender } = renderDialog();
    pickOption(PLUGIN_FULL_NAME, "Forgejo");
    fireEvent.click(screen.getByTestId("remote-branch-chip-trigger"));
    fireEvent.click(await screen.findByRole("option", { name: /^main/ }));
    mocks.inspections.set(PLUGIN_URL, {
      providerId: "forgejo",
      providerHost: PLUGIN_HOST,
      repositoryId: "42",
      ownerOrProject: "curtis",
      repositoryName: "tooling",
      cloneUrl: PLUGIN_CLONE_URL,
      defaultBranch: "dev",
    } as RepositoryInspection);
    rerender();
    fireEvent.click(confirmButton());
    await waitFor(() =>
      expect(mocks.registerRemoteRepositoryAction).toHaveBeenCalledWith(
        WORKSPACE_ID,
        expect.objectContaining({ default_branch: "main" }),
      ),
    );
  });
});

describe("AddRemoteRepositoryDialog", () => {
  it("does not load provider catalogs while closed", () => {
    renderDialog(false);
    expect(mocks.useRemoteRepositoriesCalls).toBe(0);
  });

  it("registers the picked GitHub repository and reports it back to the page", async () => {
    mocks.registerRemoteRepositoryAction.mockResolvedValue(registered);
    const { onOpenChange, onRegistered } = renderDialog();
    expect(confirmButton().hasAttribute("disabled")).toBe(true);

    pickOption("acme/site");
    await waitFor(() => expect(confirmButton().hasAttribute("disabled")).toBe(false));
    fireEvent.click(confirmButton());

    await waitFor(() => expect(onRegistered).toHaveBeenCalledWith(registered));
    expect(mocks.registerRemoteRepositoryAction).toHaveBeenCalledWith(WORKSPACE_ID, {
      remote_url: "https://github.com/acme/site",
      default_branch: "main",
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("sends the provider descriptor for a picked plugin repository", async () => {
    mocks.registerRemoteRepositoryAction.mockResolvedValue(registered);
    renderDialog();

    pickOption(PLUGIN_FULL_NAME, "Forgejo");
    await waitFor(() => expect(confirmButton().hasAttribute("disabled")).toBe(false));
    fireEvent.click(confirmButton());

    await waitFor(() => expect(mocks.registerRemoteRepositoryAction).toHaveBeenCalled());
    expect(mocks.registerRemoteRepositoryAction).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.objectContaining({
        remote_url: PLUGIN_URL,
        default_branch: "dev",
        provider: "forgejo",
        provider_host: PLUGIN_HOST,
        provider_repo_id: "42",
        provider_owner: "curtis",
        provider_name: "tooling",
      }),
    );
  });

  it("waits for inspection of a pasted plugin URL and submits its verified descriptor", async () => {
    mocks.registerRemoteRepositoryAction.mockResolvedValue(registered);
    mocks.inspections.set(PLUGIN_URL, {
      providerId: "forgejo",
      providerHost: PLUGIN_HOST,
      providerScope: "scope-1",
      repositoryId: "42",
      ownerOrProject: "curtis",
      repositoryName: "tooling",
      cloneUrl: PLUGIN_CLONE_URL,
      defaultBranch: "dev",
    } as RepositoryInspection);
    renderDialog();

    pastePluginURL();

    await waitFor(() => expect(confirmButton().hasAttribute("disabled")).toBe(false));
    fireEvent.click(confirmButton());

    await waitFor(() => expect(mocks.registerRemoteRepositoryAction).toHaveBeenCalled());
    expect(mocks.registerRemoteRepositoryAction).toHaveBeenCalledWith(
      WORKSPACE_ID,
      expect.objectContaining({
        remote_url: PLUGIN_CLONE_URL,
        default_branch: "dev",
        provider: "forgejo",
        provider_host: PLUGIN_HOST,
        provider_scope: "scope-1",
        provider_repo_id: "42",
        provider_owner: "curtis",
        provider_name: "tooling",
      }),
    );
  });

  it("keeps the dialog open and shows the failure when registration is rejected", async () => {
    mocks.registerRemoteRepositoryAction.mockRejectedValue(new Error("provider unavailable"));
    const { onOpenChange, onRegistered } = renderDialog();

    pickOption("acme/site");
    await waitFor(() => expect(confirmButton().hasAttribute("disabled")).toBe(false));
    fireEvent.click(confirmButton());

    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain("provider unavailable"),
    );
    expect(onRegistered).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });

  it("refuses to close while a registration is pending", async () => {
    mocks.registerRemoteRepositoryAction.mockReturnValue(new Promise(() => undefined));
    const { onOpenChange } = renderDialog();

    pickOption("acme/site");
    await waitFor(() => expect(confirmButton().hasAttribute("disabled")).toBe(false));
    fireEvent.click(confirmButton());

    await waitFor(() => expect(screen.getByRole("status")).toBeTruthy());
    const cancel = screen.getByRole("button", { name: "Cancel" });
    expect(cancel.hasAttribute("disabled")).toBe(true);
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });
});
