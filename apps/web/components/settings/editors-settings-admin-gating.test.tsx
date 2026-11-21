import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { EditorOption } from "@/lib/types/http";

const mocks = vi.hoisted(() => {
  const editorId = "custom-editor-1";
  const editorType = "custom";
  const editorName = "Team VS Code";
  const command = "code {file}";
  const addEditorButtonName = "Add custom editor";
  const editor: EditorOption = {
    id: editorId,
    type: editorType,
    name: editorName,
    kind: "custom_command",
    config: { command },
    enabled: true,
    installed: true,
  } as EditorOption;
  const state = {
    editors: [editor],
    setEditors: vi.fn(),
    currentUserSettings: {},
    markdownPreviewByDefault: true,
    baselineMarkdownPreviewByDefault: true,
    setMarkdownPreviewByDefault: vi.fn(),
    setBaselineMarkdownPreviewByDefault: vi.fn(),
    defaultEditorId: editor.id,
    setDefaultEditorId: vi.fn(),
    baselineDefaultId: editor.id,
    isAdding: false,
    setIsAdding: vi.fn(),
    editingId: null as string | null,
    setEditingId: vi.fn(),
    lspAutoStartLanguages: [] as string[],
    lspAutoInstallLanguages: [] as string[],
    baselineLspAutoStart: [] as string[],
    baselineLspAutoInstall: [] as string[],
    lspStatusLocation: "toolbar",
    baselineLspStatusLocation: "toolbar",
    lspConfigStrings: {} as Record<string, string>,
    setLspConfigStrings: vi.fn(),
    baselineLspConfigStrings: {} as Record<string, string>,
    lspConfigErrors: {} as Record<string, string>,
    setLspConfigErrors: vi.fn(),
    expandedConfigLang: null as string | null,
    setExpandedConfigLang: vi.fn(),
  };
  return { editorId, editorType, editorName, command, addEditorButtonName, isAdmin: true, state };
});

vi.mock("@/hooks/domains/auth/use-is-admin", () => ({
  useIsAdmin: () => mocks.isAdmin,
}));

vi.mock("@/components/settings/editors-settings-state", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/components/settings/editors-settings-state")>();
  return {
    ...actual,
    useEditorsSettingsState: () => mocks.state,
    useApplyEditors: () => vi.fn(),
    useEditorRequests: () => ({
      createRequest: { isLoading: false, status: "idle", run: vi.fn() },
      updateRequest: { isLoading: false, status: "idle", run: vi.fn() },
      deleteRequest: { isLoading: false, status: "idle", run: vi.fn() },
    }),
    useSaveRequest: () => ({ status: "idle", run: vi.fn() }),
    useLspConfigActions: () => ({ updateLspConfigString: vi.fn() }),
    useLspLanguageToggles: () => ({ toggleAutoStart: vi.fn(), toggleAutoInstall: vi.fn() }),
  };
});

vi.mock("@/components/settings/settings-page-template", () => ({
  SettingsPageTemplate: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));
vi.mock("@/components/settings/settings-group", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/settings/settings-group")>();
  return {
    ...actual,
    SettingsGroup: ({ children }: { children: React.ReactNode }) => <section>{children}</section>,
  };
});
vi.mock("@/components/settings/lsp-language-cards", () => ({ LspLanguageCards: () => null }));
vi.mock("@/components/settings/lsp-status-location-setting", () => ({
  LspStatusLocationSetting: () => null,
}));
vi.mock("@/components/settings/editor-form", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/settings/editor-form")>();
  return { ...actual, EditorForm: () => null };
});
vi.mock("@/components/combobox", () => ({ Combobox: () => null }));

import { EditorsSettings } from "./editors-settings";

afterEach(() => {
  cleanup();
  mocks.isAdmin = true;
  mocks.state.editors = [
    {
      id: mocks.editorId,
      type: mocks.editorType,
      name: mocks.editorName,
      kind: "custom_command",
      config: { command: mocks.command },
      enabled: true,
      installed: true,
    } as EditorOption,
  ];
  mocks.state.editingId = null;
  mocks.state.isAdding = false;
  mocks.state.setIsAdding.mockClear();
  mocks.state.setEditingId.mockClear();
});

describe("custom editor controls follow org.config.manage", () => {
  it("keeps add, edit, and remove actions for an administrator", () => {
    render(<EditorsSettings />);

    expect(screen.getByRole("button", { name: mocks.addEditorButtonName })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Edit" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Remove" })).toBeTruthy();
  });

  it("keeps custom editor details readable for a member and hides management actions", () => {
    mocks.isAdmin = false;
    render(<EditorsSettings />);

    expect(screen.getByText(mocks.editorName)).toBeTruthy();
    expect(screen.getByText(mocks.command)).toBeTruthy();
    expect(screen.queryByRole("button", { name: mocks.addEditorButtonName })).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Remove" })).toBeNull();

    fireEvent.click(screen.getByText(mocks.editorName));
    expect(mocks.state.setEditingId).not.toHaveBeenCalled();
  });

  it("does not offer an add action for an empty list to a member", () => {
    mocks.isAdmin = false;
    mocks.state.editors = [];
    render(<EditorsSettings />);

    expect(screen.getByText("No custom editors yet.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: mocks.addEditorButtonName })).toBeNull();
  });
});
