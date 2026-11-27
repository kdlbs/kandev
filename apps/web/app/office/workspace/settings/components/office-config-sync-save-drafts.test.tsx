import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
  type SettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import { fetchJson, type ApiRequestOptions } from "@/lib/api/client";
import { clearNavigationBlockerForTests, requestNavigation } from "@/lib/routing/navigation-guard";
import type {
  OfficeConfigSyncConfig,
  OfficeConfigSyncSetConfigRequest,
} from "@/lib/types/office-config-sync";
import { OfficeConfigSyncSection } from "./office-config-sync-section";

vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: vi.fn(),
}));

const SUBMITTED_DIRECTORY = "submitted-directory";
const NEWER_DIRECTORY = "newer-directory";
const SAVED_RAW_OWNER = "raw-owner";
const CANONICAL_OWNER = "canonical-owner";
const OWNER_LABEL = "Repository owner";
const NAME_LABEL = "Repository name";
const PROJECT_LABEL = "Project path";
const PADDED_OWNER = " canonical-owner ";
const RAW_OWNER = " raw-owner ";
const WORKSPACE_ID = "office-save-draft-workspace";
const CONFIG_URL = `/api/v1/office/workspaces/${WORKSPACE_ID}/config-sync/config`;
const BASELINE: OfficeConfigSyncConfig = {
  workspace_id: WORKSPACE_ID,
  provider: "github",
  repo_owner: "example-team",
  repo_name: "office-source",
  project_path: "",
  branch: "main",
  path: "original-directory",
  interval_seconds: 300,
  poll_enabled: true,
  last_ok: true,
  created_at: "2026-10-08T12:00:00Z",
  updated_at: "2026-10-08T12:00:00Z",
};

type PendingSave = {
  payload: OfficeConfigSyncSetConfigRequest;
  promise: Promise<OfficeConfigSyncConfig>;
  resolve: (config: OfficeConfigSyncConfig) => void;
  reject: (error: Error) => void;
  settled: boolean;
};

let saves: PendingSave[];
let coordinator: SettingsSaveCoordinator;
let loadedConfig: OfficeConfigSyncConfig;

function CoordinatorObserver() {
  coordinator = useSettingsSaveCoordinator();
  return null;
}

function pendingSave(payload: OfficeConfigSyncSetConfigRequest): PendingSave {
  let resolve!: PendingSave["resolve"];
  let reject!: PendingSave["reject"];
  const promise = new Promise<OfficeConfigSyncConfig>((fulfill, fail) => {
    resolve = fulfill;
    reject = fail;
  });
  return { payload, promise, resolve, reject, settled: false };
}

beforeEach(() => {
  saves = [];
  loadedConfig = { ...BASELINE };
  vi.mocked(fetchJson).mockReset();
  vi.mocked(fetchJson).mockImplementation(async <T,>(url: string, options?: ApiRequestOptions) => {
    if (url !== CONFIG_URL) return undefined as T;
    if (options?.init?.method !== "POST") return loadedConfig as T;
    const save = pendingSave(JSON.parse(String(options.init.body)));
    saves.push(save);
    return save.promise as Promise<T>;
  });
});

afterEach(async () => {
  await act(async () => {
    for (const save of saves) {
      if (!save.settled) save.resolve(BASELINE);
    }
    await Promise.allSettled(saves.map((save) => save.promise));
  });
  cleanup();
  clearNavigationBlockerForTests();
});

async function renderOffice(config = BASELINE) {
  loadedConfig = config;
  render(
    <TooltipProvider>
      <ToastProvider>
        <StateProvider initialState={{ workspaces: { items: [], activeId: WORKSPACE_ID } }}>
          <SettingsSaveProvider>
            <OfficeConfigSyncSection />
            <CoordinatorObserver />
          </SettingsSaveProvider>
        </StateProvider>
      </ToastProvider>
    </TooltipProvider>,
  );
  await waitFor(() => expect(input("Directory").value).toBe(config.path));
  fireEvent.click(screen.getByText("Edit configuration"));
  expect(input("Directory").closest("details")?.open).toBe(true);
  expect(coordinator.hasDirty).toBe(false);
}

function input(label: string): HTMLInputElement {
  return screen.getByLabelText(label) as HTMLInputElement;
}

function edit(label: string, value: string) {
  const field = input(label);
  expect(field.disabled).toBe(false);
  fireEvent.change(field, { target: { value } });
}

async function startSave(buttonName = "Save changes") {
  const previousCount = saves.length;
  fireEvent.click(screen.getByRole("button", { name: buttonName }));
  await waitFor(() => expect(saves).toHaveLength(previousCount + 1));
  expect(coordinator.status).toBe("saving");
  return saves[previousCount];
}

async function acknowledge(save: PendingSave, config: OfficeConfigSyncConfig) {
  await act(async () => {
    save.settled = true;
    save.resolve(config);
    await save.promise;
  });
}

function expectDirty(dirty: boolean) {
  expect(coordinator.hasDirty).toBe(dirty);
  expect(coordinator.contributorStates).toContainEqual({
    id: "office-config-sync",
    isDirty: dirty,
    invalid: false,
    saveFailed: false,
  });
}

function savedResponse(save: PendingSave): OfficeConfigSyncConfig {
  return { ...BASELINE, repo_owner: "", repo_name: "", ...save.payload };
}

function chooseProvider(provider: "GitHub" | "GitLab") {
  const tab = screen.getByRole("tab", { name: provider });
  expect((tab as HTMLButtonElement).disabled).toBe(false);
  fireEvent.mouseDown(tab, { button: 0, ctrlKey: false });
}

async function resetDraft() {
  const count = saves.length;
  fireEvent.click(screen.getByRole("button", { name: "Reset" }));
  await waitFor(() => expect(coordinator.hasDirty).toBe(false));
  expect(saves).toHaveLength(count);
}

describe("Office configuration save drafts", () => {
  // @covers AC-UI-SETTINGS-MANUAL-SAVE-001.4
  it("retains a newer directory draft and dirty contributor after an older acknowledgment", async () => {
    await renderOffice();
    edit("Directory", SUBMITTED_DIRECTORY);
    const save = await startSave();
    expect(save.payload.path).toBe(SUBMITTED_DIRECTORY);

    edit("Directory", "newer-unsaved");
    await acknowledge(save, { ...BASELINE, path: SUBMITTED_DIRECTORY });

    expect.soft(input("Directory").value).toBe("newer-unsaved");
    expect.soft(coordinator.hasDirty).toBe(true);
    expect.soft(coordinator.contributorStates[0]?.isDirty).toBe(true);
  });

  it("saves the retained draft on the next explicit Save", async () => {
    await renderOffice();
    edit("Directory", "first-save");
    const first = await startSave();
    edit("Directory", "second-save");
    await acknowledge(first, savedResponse(first));
    expectDirty(true);
    expect(saves).toHaveLength(1);

    const second = await startSave();
    expect(first.payload).toEqual({
      provider: "github",
      repo_owner: "example-team",
      repo_name: "office-source",
      branch: "main",
      path: "first-save",
      interval_seconds: 300,
      poll_enabled: true,
    });
    expect(second.payload).toEqual({ ...first.payload, path: "second-save" });
    await acknowledge(second, savedResponse(second));
    expect(input("Directory").value).toBe("second-save");
    expectDirty(false);
    expect(saves).toHaveLength(2);
  });

  it("resets newer edits to the acknowledged configuration without posting", async () => {
    await renderOffice();
    edit("Directory", "acknowledged-directory");
    edit("Branch", "release");
    edit(OWNER_LABEL, "acknowledged-owner");
    const save = await startSave();
    edit("Directory", "unsaved-directory");
    edit("Branch", "unsaved-branch");
    edit(OWNER_LABEL, "unsaved-owner");
    await acknowledge(save, savedResponse(save));
    expectDirty(true);

    await resetDraft();
    expect(input("Directory").value).toBe("acknowledged-directory");
    expect(input("Branch").value).toBe("release");
    expect(input(OWNER_LABEL).value).toBe("acknowledged-owner");
    expect(input(NAME_LABEL).value).toBe("office-source");
    expect(input("Sync interval in seconds").value).toBe("300");
    expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("true");
    expectDirty(false);
  });
});

describe("Office configuration scalar drafts", () => {
  it.each([
    [OWNER_LABEL, "next-owner"],
    [NAME_LABEL, "next-repository"],
    ["Branch", "next-branch"],
    ["Directory", "next-directory"],
    ["Sync interval in seconds", "600"],
  ])("preserves a newer edit to %s during saving", async (label, value) => {
    await renderOffice();
    edit("Directory", SUBMITTED_DIRECTORY);
    const save = await startSave();
    edit(label, value);
    await acknowledge(save, savedResponse(save));
    expect(input(label).value).toBe(value);
    expectDirty(true);
  });

  it("preserves a newer polling switch edit during saving", async () => {
    await renderOffice();
    edit("Directory", SUBMITTED_DIRECTORY);
    const save = await startSave();
    fireEvent.click(screen.getByRole("switch"));
    await acknowledge(save, savedResponse(save));
    expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("false");
    expectDirty(true);
    await resetDraft();
    expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("true");
  });
});

describe("Office configuration provider drafts", () => {
  // @covers AC-OFFICE-CONFIG-SYNC-006.1, AC-OFFICE-CONFIG-SYNC-006.2
  it.each(["github", "gitlab"] as const)(
    "retains provider changes while saving %s and uses the new provider on the next Save",
    async (provider) => {
      const config: OfficeConfigSyncConfig =
        provider === "github"
          ? BASELINE
          : { ...BASELINE, provider, repo_owner: "", repo_name: "", project_path: "old/project" };
      await renderOffice(config);
      edit("Directory", SUBMITTED_DIRECTORY);
      const first = await startSave();

      if (provider === "github") {
        chooseProvider("GitLab");
        expect(input(PROJECT_LABEL).value).toBe("");
        edit(PROJECT_LABEL, "new/project");
      } else {
        chooseProvider("GitHub");
        expect(input(OWNER_LABEL).value).toBe("");
        expect(input(NAME_LABEL).value).toBe("");
        edit(OWNER_LABEL, "new-owner");
        edit(NAME_LABEL, "new-repository");
      }
      await acknowledge(first, savedResponse(first));
      expectDirty(true);

      const second = await startSave();
      if (provider === "github") {
        expect(second.payload.provider).toBe("gitlab");
        expect(second.payload.project_path).toBe("new/project");
        expect(second.payload).not.toHaveProperty("repo_owner");
        expect(second.payload).not.toHaveProperty("repo_name");
      } else {
        expect(second.payload.provider).toBe("github");
        expect(second.payload.repo_owner).toBe("new-owner");
        expect(second.payload.repo_name).toBe("new-repository");
        expect(second.payload).not.toHaveProperty("project_path");
      }
      await acknowledge(second, savedResponse(second));
      expectDirty(false);

      chooseProvider(provider === "github" ? "GitHub" : "GitLab");
      await resetDraft();
      expect(screen.getByRole("tab", { selected: true }).textContent).toBe(
        provider === "github" ? "GitLab" : "GitHub",
      );
      expect(saves).toHaveLength(2);
    },
  );

  it("preserves a newer GitLab project path during saving", async () => {
    await renderOffice({
      ...BASELINE,
      provider: "gitlab",
      repo_owner: "",
      repo_name: "",
      project_path: "group/original",
    });
    edit("Directory", SUBMITTED_DIRECTORY);
    const save = await startSave();
    edit(PROJECT_LABEL, "group/newer");
    await acknowledge(save, savedResponse(save));
    expect(input(PROJECT_LABEL).value).toBe("group/newer");
    expectDirty(true);
  });
});

describe("Office configuration normalization and baselines", () => {
  it("normalizes an unchanged submitted snapshot and becomes clean", async () => {
    await renderOffice();
    edit(OWNER_LABEL, PADDED_OWNER);
    edit(NAME_LABEL, " canonical-repo ");
    edit("Branch", " ");
    edit("Directory", "");
    const save = await startSave();
    expect(save.payload).toMatchObject({
      repo_owner: CANONICAL_OWNER,
      repo_name: "canonical-repo",
      branch: "",
      path: "",
    });
    await acknowledge(save, { ...savedResponse(save), branch: "main" });
    expect(input(OWNER_LABEL).value).toBe(CANONICAL_OWNER);
    expect(input(NAME_LABEL).value).toBe("canonical-repo");
    expect(input("Branch").value).toBe("main");
    expect(input("Directory").value).toBe("");
    expectDirty(false);
    expect(coordinator.status).toBe("saved");
    expect(saves).toHaveLength(1);
  });

  it("does not replace the submitted draft when another field changes before acknowledgment", async () => {
    await renderOffice();
    edit(OWNER_LABEL, RAW_OWNER);
    edit("Branch", " raw-branch ");
    const save = await startSave();
    edit("Directory", NEWER_DIRECTORY);
    await acknowledge(save, savedResponse(save));
    expect(input(OWNER_LABEL).value).toBe(RAW_OWNER);
    expect(input("Branch").value).toBe(" raw-branch ");
    expect(input("Directory").value).toBe(NEWER_DIRECTORY);
    expectDirty(true);
    await resetDraft();
    expect(input(OWNER_LABEL).value).toBe(SAVED_RAW_OWNER);
    expect(input("Branch").value).toBe("raw-branch");
    expect(input("Directory").value).toBe(BASELINE.path);
  });

  it("retains edits reverted to the previous baseline until another Save or Reset", async () => {
    await renderOffice();
    edit("Directory", SUBMITTED_DIRECTORY);
    const save = await startSave();
    edit("Directory", BASELINE.path);
    expectDirty(false);
    await acknowledge(save, savedResponse(save));
    expect(input("Directory").value).toBe(BASELINE.path);
    expectDirty(true);
    await resetDraft();
    expect(input("Directory").value).toBe(SUBMITTED_DIRECTORY);
  });

  it("accepts an edit reverted exactly to the submitted snapshot", async () => {
    await renderOffice();
    edit(OWNER_LABEL, PADDED_OWNER);
    const save = await startSave();
    edit(OWNER_LABEL, "temporary-owner");
    edit(OWNER_LABEL, PADDED_OWNER);
    await acknowledge(save, savedResponse(save));
    expect(input(OWNER_LABEL).value).toBe(CANONICAL_OWNER);
    expectDirty(false);
  });
});

describe("Office configuration save failures", () => {
  it("retains newer edits and the previous baseline after a rejected save", async () => {
    await renderOffice();
    edit("Directory", SUBMITTED_DIRECTORY);
    const failed = await startSave();
    edit("Directory", NEWER_DIRECTORY);
    await act(async () => {
      failed.settled = true;
      failed.reject(new Error("config transport rejected"));
      await Promise.allSettled([failed.promise]);
    });
    expect(input("Directory").value).toBe(NEWER_DIRECTORY);
    expect(coordinator.hasDirty).toBe(true);
    expect(coordinator.status).toBe("error");
    expect(coordinator.contributorStates[0]?.saveFailed).toBe(true);
    await resetDraft();
    expect(input("Directory").value).toBe(BASELINE.path);

    edit("Directory", "retry-directory");
    const retry = await startSave();
    expect(retry.payload.path).toBe("retry-directory");
    await acknowledge(retry, savedResponse(retry));
    expectDirty(false);
  });
});

describe("Office configuration save and leave", () => {
  it("reports canLeave false from the real coordinator after a newer draft is acknowledged", async () => {
    await renderOffice();
    edit("Directory", SUBMITTED_DIRECTORY);
    let result!: Awaited<ReturnType<SettingsSaveCoordinator["saveAll"]>>;
    let saving!: ReturnType<SettingsSaveCoordinator["saveAll"]>;
    act(() => {
      saving = coordinator.saveAll();
    });
    expect(saves).toHaveLength(1);
    edit("Branch", "newer-branch");
    await act(async () => {
      saves[0].settled = true;
      saves[0].resolve(savedResponse(saves[0]));
      result = await saving;
    });
    expect(result.canLeave).toBe(false);
    expect(result.failedIds.size).toBe(0);
    expectDirty(true);
  });

  // @covers AC-UI-SETTINGS-MANUAL-SAVE-001.4 and its navigation policy
  it("refuses save-and-leave when the draft changes before acknowledgment", async () => {
    await renderOffice();
    edit("Directory", SUBMITTED_DIRECTORY);
    const proceed = vi.fn();
    act(() => requestNavigation(proceed));
    const save = await startSave("Save and leave");
    edit("Branch", "newer-branch");
    await acknowledge(save, savedResponse(save));
    expectDirty(true);
    expect(input("Branch").value).toBe("newer-branch");
    expect(proceed).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Continue editing" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Continue editing" }));
    expectDirty(true);
  });

  it("continues save-and-leave after an unchanged normalized save", async () => {
    await renderOffice();
    edit(OWNER_LABEL, PADDED_OWNER);
    const proceed = vi.fn();
    act(() => requestNavigation(proceed));
    const save = await startSave("Save and leave");
    expect(proceed).not.toHaveBeenCalled();
    await acknowledge(save, savedResponse(save));
    expect(input(OWNER_LABEL).value).toBe(CANONICAL_OWNER);
    expectDirty(false);
    expect(proceed).toHaveBeenCalledTimes(1);
  });
});
