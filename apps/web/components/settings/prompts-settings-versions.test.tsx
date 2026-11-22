import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

import type { CustomPrompt } from "@/lib/types/http";
import { SettingsSaveProvider } from "./settings-save-provider";
import { PromptsSettings } from "./prompts-settings";

const mocks = vi.hoisted(() => ({
  prompts: [] as CustomPrompt[],
  promptEditor: vi.fn(),
  createPrompt: vi.fn(),
  saveVersions: vi.fn(),
  updatePrompt: vi.fn(),
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
  createPrompt: (...args: unknown[]) => mocks.createPrompt(...args),
  deletePrompt: vi.fn(),
  updatePrompt: (...args: unknown[]) => mocks.updatePrompt(...args),
}));

const rowTestId = "prompt-list-item";
const saveChangesLabel = "Save changes";
const viewTestId = "prompt-view-content";
const editorTestId = "prompt-content-input";
const editButtonTestId = "prompt-edit-button";
const viewButtonTestId = "prompt-view-button";
const unsavedTestId = "prompt-unsaved-badge";
const CHECKED = "aria-checked";
const EXPANDED = "aria-expanded";

const base: CustomPrompt = {
  id: "s1",
  name: "hook",
  content: "Repository text",
  alternate_content: "My text",
  builtin: false,
  source: "sync",
  source_path: "p/hook.md",
  created_at: "2026-08-20T10:00:00Z",
  updated_at: "2026-08-20T10:00:00Z",
};
const override: CustomPrompt = {
  ...base,
  id: "o1",
  name: "other",
  content: "My other text",
  alternate_content: "Repository other text",
  source: "override",
  source_path: "p/other.md",
};

beforeEach(() => {
  vi.clearAllMocks();
  mocks.prompts = [];
  mocks.saveVersions.mockImplementation(async (id: string) =>
    mocks.prompts.find((prompt) => prompt.id === id),
  );
  mocks.updatePrompt.mockImplementation(async (id: string) =>
    mocks.prompts.find((prompt) => prompt.id === id),
  );
});

afterEach(cleanup);

function renderPrompts() {
  render(
    <TooltipProvider>
      <SettingsSaveProvider>
        <PromptsSettings />
      </SettingsSaveProvider>
    </TooltipProvider>,
  );
}

function row(name: string) {
  return screen
    .getAllByTestId(rowTestId)
    .find((item) => item.getAttribute("data-prompt-name") === name)!;
}

function versionSwitch(name: string) {
  return screen.getByRole("switch", { name: `Use the repository version of @${name}` });
}

const manual: CustomPrompt = {
  ...base,
  id: "m1",
  name: "notes",
  content: "Notes text",
  alternate_content: undefined,
  source: "manual",
  source_path: undefined,
};
const manual2: CustomPrompt = { ...manual, id: "m2", name: "plans", content: "Plans text" };

function inRow(name: string) {
  return within(row(name));
}

function editor(name: string) {
  return inRow(name).getByTestId(editorTestId) as HTMLTextAreaElement;
}

function save() {
  fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
}

// @covers AC-INTEGRATIONS-PROMPT-SYNC-007.1 AC-INTEGRATIONS-PROMPT-SYNC-007.2 AC-INTEGRATIONS-PROMPT-SYNC-009.2
describe("pending version switches", () => {
  it("uses the review step's Local | Repository control, on for the repository", () => {
    mocks.prompts = [base, override];
    renderPrompts();
    expect(versionSwitch("hook").getAttribute(CHECKED)).toBe("true");
    expect(versionSwitch("other").getAttribute(CHECKED)).toBe("false");
    expect(inRow("hook").getByText("Local")).toBeTruthy();
    expect(inRow("hook").getByText("Repository")).toBeTruthy();
  });

  it("opens Repository rows read-only with the eye and Local rows editable with Edit", () => {
    mocks.prompts = [base, override];
    renderPrompts();
    expect(inRow("hook").queryByTestId(editButtonTestId)).toBeNull();
    expect(inRow("other").queryByTestId(viewButtonTestId)).toBeNull();
    fireEvent.click(inRow("hook").getByTestId(viewButtonTestId));
    const view = inRow("hook").getByTestId(viewTestId) as HTMLTextAreaElement;
    expect(view.value).toBe("Repository text");
    expect(view.readOnly).toBe(true);
    fireEvent.click(inRow("other").getByTestId(editButtonTestId));
    expect(editor("other").value).toBe("My other text");
    expect(editor("other").readOnly).toBe(false);
    expect(inRow("other").queryByTestId("prompt-name-input")).toBeNull();
    fireEvent.click(inRow("other").getByTestId(editButtonTestId));
    expect(inRow("other").queryByTestId(editorTestId)).toBeNull();
  });

  it("keeps a collapsed row collapsed when switched, applying nothing yet", () => {
    mocks.prompts = [base];
    renderPrompts();
    fireEvent.click(versionSwitch("hook"));
    expect(versionSwitch("hook").getAttribute(CHECKED)).toBe("false");
    expect(inRow("hook").queryByTestId(editorTestId)).toBeNull();
    expect(inRow("hook").queryByTestId(viewTestId)).toBeNull();
    expect(inRow("hook").getByTestId(editButtonTestId).getAttribute(EXPANDED)).toBe("false");
    expect(inRow("hook").getByTestId(unsavedTestId)).toBeTruthy();
    expect(screen.getByRole("button", { name: saveChangesLabel })).toBeTruthy();
    expect(mocks.saveVersions).not.toHaveBeenCalled();
  });

  it("keeps an open row open when switched, editable at once on Local", () => {
    mocks.prompts = [base];
    renderPrompts();
    fireEvent.click(inRow("hook").getByTestId(viewButtonTestId));
    fireEvent.click(versionSwitch("hook"));
    expect(editor("hook").value).toBe("My text");
    expect(editor("hook").readOnly).toBe(false);
    expect(inRow("hook").getByTestId(editButtonTestId).getAttribute(EXPANDED)).toBe("true");
    fireEvent.change(editor("hook"), { target: { value: "Edited" } });
    fireEvent.click(versionSwitch("hook"));
    expect((inRow("hook").getByTestId(viewTestId) as HTMLTextAreaElement).value).toBe(
      "Repository text",
    );
    fireEvent.click(versionSwitch("hook"));
    expect(editor("hook").value).toBe("Edited");
  });

  it("saves several switched prompts with one Save", async () => {
    mocks.prompts = [base, override];
    renderPrompts();
    fireEvent.click(versionSwitch("hook"));
    fireEvent.click(versionSwitch("other"));
    save();
    await waitFor(() => expect(mocks.saveVersions).toHaveBeenCalledTimes(2));
    expect(mocks.saveVersions.mock.calls.map((call) => [call[0], call[1]])).toEqual([
      ["s1", { use_local: true }],
      ["o1", { use_local: false }],
    ]);
  });

  it("restores every pending switch and edit on Reset", () => {
    mocks.prompts = [base, override];
    renderPrompts();
    fireEvent.click(versionSwitch("hook"));
    fireEvent.click(inRow("other").getByTestId(editButtonTestId));
    fireEvent.change(editor("other"), { target: { value: "Changed" } });
    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(versionSwitch("hook").getAttribute(CHECKED)).toBe("true");
    expect(screen.queryByTestId(unsavedTestId)).toBeNull();
    expect(screen.queryByTestId(editorTestId)).toBeNull();
    expect(mocks.saveVersions).not.toHaveBeenCalled();
  });
});

// @covers AC-INTEGRATIONS-PROMPT-SYNC-009.2
describe("editing several prompts before one Save", () => {
  it("marks each edited prompt unsaved and saves every edit together", async () => {
    mocks.prompts = [base, override, manual];
    renderPrompts();
    fireEvent.click(versionSwitch("hook"));
    fireEvent.click(inRow("hook").getByTestId(editButtonTestId));
    fireEvent.change(editor("hook"), { target: { value: "Hook edit" } });
    fireEvent.click(inRow("other").getByTestId(editButtonTestId));
    fireEvent.change(editor("other"), { target: { value: "Other edit" } });
    fireEvent.click(inRow("notes").getByTestId(editButtonTestId));
    fireEvent.change(editor("notes"), { target: { value: "Notes edit" } });
    fireEvent.click(inRow("notes").getByTestId(editButtonTestId));
    expect(inRow("notes").queryByTestId(editorTestId)).toBeNull();
    for (const name of ["hook", "other", "notes"]) {
      expect(inRow(name).getByTestId(unsavedTestId)).toBeTruthy();
    }
    save();
    await waitFor(() => expect(mocks.updatePrompt).toHaveBeenCalledOnce());
    expect(mocks.saveVersions.mock.calls.map((call) => [call[0], call[1]])).toEqual([
      ["s1", { use_local: true, local_content: "Hook edit", expected_updated_at: base.updated_at }],
      [
        "o1",
        { use_local: true, local_content: "Other edit", expected_updated_at: base.updated_at },
      ],
    ]);
    expect(mocks.updatePrompt.mock.calls[0]![0]).toBe("m1");
    expect(mocks.updatePrompt.mock.calls[0]![1]).toMatchObject({
      name: "notes",
      content: "Notes edit",
    });
  });

  it("keeps two manual prompts open for editing at once", () => {
    mocks.prompts = [manual, manual2];
    renderPrompts();
    fireEvent.click(inRow("notes").getByTestId(editButtonTestId));
    fireEvent.click(inRow("plans").getByTestId(editButtonTestId));
    expect(editor("notes").value).toBe("Notes text");
    expect(editor("plans").value).toBe("Plans text");
    expect(inRow("plans").queryByTestId(unsavedTestId)).toBeNull();
  });

  it("shows an override whose file left the repository without a version switch", () => {
    mocks.prompts = [{ ...override, alternate_content: "" }];
    renderPrompts();
    const item = row("other");
    expect(within(item).queryByRole("switch")).toBeNull();
    expect(within(item).getByTestId("prompt-repository-removed").textContent).toBe(
      "Removed from repository",
    );
    fireEvent.click(within(item).getByTestId(editButtonTestId));
    expect(within(item).getByTestId(editorTestId)).toHaveProperty("value", "My other text");
  });

  it("keeps the text of a prompt removed while edited and saves it as a new prompt", async () => {
    const manual: CustomPrompt = {
      ...base,
      id: "m1",
      name: "notes",
      source: "manual",
      content: "Old",
    };
    mocks.prompts = [manual];
    mocks.createPrompt.mockImplementation(async (body: { name: string; content: string }) => ({
      ...manual,
      id: "m2",
      ...body,
    }));
    const view = render(
      <TooltipProvider>
        <SettingsSaveProvider>
          <PromptsSettings />
        </SettingsSaveProvider>
      </TooltipProvider>,
    );
    fireEvent.click(within(row("notes")).getByTestId(editButtonTestId));
    fireEvent.change(within(row("notes")).getByTestId(editorTestId), { target: { value: "Mine" } });
    mocks.prompts = [];
    view.rerender(
      <TooltipProvider>
        <SettingsSaveProvider>
          <PromptsSettings />
        </SettingsSaveProvider>
      </TooltipProvider>,
    );
    expect(screen.getByTestId("prompt-orphaned-drafts-notice").textContent).toContain("@notes");
    fireEvent.click(screen.getByRole("button", { name: saveChangesLabel }));
    await waitFor(() =>
      expect(mocks.createPrompt).toHaveBeenCalledWith(
        { name: "notes", content: "Mine" },
        expect.anything(),
      ),
    );
  });
});
