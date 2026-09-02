"use client";

import { useCallback, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useEditors } from "@/hooks/domains/settings/use-editors";
import { createEditor, deleteEditor, updateEditor, updateUserSettings } from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";
import type { EditorOption, LspStatusLocation } from "@/lib/types/http";
import { type ComboboxOption } from "@/components/combobox";
import { mapUserSettingsResponse } from "@/lib/ssr/user-settings";
import {
  type EditorFormState,
  buildConfig,
  resolveAvailableEditors,
  resolveDefaultEditorId,
  getCustomKindLabel,
  isCustomEditor,
} from "@/components/settings/editor-form";
import { Badge } from "@kandev/ui/badge";
import { useTranslation } from "react-i18next";

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
  const [lspStatusHiddenLanguages, setLspStatusHiddenLanguages] = useState<string[]>(
    () => currentUserSettings.lspStatusHiddenLanguages ?? [],
  );
  const [baselineLspStatusHiddenLanguages, setBaselineLspStatusHiddenLanguages] = useState<
    string[]
  >(() => currentUserSettings.lspStatusHiddenLanguages ?? []);
  const [lspStatusLocation, setLspStatusLocation] = useState<LspStatusLocation>(
    () => currentUserSettings.lspStatusLocation ?? "toolbar",
  );
  const [baselineLspStatusLocation, setBaselineLspStatusLocation] = useState<LspStatusLocation>(
    () => currentUserSettings.lspStatusLocation ?? "toolbar",
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
    lspStatusHiddenLanguages,
    setLspStatusHiddenLanguages,
    baselineLspStatusHiddenLanguages,
    setBaselineLspStatusHiddenLanguages,
    lspStatusLocation,
    setLspStatusLocation,
    baselineLspStatusLocation,
    setBaselineLspStatusLocation,
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

function updateStatusVisibility(previous: string[], language: string, visible: boolean) {
  if (visible) return previous.filter((id) => id !== language);
  if (previous.includes(language)) return previous;
  return [...previous, language];
}

export function useLspConfigActions(
  setLspConfigStrings: (updater: (prev: Record<string, string>) => Record<string, string>) => void,
  setLspConfigErrors: (updater: (prev: Record<string, string>) => Record<string, string>) => void,
) {
  const { t } = useTranslation();
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
          setLspConfigErrors((prev) => ({ ...prev, [langId]: t("settings:mustBeAJsonObject") }));
        } else {
          clearLspConfigError(langId);
        }
      } catch {
        setLspConfigErrors((prev) => ({ ...prev, [langId]: t("settings:invalidJson") }));
      }
    },
    [setLspConfigStrings, setLspConfigErrors, clearLspConfigError, t],
  );

  return { updateLspConfigString };
}

/** Add/remove a language id in the LSP preference sets. */
export function useLspLanguageToggles(state: EditorsSettingsState) {
  const { setLspAutoStartLanguages, setLspAutoInstallLanguages, setLspStatusHiddenLanguages } =
    state;
  const toggleAutoStart = useCallback(
    (langId: string, checked: boolean) => {
      setLspAutoStartLanguages((prev) =>
        checked ? [...prev, langId] : prev.filter((id) => id !== langId),
      );
    },
    [setLspAutoStartLanguages],
  );
  const toggleAutoInstall = useCallback(
    (langId: string, checked: boolean) => {
      setLspAutoInstallLanguages((prev) =>
        checked ? [...prev, langId] : prev.filter((id) => id !== langId),
      );
    },
    [setLspAutoInstallLanguages],
  );
  const toggleStatusVisibility = useCallback(
    (langId: string, visible: boolean) => {
      setLspStatusHiddenLanguages((prev) => updateStatusVisibility(prev, langId, visible));
    },
    [setLspStatusHiddenLanguages],
  );
  return { toggleAutoStart, toggleAutoInstall, toggleStatusVisibility };
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

/**
 * `t` is a parameter rather than a module import: the option list is built in a
 * `useMemo`, so passing the caller's `t` both keeps the copy out of module scope
 * and gives the memo a dependency that changes on a locale switch.
 */
export function buildDefaultEditorOptions(
  availableEditors: EditorOption[],
  defaultEditorId: string,
  t: (key: string) => string,
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
            {editor.installed ? t("settings:installed") : t("settings:notInstalled")}
          </Badge>
        ) : (
          <Badge variant="secondary" className="ml-auto">
            {getCustomKindLabel(t, editor.kind)}
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
  lsp: {
    autoStartLanguages: string[];
    autoInstallLanguages: string[];
    statusHiddenLanguages: string[];
    statusLocation: LspStatusLocation;
    serverConfigs: Record<string, Record<string, unknown>>;
  },
): Parameters<typeof updateUserSettings>[0] {
  return {
    workspace_id: s.workspaceId ?? "",
    repository_ids: s.repositoryIds ?? [],
    default_editor_id: defaultEditorId || undefined,
    lsp_auto_start_languages: lsp.autoStartLanguages,
    lsp_auto_install_languages: lsp.autoInstallLanguages,
    lsp_status_hidden_languages: lsp.statusHiddenLanguages,
    lsp_status_location: lsp.statusLocation,
    lsp_server_configs: lsp.serverConfigs,
  };
}

function applySettingsResponseToStore(
  response: UpdateUserSettingsResponse,
  current: UserSettingsState,
  setUserSettings: SetUserSettingsFn,
) {
  if (!response?.settings) return;
  setUserSettings(mapUserSettingsResponse(response, current));
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
    lspStatusHiddenLanguages,
    setBaselineLspStatusHiddenLanguages,
    lspStatusLocation,
    setBaselineLspStatusLocation,
    lspConfigStrings,
    setBaselineLspConfigStrings,
  } = state;
  return useRequest(async () => {
    const parsedConfigs = parseLspConfigStrings(lspConfigStrings);
    if (parsedConfigs === null) return;
    const payload = buildSettingsPayload(currentUserSettings, defaultEditorId, {
      autoStartLanguages: lspAutoStartLanguages,
      autoInstallLanguages: lspAutoInstallLanguages,
      statusHiddenLanguages: lspStatusHiddenLanguages,
      statusLocation: lspStatusLocation,
      serverConfigs: parsedConfigs,
    });
    const response = await updateUserSettings(payload, { cache: "no-store" });
    setBaselineDefaultId(defaultEditorId);
    setBaselineLspAutoStart([...lspAutoStartLanguages]);
    setBaselineLspAutoInstall([...lspAutoInstallLanguages]);
    setBaselineLspStatusHiddenLanguages([...lspStatusHiddenLanguages]);
    setBaselineLspStatusLocation(lspStatusLocation);
    setBaselineLspConfigStrings({ ...lspConfigStrings });
    applySettingsResponseToStore(response, currentUserSettings, setUserSettings);
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
