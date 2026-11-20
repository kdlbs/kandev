import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

import type { CustomPrompt } from "@/lib/types/http";
import { SettingsSaveProvider } from "./settings-save-provider";
import { PromptsSettings } from "./prompts-settings";

const mocks = vi.hoisted(() => ({
  prompts: [] as CustomPrompt[],
  promptEditor: vi.fn(),
  saveVersions: vi.fn(),
}));

vi.mock("@/lib/api/domains/prompt-sync-api", () => ({
  savePromptVersions: (...args: unknown[]) => mocks.saveVersions(...args),
}));
vi.mock("@/components/settings/prompt-sync-section", () => ({ PromptSyncSection: () => null }));
vi.mock("@/hooks/domains/settings/use-custom-prompts", () => ({
  useCustomPrompts: () => ({ loaded: true }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => ({ prompts: { items: mocks.prompts } }) }),
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ prompts: { items: mocks.prompts }, setPrompts: vi.fn() }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: true, isMobile: false }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    mocks.promptEditor(props);
    return (
      <textarea
        data-testid={props.testId as string}
        value={props.value as string}
        readOnly={props.readOnly as boolean}
        onChange={(event) => (props.onChange as (value: string) => void)(event.target.value)}
      />
    );
  },
}));
vi.mock("@/lib/api", () => ({
  createPrompt: vi.fn(),
  deletePrompt: vi.fn(),
  updatePrompt: vi.fn(),
}));

const promptRowTestId = "prompt-list-item";
const promptNameAttribute = "data-prompt-name";
const promptEditButtonTestId = "prompt-edit-button";
const promptDeleteButtonTestId = "prompt-delete-button";
const viewButtonTestId = "prompt-view-button";
const viewContentTestId = "prompt-view-content";
const saveChangesLabel = "Save changes";

const existingPrompt: CustomPrompt = {
  id: "prompt-1",
  name: "Review",
  content: "Review this change",
  builtin: false,
  created_at: "2026-08-20T10:00:00Z",
  updated_at: "2026-08-20T10:00:00Z",
};

beforeEach(() => {
  vi.clearAllMocks();
  mocks.prompts = [];
});

afterEach(cleanup);

// @covers AC-INTEGRATIONS-PROMPT-SYNC-004.3
describe("synced prompts", () => {
  it("show a synced badge and no edit or delete controls", () => {
    mocks.prompts = [
      {
        ...existingPrompt,
        id: "synced-1",
        name: "principles",
        source: "sync",
        source_path: "p/principles.md",
      },
      { ...existingPrompt, id: "manual-1", name: "mine", source: "manual" },
    ];
    render(
      <TooltipProvider>
        <SettingsSaveProvider>
          <PromptsSettings />
        </SettingsSaveProvider>
      </TooltipProvider>,
    );
    const rows = screen.getAllByTestId(promptRowTestId);
    const synced = rows.find((row) => row.getAttribute(promptNameAttribute) === "principles")!;
    const manual = rows.find((row) => row.getAttribute(promptNameAttribute) === "mine")!;
    expect(within(synced).getByTestId("prompt-synced-badge")).toBeTruthy();
    expect(within(synced).queryByTestId(promptEditButtonTestId)).toBeNull();
    expect(within(synced).queryByTestId(promptDeleteButtonTestId)).toBeNull();
    expect(within(manual).queryByTestId("prompt-synced-badge")).toBeNull();
    expect(within(manual).getByTestId(promptEditButtonTestId)).toBeTruthy();
    expect(within(manual).queryByTestId(viewButtonTestId)).toBeNull();
  });

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-004.3
  it("open the full content read-only from a View button", () => {
    const fullContent = "Line one of the synced rule.\nLine two only visible when viewing.";
    mocks.prompts = [
      {
        ...existingPrompt,
        id: "synced-1",
        name: "principles",
        content: fullContent,
        source: "sync",
        source_path: "p/principles.md",
      },
    ];
    render(
      <TooltipProvider>
        <SettingsSaveProvider>
          <PromptsSettings />
        </SettingsSaveProvider>
      </TooltipProvider>,
    );
    const row = screen.getByTestId(promptRowTestId);
    const view = within(row).getByTestId(viewButtonTestId);
    expect(view.getAttribute("aria-expanded")).toBe("false");
    expect(within(row).queryByTestId(viewContentTestId)).toBeNull();

    fireEvent.click(view);
    expect(view.getAttribute("aria-expanded")).toBe("true");
    const content = within(row).getByTestId(viewContentTestId) as HTMLTextAreaElement;
    expect(content.value).toBe(fullContent);
    expect(mocks.promptEditor).toHaveBeenCalledWith(
      expect.objectContaining({ testId: viewContentTestId, readOnly: true }),
    );
    expect(within(row).getByTestId("prompt-view-source-note").textContent).toContain(
      "p/principles.md",
    );
    expect(within(row).queryByRole("button", { name: saveChangesLabel })).toBeNull();

    fireEvent.click(view);
    expect(within(row).queryByTestId(viewContentTestId)).toBeNull();
  });
});

function renderPrompts() {
  render(
    <TooltipProvider>
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>
    </TooltipProvider>,
  );
}

const switchTestId = "prompt-override-switch";

// @covers AC-INTEGRATIONS-PROMPT-SYNC-007.1 AC-INTEGRATIONS-PROMPT-SYNC-007.2 AC-INTEGRATIONS-PROMPT-SYNC-009.2
describe("pending Local override switch", () => {
  const contentInputTestId = "prompt-content-input";
  const syncedHook: CustomPrompt = {
    ...existingPrompt,
    id: "s1",
    name: "hook",
    content: "Repository text",
    alternate_content: "My text",
    source: "sync",
    source_path: "p/hook.md",
  };

  it("opens a synced prompt for editing when its switch is turned on, applying nothing yet", () => {
    mocks.prompts = [syncedHook];
    renderPrompts();
    const toggle = screen.getByRole("switch", { name: "Local override" });
    expect(toggle.getAttribute("data-testid")).toBe(switchTestId);
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(toggle);

    const row = screen.getByTestId(promptRowTestId);
    expect(within(row).getByTestId(switchTestId).getAttribute("aria-checked")).toBe("true");
    const editor = within(row).getByTestId(contentInputTestId) as HTMLTextAreaElement;
    expect(editor.value).toBe("My text");
    expect(editor.readOnly).toBe(false);
    expect((within(row).getByTestId("prompt-name-input") as HTMLInputElement).disabled).toBe(true);
    expect(within(row).queryByTestId(promptDeleteButtonTestId)).toBeNull();
    expect(screen.getByRole("button", { name: saveChangesLabel })).toBeTruthy();
    expect(mocks.saveVersions).not.toHaveBeenCalled();
  });

  it("saves the switch and the edited local version in one write", async () => {
    mocks.prompts = [syncedHook];
    mocks.saveVersions.mockResolvedValue({ ...syncedHook, source: "override" });
    renderPrompts();
    fireEvent.click(screen.getByRole("switch", { name: "Local override" }));
    fireEvent.change(screen.getByTestId(contentInputTestId), { target: { value: "Edited" } });
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
    await waitFor(() => expect(mocks.saveVersions).toHaveBeenCalledOnce());
    expect(mocks.saveVersions.mock.calls[0]![0]).toBe("s1");
    expect(mocks.saveVersions.mock.calls[0]![1]).toEqual({
      use_local: true,
      local_content: "Edited",
      expected_updated_at: syncedHook.updated_at,
    });
  });

  it("shows the repository version read-only when an override's switch is turned off", async () => {
    mocks.prompts = [
      {
        ...syncedHook,
        id: "o1",
        content: "My text",
        alternate_content: "Repository text",
        source: "override",
      },
    ];
    mocks.saveVersions.mockResolvedValue(syncedHook);
    renderPrompts();
    const row = screen.getByTestId(promptRowTestId);
    expect(within(row).getByTestId("prompt-override-badge")).toBeTruthy();
    expect(within(row).getByTestId(promptEditButtonTestId)).toBeTruthy();
    expect(within(row).getByTestId(promptDeleteButtonTestId)).toBeTruthy();
    fireEvent.click(within(row).getByTestId(switchTestId));

    const editor = within(row).getByTestId(contentInputTestId) as HTMLTextAreaElement;
    expect(editor.value).toBe("Repository text");
    expect(editor.readOnly).toBe(true);
    expect(within(row).getByTestId("prompt-view-source-note").textContent).toContain("p/hook.md");
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
    await waitFor(() => expect(mocks.saveVersions).toHaveBeenCalledOnce());
    expect(mocks.saveVersions.mock.calls[0]![1]).toEqual({
      use_local: false,
      expected_updated_at: syncedHook.updated_at,
    });
  });

  it("restores the saved state on Reset", () => {
    mocks.prompts = [syncedHook];
    renderPrompts();
    fireEvent.click(screen.getByRole("switch", { name: "Local override" }));
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    const row = screen.getByTestId(promptRowTestId);
    expect(within(row).getByTestId(switchTestId).getAttribute("aria-checked")).toBe("false");
    expect(within(row).queryByTestId(contentInputTestId)).toBeNull();
    expect(mocks.saveVersions).not.toHaveBeenCalled();
  });
});

describe("prompt overrides", () => {
  // @covers AC-INTEGRATIONS-PROMPT-SYNC-007.1
  it("keeps the switch first among the trailing controls in every row state", () => {
    mocks.prompts = [
      { ...existingPrompt, id: "s1", name: "synced", source: "sync", source_path: "p/s.md" },
      { ...existingPrompt, id: "o1", name: "local", source: "override", source_path: "p/o.md" },
    ];
    renderPrompts();
    const rows = screen.getAllByTestId(promptRowTestId);
    for (const row of rows) {
      const trailing = within(row).getByTestId("prompt-row-trailing");
      expect(trailing.firstElementChild?.getAttribute("data-testid")).toBe(
        "prompt-override-switch-label",
      );
    }
  });

  // @covers AC-INTEGRATIONS-PROMPT-SYNC-008.5
  it("marks a conflicted override with a Conflict badge", () => {
    mocks.prompts = [
      {
        ...existingPrompt,
        id: "c1",
        name: "clash",
        source: "override",
        source_path: "p/c.md",
        conflict: true,
      },
      { ...existingPrompt, id: "o1", name: "plain", source: "override", source_path: "p/o.md" },
    ];
    renderPrompts();
    const [clash, plain] = screen.getAllByTestId(promptRowTestId);
    expect(within(clash!).getByTestId("prompt-conflict-badge").textContent).toBe("Conflict");
    expect(within(plain!).queryByTestId("prompt-conflict-badge")).toBeNull();
  });
});
