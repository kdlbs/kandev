import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { ApiError } from "@/lib/api/client";

const mocks = vi.hoisted(() => ({
  deleteSecret: vi.fn(),
  listSecretReferences: vi.fn(),
  addSecret: vi.fn(),
  updateSecret: vi.fn(),
  removeSecret: vi.fn(),
  toast: vi.fn(),
  items: [] as SecretListItem[],
  responsive: { isFinePointer: true },
}));

vi.mock("@/lib/api/domains/secrets-api", () => ({
  createSecret: vi.fn(),
  updateSecret: mocks.updateSecret,
  deleteSecret: mocks.deleteSecret,
  listSecretReferences: mocks.listSecretReferences,
}));
vi.mock("@/hooks/domains/settings/use-secrets", () => ({
  useSecrets: () => ({
    loaded: true,
    items: mocks.items,
    addSecret: mocks.addSecret,
    updateSecret: mocks.updateSecret,
    removeSecret: mocks.removeSecret,
  }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ addSecret: mocks.addSecret, workspaces: { items: [] } }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => mocks.responsive,
}));
vi.mock("@/components/settings/settings-page-template", () => ({
  SettingsPageTemplate: ({ children }: { children: ReactNode }) =>
    createElement("div", null, children),
}));
vi.mock("@/components/settings/workspaces/workspace-section-header", () => ({
  WorkspaceSectionHeader: () => null,
}));
vi.mock("./copy-move-secret-dialog", () => ({ CopyMoveSecretDialog: () => null }));

import { getSecretDraftMeta, SecretsSettings } from "./secrets-settings";

const secret: SecretListItem = {
  id: "secret-1",
  name: "Saved secret",
  has_value: true,
  created_at: "",
  updated_at: "",
};

const DELETE_BUTTON = "Delete secret Saved secret";
const CONFIRM_POPOVER_TEST_ID = "secret-delete-confirm-popover";
const CONFIRM_TEST_ID = "secret-delete-confirm";

function renderSettings() {
  render(createElement(SecretsSettings, { initialItems: mocks.items }));
}

async function openDeleteConfirmation() {
  fireEvent.click(screen.getByRole("button", { name: DELETE_BUTTON }));
  const popover = await screen.findByTestId(CONFIRM_POPOVER_TEST_ID);
  const confirm = within(popover).getByTestId(CONFIRM_TEST_ID) as HTMLButtonElement;
  await waitFor(() => expect(confirm.disabled).toBe(false));
  return popover;
}

beforeEach(() => {
  mocks.items = [secret];
  mocks.responsive.isFinePointer = true;
  mocks.deleteSecret.mockReset();
  mocks.deleteSecret.mockResolvedValue(undefined);
  mocks.listSecretReferences.mockReset();
  mocks.listSecretReferences.mockResolvedValue([]);
  mocks.removeSecret.mockReset();
  mocks.toast.mockReset();
});

afterEach(() => cleanup());

describe("getSecretDraftMeta", () => {
  it("treats a newly opened secret form as dirty before persistence", () => {
    const state = getSecretDraftMeta([], null, true, { name: "", value: "" });

    expect(state.isDirty).toBe(true);
    expect(state.revision.startsWith("new:")).toBe(true);
  });

  it("tracks existing secret edits against the saved name", () => {
    expect(
      getSecretDraftMeta([secret], secret.id, false, { name: secret.name, value: "" }).isDirty,
    ).toBe(false);
    expect(
      getSecretDraftMeta([secret], secret.id, false, { name: "Renamed", value: "" }).isDirty,
    ).toBe(true);
  });
});

describe("SecretsSettings deletion preflight", () => {
  it("keeps deletion disabled while checking and ignores a cancelled lookup", async () => {
    let resolveReferences!: (references: unknown[]) => void;
    mocks.listSecretReferences.mockReturnValue(
      new Promise((resolve) => {
        resolveReferences = resolve;
      }),
    );
    renderSettings();

    fireEvent.click(screen.getByRole("button", { name: DELETE_BUTTON }));
    const popover = await screen.findByTestId(CONFIRM_POPOVER_TEST_ID);
    expect(within(popover).getByText("Checking where this secret is used…")).toBeTruthy();
    expect((within(popover).getByTestId(CONFIRM_TEST_ID) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.click(within(popover).getByRole("button", { name: "Cancel" }));
    resolveReferences([{ kind: "repository", id: "repo-1", name: "App", key: "DEPLOY_TOKEN" }]);

    await waitFor(() => expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull());
    expect(screen.queryByTestId("secret-delete-conflict-dialog")).toBeNull();
    expect(mocks.deleteSecret).not.toHaveBeenCalled();
  });

  it("shows references before deletion and does not offer an unsafe delete", async () => {
    mocks.listSecretReferences.mockResolvedValue([
      { kind: "agent_profile", id: "profile-1", name: "Claude review", key: "MY_TOKEN" },
    ]);
    renderSettings();

    fireEvent.click(screen.getByRole("button", { name: DELETE_BUTTON }));

    const dialog = await screen.findByTestId("secret-delete-conflict-dialog");
    expect(within(dialog).getByText(/Claude review/)).toBeTruthy();
    expect(within(dialog).getByText(/MY_TOKEN/)).toBeTruthy();
    expect(within(dialog).queryByTestId(CONFIRM_TEST_ID)).toBeNull();
    expect(mocks.deleteSecret).not.toHaveBeenCalled();
  });

  it("shows fresh references when deletion loses a race", async () => {
    mocks.deleteSecret.mockRejectedValue(
      new ApiError("private server details", 409, {
        code: "secret_in_use",
        references: [
          { kind: "agent_profile", id: "profile-1", name: "Claude review", key: "MY_TOKEN" },
        ],
      }),
    );
    renderSettings();
    await openDeleteConfirmation();
    fireEvent.click(
      within(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).getByTestId(CONFIRM_TEST_ID),
    );
    const dialog = await screen.findByTestId("secret-delete-conflict-dialog");
    expect(within(dialog).getByText(/Claude review/)).toBeTruthy();
    expect(mocks.removeSecret).not.toHaveBeenCalled();
    expect(document.body.textContent).not.toContain("private server details");
  });
});

describe("SecretsSettings confirmed deletion", () => {
  it("cancels locally without dispatching deletion", async () => {
    renderSettings();

    const popover = await openDeleteConfirmation();
    fireEvent.click(within(popover).getByRole("button", { name: "Cancel" }));

    expect(mocks.deleteSecret).not.toHaveBeenCalled();
    expect(screen.queryByTestId(CONFIRM_POPOVER_TEST_ID)).toBeNull();
  });

  it("closes before dispatching exactly one delete and removes on success", async () => {
    let confirmationClosed = false;
    mocks.deleteSecret.mockImplementation(async () => {
      confirmationClosed = screen.queryByTestId(CONFIRM_POPOVER_TEST_ID) === null;
    });
    renderSettings();

    await openDeleteConfirmation();
    fireEvent.click(
      within(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).getByTestId(CONFIRM_TEST_ID),
    );

    await waitFor(() => expect(mocks.deleteSecret).toHaveBeenCalledTimes(1));
    expect(confirmationClosed).toBe(true);
    await waitFor(() => expect(mocks.removeSecret).toHaveBeenCalledWith(secret.id));
  });

  it("keeps row actions busy until deletion settles", async () => {
    let resolveDelete!: () => void;
    mocks.deleteSecret.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveDelete = resolve;
      }),
    );
    renderSettings();

    await openDeleteConfirmation();
    fireEvent.click(
      within(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).getByTestId(CONFIRM_TEST_ID),
    );

    await waitFor(() => expect(mocks.deleteSecret).toHaveBeenCalledTimes(1));
    expect(
      (screen.getByRole("button", { name: DELETE_BUTTON }) as HTMLButtonElement).disabled,
    ).toBe(true);
    resolveDelete();
    await waitFor(() => expect(mocks.removeSecret).toHaveBeenCalledWith(secret.id));
  });

  it("keeps the secret and reports a localized failure when deletion fails", async () => {
    mocks.deleteSecret.mockRejectedValue(new Error("server details must stay out of copy"));
    renderSettings();

    await openDeleteConfirmation();
    fireEvent.click(
      within(screen.getByTestId(CONFIRM_POPOVER_TEST_ID)).getByTestId(CONFIRM_TEST_ID),
    );

    await waitFor(() =>
      expect(mocks.toast).toHaveBeenCalledWith({
        description: "Couldn't delete secret",
        variant: "error",
      }),
    );
    expect(screen.getByText(secret.name)).not.toBeNull();
    expect(
      (screen.getByRole("button", { name: DELETE_BUTTON }) as HTMLButtonElement).disabled,
    ).toBe(false);
    expect(mocks.removeSecret).not.toHaveBeenCalled();
    expect(JSON.stringify(mocks.toast.mock.calls)).not.toContain(
      "server details must stay out of copy",
    );
  });
});
