import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type {
  OfficeConfigSyncController,
  OfficeConfigSyncFormState,
} from "@/hooks/domains/office/use-office-config-sync";
import type { OfficeConfigSyncConfig } from "@/lib/types/office-config-sync";
import { OfficeConfigSyncSection } from "./office-config-sync-section";

const EMPTY_FORM: OfficeConfigSyncFormState = {
  provider: "github",
  repo_owner: "",
  repo_name: "",
  project_path: "",
  branch: "main",
  path: "",
  interval_seconds: 300,
  poll_enabled: true,
};

function config(overrides: Partial<OfficeConfigSyncConfig> = {}): OfficeConfigSyncConfig {
  return {
    workspace_id: "workspace-1",
    provider: "github",
    repo_owner: "acme",
    repo_name: "office-config",
    project_path: "",
    branch: "main",
    path: "",
    interval_seconds: 300,
    poll_enabled: true,
    last_ok: true,
    created_at: "2026-08-01T00:00:00Z",
    updated_at: "2026-08-01T00:00:00Z",
    ...overrides,
  };
}

function formForConfig(currentConfig: OfficeConfigSyncConfig | null): OfficeConfigSyncFormState {
  if (!currentConfig) return { ...EMPTY_FORM };
  return {
    provider: currentConfig.provider,
    repo_owner: currentConfig.repo_owner,
    repo_name: currentConfig.repo_name,
    project_path: currentConfig.project_path,
    branch: currentConfig.branch,
    path: currentConfig.path,
    interval_seconds: currentConfig.interval_seconds,
    poll_enabled: currentConfig.poll_enabled,
  };
}

function controller(
  overrides: Partial<OfficeConfigSyncController> = {},
): OfficeConfigSyncController {
  const currentConfig = overrides.config === undefined ? null : overrides.config;
  return {
    config: currentConfig,
    form: formForConfig(currentConfig),
    loading: false,
    saving: false,
    syncing: false,
    update: vi.fn(),
    setProvider: vi.fn(),
    handleSave: vi.fn().mockResolvedValue(true),
    handleDelete: vi.fn().mockResolvedValue(true),
    handleSyncNow: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

let mockController: OfficeConfigSyncController = controller();
vi.mock("@/hooks/domains/office/use-office-config-sync", () => ({
  useOfficeConfigSync: () => mockController,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { workspaces: { activeId: string } }) => unknown) =>
    selector({ workspaces: { activeId: "workspace-1" } }),
}));

describe("OfficeConfigSyncSection", () => {
  afterEach(cleanup);

  it("shows the configure form (not a status card) when no config sync source exists yet", () => {
    mockController = controller({ config: null });
    render(<OfficeConfigSyncSection />);

    expect(screen.getByLabelText("Repository owner")).toBeTruthy();
    expect(screen.queryByTestId("office-config-sync-status")).toBeNull();
  });

  it("shows the status card and collapses the form behind a details disclosure when configured", () => {
    mockController = controller({ config: config() });
    render(<OfficeConfigSyncSection />);

    expect(screen.getByTestId("office-config-sync-status")).toBeTruthy();
    // The edit form lives inside a closed <details>; it is present in the DOM
    // but not what a user sees without expanding it first.
    expect(screen.getByText("Edit configuration").closest("details")?.open).toBe(false);
  });

  it("switches to the GitLab project-path field and clears GitHub fields on tab change", () => {
    const setProvider = vi.fn();
    mockController = controller({ config: null, setProvider });
    render(<OfficeConfigSyncSection />);

    // Radix Tabs activate on pointerdown, not click.
    fireEvent.mouseDown(screen.getByRole("tab", { name: "GitLab" }), { button: 0, ctrlKey: false });

    expect(setProvider).toHaveBeenCalledWith("gitlab");
  });

  it("does not render a page-local Save control", () => {
    mockController = controller({
      config: null,
      form: { ...EMPTY_FORM, repo_owner: "acme", repo_name: "office-config" },
    });
    render(<OfficeConfigSyncSection />);

    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
  });

  it("requires an explicit confirmation before calling handleDelete", async () => {
    const handleDelete = vi.fn().mockResolvedValue(true);
    mockController = controller({ config: config(), handleDelete });
    render(<OfficeConfigSyncSection />);

    // Expand the disclosure so the remove control is present, then confirm.
    fireEvent.click(screen.getByText("Edit configuration"));
    fireEvent.click(screen.getByTestId("office-config-sync-remove"));

    expect(handleDelete).not.toHaveBeenCalled();
    expect(screen.getByTestId("office-config-sync-remove-confirmation")).toBeTruthy();

    // InlineConfirmActions defers onConfirm to a microtask after the click.
    fireEvent.click(screen.getByTestId("office-config-sync-remove-confirm"));
    await waitFor(() => expect(handleDelete).toHaveBeenCalledTimes(1));
  });
});
