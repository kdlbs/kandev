import { useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import {
  fetchGitHubWorkspaceSettings,
  updateGitHubWorkspaceSettings,
} from "@/lib/api/domains/github-api";
import { SavePresetDialog } from "./save-preset-dialog";
import { useSavedPresetActions } from "./use-saved-preset-actions";
import { __resetSnapshotForTests, useSavedPresets, type SavedPreset } from "./use-saved-presets";

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchGitHubWorkspaceSettings: vi.fn(),
  updateGitHubWorkspaceSettings: vi.fn(),
}));

const workspaceId = "workspace-capability";
const query = "is:open author:@me";
const repo = "example/project";
const uuidPattern = /^p_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const noop = () => {};

function Harness() {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState("");
  const store = useSavedPresets(workspaceId);
  const actions = useSavedPresetActions({
    workspaceId,
    selection: { kind: "pr", source: "preset", id: "" },
    customQuery: query,
    resolvedPrPresets: [],
    resolvedIssuePresets: [],
    setProgrammaticSelection: (selection) => setSelected(selection.id),
    setQueryImmediate: noop,
    setRepoFilter: noop,
    savedPresetStore: store,
    markSearchInteracted: noop,
  });
  return (
    <>
      <button disabled={store.loading} onClick={() => setOpen(true)}>
        Open save
      </button>
      <output aria-label="Saved views">
        {actions.savedPresets.map((preset) => preset.label).join(", ")}
      </output>
      <output aria-label="Selected view">{selected}</output>
      <SavePresetDialog
        open={open}
        onOpenChange={setOpen}
        kind="pr"
        customQuery={query}
        repoFilter={repo}
        repoOptions={[repo]}
        suggestedLabel="Review queue"
        onSave={actions.onConfirmSave}
      />
    </>
  );
}

const settings = {
  workspace_id: workspaceId,
  github_host: "github.com",
  repo_scope_mode: "all",
  repo_scope_orgs: [],
  repo_scope_repos: [],
  saved_presets: [],
  default_query_presets: null,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
} as Awaited<ReturnType<typeof fetchGitHubWorkspaceSettings>>;

async function saveView(label: string) {
  fireEvent.click(screen.getByRole("button", { name: "Open save" }));
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: label } });
  const save = screen.getByRole("button", { name: "Save" });
  expect(save.hasAttribute("disabled")).toBe(false);
  fireEvent.click(save);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
}

// @covers AC-INTEGRATIONS-GITHUB-SAVED-QUERY-CAPABILITY-001.1
// @covers AC-INTEGRATIONS-GITHUB-SAVED-QUERY-CAPABILITY-001.2
describe("GitHub saved views with browser UUID capabilities", () => {
  beforeEach(() => {
    __resetSnapshotForTests();
    vi.mocked(fetchGitHubWorkspaceSettings).mockResolvedValue(settings);
    vi.mocked(updateGitHubWorkspaceSettings).mockResolvedValue(settings);
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.resetAllMocks();
  });

  it.each(["fallback", "randomUUID"] as const)(
    "saves and selects two views using %s",
    async (capability) => {
      const getRandomValues = globalThis.crypto.getRandomValues.bind(globalThis.crypto);
      const randomUUID = globalThis.crypto.randomUUID.bind(globalThis.crypto);
      vi.stubGlobal(
        "crypto",
        capability === "fallback" ? { getRandomValues } : { getRandomValues, randomUUID },
      );
      render(
        <ToastProvider>
          <Harness />
        </ToastProvider>,
      );
      await waitFor(() =>
        expect(screen.getByRole("button", { name: "Open save" }).hasAttribute("disabled")).toBe(
          false,
        ),
      );
      await saveView("Review queue");
      await saveView("Second queue");
      expect(updateGitHubWorkspaceSettings).toHaveBeenCalledTimes(2);
      const payload = vi.mocked(updateGitHubWorkspaceSettings).mock.calls[1][0];
      expect(payload.workspace_id).toBe(workspaceId);
      expect(payload.expected_host).toBe("github.com");
      expect(payload.saved_presets).toHaveLength(2);
      const [first, second] = payload.saved_presets as SavedPreset[];
      expect(first).toMatchObject({
        label: "Review queue",
        kind: "pr",
        customQuery: query,
        repoFilter: repo,
        isDefault: false,
      });
      expect(second).toMatchObject({
        label: "Second queue",
        kind: "pr",
        customQuery: query,
        repoFilter: repo,
        isDefault: false,
      });
      expect(first.id).toMatch(uuidPattern);
      expect(second.id).toMatch(uuidPattern);
      expect(first.id).not.toBe(second.id);
      expect(screen.getByLabelText("Saved views").textContent).toContain("Second queue");
      expect(screen.getByLabelText("Selected view").textContent).toBe(second.id);
    },
  );
});
