"use client";

import { useCallback, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useEditors } from "@/hooks/domains/settings/use-editors";
import { createEditor, deleteEditor, updateEditor, updateUserSettings } from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";
import type { EditorOption } from "@/lib/types/http";
import { type ComboboxOption } from "@/components/combobox";
import {
  type EditorFormState,
  buildConfig,
  resolveAvailableEditors,
  resolveDefaultEditorId,
  getCustomKindLabel,
  isCustomEditor,
} from "@/components/settings/editor-form";
import { Badge } from "@kandev/ui/badge";

export function useEditorsSettingsState() {
  const setEditors = useAppStore((state) => state.setEditors);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const currentUserSettings = useAppStore((state) => state.userSettings);
  const { editors: storeEditors } = useEditors();
  const [editors, setEditorItems] = useState<EditorOption[]>(() => storeEditors ?? []);
  const initialDefaultId = resolveDefaultEditorId(
    editors ?? [],
    currentUserSettings.defaultEditorId ?? "",
  );
  const [defaultEditorId, setDefaultEditorId] = useState(initialDefaultId);
  const [baselineDefaultId, setBaselineDefaultId] = useState(initialDefaultId);
  const [isAdding, setIsAdding] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [lspAutoStartLanguages, setLspAutoStartLanguages] = useState<string[]>(
    () => currentUserSettings.lspAutoStartLanguages ?? [],
  );
  const [lspAutoInstallLanguages, setLspAutoInstallLanguages] = useState<string[]>(
    () => currentUserSettings.lspAutoInstallLanguages ?? [],
  );
  const [baselineLspAutoStart, setBaselineLspAutoStart] = useState<string[]>(
    () => currentUserSettings.lspAutoStartLanguages ?? [],
  );
  const [baselineLspAutoInstall, setBaselineLspAutoInstall] = useState<string[]>(
    () => currentUserSettings.lspAutoInstallLanguages ?? [],
  );

  const initConfigStrings = useCallback((): Record<string, string> => {
    const configs = currentUserSettings.lspServerConfigs ?? {};
    const result: Record<string, string> = {};
    for (const [lang, config] of Object.entries(configs)) {
      if (config && Object.keys(config).length > 0) result[lang] = JSON.stringify(config, null, 2);
    }
    return result;
  }, [currentUserSettings.lspServerConfigs]);
  const [lspConfigStrings, setLspConfigStrings] =
    useState<Record<string, string>>(initConfigStrings);
  const [baselineLspConfigStrings, setBaselineLspConfigStrings] =
    useState<Record<string, string>>(initConfigStrings);
  const [expandedConfigLang, setExpandedConfigLang] = useState<string | null>(null);
  const [lspConfigErrors, setLspConfigErrors] = useState<Record<string, string>>({});

  return {
    setEditors,
    setUserSettings,
    currentUserSettings,
    editors,
    setEditorItems,
    defaultEditorId,
    setDefaultEditorId,
    baselineDefaultId,
    setBaselineDefaultId,
    isAdding,
    setIsAdding,
    editingId,
    setEditingId,
    lspAutoStartLanguages,
    setLspAutoStartLanguages,
    lspAutoInstallLanguages,
    setLspAutoInstallLanguages,
    baselineLspAutoStart,
    setBaselineLspAutoStart,
    baselineLspAutoInstall,
    setBaselineLspAutoInstall,
    lspConfigStrings,
    setLspConfigStrings,
    baselineLspConfigStrings,
    setBaselineLspConfigStrings,
    expandedConfigLang,
    setExpandedConfigLang,
    lspConfigErrors,
    setLspConfigErrors,
  };
}

export type EditorsSettingsState = ReturnType<typeof useEditorsSettingsState>;

export function useLspConfigActions(
  setLspConfigStrings: (updater: (prev: Record<string, string>) => Record<string, string>) => void,
  setLspConfigErrors: (updater: (prev: Record<string, string>) => Record<string, string>) => void,
) {
  const clearLspConfigError = useCallback(
    (langId: string) => {
      setLspConfigErrors((prev) => {
        const next = { ...prev };
        delete next[langId];
        return next;
      });
    },
    [setLspConfigErrors],
  );

  const updateLspConfigString = useCallback(
    (langId: string, value: string) => {
      setLspConfigStrings((prev) => {
        if (!value.trim()) {
          const next = { ...prev };
          delete next[langId];
          return next;
        }
        return { ...prev, [langId]: value };
      });
      if (!value.trim()) {
        clearLspConfigError(langId);
        return;
      }
      try {
        const parsed = JSON.parse(value);
        if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
          setLspConfigErrors((prev) => ({ ...prev, [langId]: "Must be a JSON object" }));
        } else {
          clearLspConfigError(langId);
        }
      } catch {
        setLspConfigErrors((prev) => ({ ...prev, [langId]: "Invalid JSON" }));
      }
    },
    [setLspConfigStrings, setLspConfigErrors, clearLspConfigError],
  );

  return { updateLspConfigString };
}

export function parseLspConfigStrings(
  lspConfigStrings: Record<string, string>,
): Record<string, Record<string, unknown>> | null {
  const result: Record<string, Record<string, unknown>> = {};
  for (const [lang, str] of Object.entries(lspConfigStrings)) {
    if (!str.trim()) continue;
    try {
      const parsed = JSON.parse(str);
      if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return null;
      result[lang] = parsed as Record<string, unknown>;
    } catch {
      return null;
    }
  }
  return result;
}

export function buildDefaultEditorOptions(
  availableEditors: EditorOption[],
  defaultEditorId: string,
): ComboboxOption[] {
  if (availableEditors.length === 0) return [];
  const selected = defaultEditorId ? availableEditors.filter((e) => e.id === defaultEditorId) : [];
  const rest = availableEditors.filter((e) => e.id !== defaultEditorId);
  const ordered = [...selected, ...rest];
  return ordered.map((editor) => ({
    value: editor.id,
    label: editor.name,
    renderLabel: () => (
      <div className="flex min-w-0 flex-1 items-center gap-2">
        <span className="truncate">{editor.name}</span>
        {editor.kind === "built_in" ? (
          <Badge variant={editor.installed ? "secondary" : "outline"} className="ml-auto">
            {editor.installed ? "Installed" : "Not installed"}
          </Badge>
        ) : (
          <Badge variant="secondary" className="ml-auto">
            {getCustomKindLabel(editor.kind)}
          </Badge>
        )}
      </div>
    ),
  }));
}

type UserSettingsState = ReturnType<typeof useEditorsSettingsState>["currentUserSettings"];
type UpdateUserSettingsResponse = Awaited<ReturnType<typeof updateUserSettings>>;
type SetUserSettingsFn = ReturnType<typeof useEditorsSettingsState>["setUserSettings"];

function buildSettingsPayload(
  s: UserSettingsState,
  defaultEditorId: string,
  lspAutoStartLanguages: string[],
  lspAutoInstallLanguages: string[],
  parsedConfigs: Record<string, Record<string, unknown>>,
): Parameters<typeof updateUserSettings>[0] {
  return {
    workspace_id: s.workspaceId ?? "",
    repository_ids: s.repositoryIds ?? [],
    default_editor_id: defaultEditorId || undefined,
    lsp_auto_start_languages: lspAutoStartLanguages,
    lsp_auto_install_languages: lspAutoInstallLanguages,
    lsp_server_configs: parsedConfigs,
  };
}

function mapEditorSettingsFields(
  s: NonNullable<NonNullable<UpdateUserSettingsResponse>["settings"]>,
) {
  return {
    chatSubmitKey: s.chat_submit_key ?? "cmd_enter",
    reviewAutoMarkOnScroll: s.review_auto_mark_on_scroll ?? true,
    showReleaseNotification: s.show_release_notification ?? true,
    releaseNotesLastSeenVersion: (s.release_notes_last_seen_version as string) || null,
    lspAutoStartLanguages: s.lsp_auto_start_languages ?? [],
    lspAutoInstallLanguages: s.lsp_auto_install_languages ?? [],
    lspServerConfigs: s.lsp_server_configs ?? {},
    savedLayouts: s.saved_layouts ?? [],
    loaded: true as const,
  };
}

function buildUserSettingsFromResponse(
  s: NonNullable<UpdateUserSettingsResponse>["settings"],
  shellOptions: Array<{ value: string; label: string }> | null | undefined,
) {
  if (!s) return null;
  return {
    workspaceId: s.workspace_id || null,
    workflowId: s.workflow_filter_id || null,
    kanbanViewMode: s.kanban_view_mode || null,
    repositoryIds: s.repository_ids ?? [],
    preferredShell: s.preferred_shell || null,
    shellOptions: shellOptions ?? [],
    defaultEditorId: s.default_editor_id || null,
    enablePreviewOnClick: s.enable_preview_on_click ?? false,
    ...mapEditorSettingsFields(s),
  };
}

function applySettingsResponseToStore(
  response: UpdateUserSettingsResponse,
  shellOptions: Array<{ value: string; label: string }> | null | undefined,
  setUserSettings: SetUserSettingsFn,
) {
  if (!response?.settings) return;
  const settings = buildUserSettingsFromResponse(response.settings, shellOptions);
  if (settings) setUserSettings(settings);
}

export function useApplyEditors(state: EditorsSettingsState) {
  const { defaultEditorId, setEditorItems, setDefaultEditorId, setBaselineDefaultId } = state;
  return useCallback(
    (updater: EditorOption[] | ((prev: EditorOption[]) => EditorOption[])) => {
      setEditorItems((prev) => {
        const next = typeof updater === "function" ? updater(prev) : updater;
        const resolvedDefault = resolveDefaultEditorId(next, defaultEditorId);
        if (resolvedDefault !== defaultEditorId) {
          setDefaultEditorId(resolvedDefault);
          setBaselineDefaultId(resolvedDefault);
        }
        return next;
      });
    },
    [defaultEditorId, setEditorItems, setDefaultEditorId, setBaselineDefaultId],
  );
}

export function useEditorRequests(
  state: EditorsSettingsState,
  applyEditors: (updater: EditorOption[] | ((prev: EditorOption[]) => EditorOption[])) => void,
) {
  const {
    setIsAdding,
    defaultEditorId,
    setDefaultEditorId,
    setBaselineDefaultId,
    setUserSettings,
    currentUserSettings,
  } = state;

  const createRequest = useRequest(async (editorState: EditorFormState) => {
    const created = await createEditor(
      {
        name: editorState.name,
        kind: editorState.kind,
        config: buildConfig(editorState),
        enabled: editorState.enabled,
      },
      { cache: "no-store" },
    );
    applyEditors((prev: EditorOption[]) => [...prev, created]);
    setIsAdding(false);
  });

  const updateRequest = useRequest(async (editorId: string, editorState: EditorFormState) => {
    const updated = await updateEditor(
      editorId,
      {
        name: editorState.name,
        kind: editorState.kind,
        config: buildConfig(editorState),
        enabled: editorState.enabled,
      },
      { cache: "no-store" },
    );
    applyEditors((prev: EditorOption[]) =>
      prev.map((editor: EditorOption) => (editor.id === editorId ? updated : editor)),
    );
  });

  const deleteRequest = useRequest(async (editorId: string) => {
    await deleteEditor(editorId, { cache: "no-store" });
    applyEditors((prev: EditorOption[]) =>
      prev.filter((editor: EditorOption) => editor.id !== editorId),
    );
    if (defaultEditorId === editorId) {
      setDefaultEditorId("");
      setBaselineDefaultId("");
      setUserSettings({ ...currentUserSettings, defaultEditorId: null, loaded: true });
    }
  });

  return { createRequest, updateRequest, deleteRequest };
}

export function useSaveRequest(state: EditorsSettingsState) {
  const {
    setUserSettings,
    currentUserSettings,
    defaultEditorId,
    setBaselineDefaultId,
    lspAutoStartLanguages,
    setBaselineLspAutoStart,
    lspAutoInstallLanguages,
    setBaselineLspAutoInstall,
    lspConfigStrings,
    setBaselineLspConfigStrings,
  } = state;
  return useRequest(async () => {
    const parsedConfigs = parseLspConfigStrings(lspConfigStrings);
    if (parsedConfigs === null) return;
    const payload = buildSettingsPayload(
      currentUserSettings,
      defaultEditorId,
      lspAutoStartLanguages,
      lspAutoInstallLanguages,
      parsedConfigs,
    );
    const response = await updateUserSettings(payload, { cache: "no-store" });
    setBaselineDefaultId(defaultEditorId);
    setBaselineLspAutoStart([...lspAutoStartLanguages]);
    setBaselineLspAutoInstall([...lspAutoInstallLanguages]);
    setBaselineLspConfigStrings({ ...lspConfigStrings });
    applySettingsResponseToStore(response, currentUserSettings.shellOptions, setUserSettings);
  });
}

export function sortCustomEditors(items: EditorOption[]): EditorOption[] {
  return items.slice().sort((a, b) => {
    const createdA = a.created_at ? Date.parse(a.created_at) : 0;
    const createdB = b.created_at ? Date.parse(b.created_at) : 0;
    if (createdA !== createdB) return createdB - createdA;
    const nameA = (a.name || "").toLowerCase();
    const nameB = (b.name || "").toLowerCase();
    if (nameA < nameB) return -1;
    if (nameA > nameB) return 1;
    return a.id.localeCompare(b.id);
  });
}

export { resolveAvailableEditors, isCustomEditor };
