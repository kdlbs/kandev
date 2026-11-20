import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

import type { CustomPrompt } from "@/lib/types/http";
import { SettingsSaveProvider } from "./settings-save-provider";
import { PromptsSettings } from "./prompts-settings";

const mocks = vi.hoisted(() => ({
  prompts: [] as CustomPrompt[],
  promptEditor: vi.fn(),
  override: vi.fn(),
  revert: vi.fn(),
}));

vi.mock("@/hooks/domains/settings/use-prompt-override", () => ({
  usePromptOverride: () => ({ pendingId: null, override: mocks.override, revert: mocks.revert }),
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
    return <textarea data-testid={props.testId as string} value={props.value as string} readOnly />;
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

// @covers AC-INTEGRATIONS-PROMPT-SYNC-007.1 AC-INTEGRATIONS-PROMPT-SYNC-007.2
describe("prompt overrides", () => {
  function renderPrompts() {
    render(
      <TooltipProvider>
        <SettingsSaveProvider>
          <PromptsSettings />
        </SettingsSaveProvider>
      </TooltipProvider>,
    );
  }

  it("offers Override on a synced prompt", () => {
    mocks.prompts = [
      { ...existingPrompt, id: "s1", name: "hook", source: "sync", source_path: "p/hook.md" },
    ];
    renderPrompts();
    const row = screen.getByTestId(promptRowTestId);
    fireEvent.click(within(row).getByTestId("prompt-override-button"));
    expect(mocks.override).toHaveBeenCalledWith("s1");
  });

  it("shows an Overridden badge, Revert, and normal edit controls on an override", () => {
    mocks.prompts = [
      { ...existingPrompt, id: "o1", name: "hook", source: "override", source_path: "p/hook.md" },
    ];
    renderPrompts();
    const row = screen.getByTestId(promptRowTestId);
    expect(within(row).getByTestId("prompt-override-badge")).toBeTruthy();
    expect(within(row).getByTestId(promptEditButtonTestId)).toBeTruthy();
    expect(within(row).getByTestId(promptDeleteButtonTestId)).toBeTruthy();
    expect(within(row).queryByTestId("prompt-synced-badge")).toBeNull();
    fireEvent.click(within(row).getByTestId("prompt-revert-button"));
    expect(mocks.revert).toHaveBeenCalledWith("o1");
  });
});
