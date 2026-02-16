'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { IconEdit, IconTrash, IconChevronDown, IconExternalLink, IconInfoCircle } from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import { Button } from '@kandev/ui/button';
import { Checkbox } from '@kandev/ui/checkbox';
import { Input } from '@kandev/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { Separator } from '@kandev/ui/separator';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { Switch } from '@kandev/ui/switch';
import { Textarea } from '@kandev/ui/textarea';
import { SettingsPageTemplate } from '@/components/settings/settings-page-template';
import { Combobox, type ComboboxOption } from '@/components/combobox';
import { useAppStore } from '@/components/state-provider';
import { EditableCard } from '@/components/settings/editable-card';
import { useEditors } from '@/hooks/domains/settings/use-editors';
import { createEditor, deleteEditor, updateEditor, updateUserSettings } from '@/lib/api';
import { useRequest } from '@/lib/http/use-request';
import { LSP_DEFAULT_CONFIGS } from '@/lib/lsp/lsp-client-manager';
import type { EditorOption } from '@/lib/types/http';

type CustomKind = 'custom_command' | 'custom_remote_ssh' | 'custom_hosted_url';

type EditorFormState = {
  name: string;
  kind: CustomKind;
  command: string;
  host: string;
  user: string;
  url: string;
  scheme: string;
  enabled: boolean;
};

const CUSTOM_KIND_OPTIONS: Array<{ value: CustomKind; label: string }> = [
  { value: 'custom_command', label: 'Command' },
  { value: 'custom_remote_ssh', label: 'VS Code Remote SSH' },
  { value: 'custom_hosted_url', label: 'Hosted URL' },
];

const PLACEHOLDER_HINT = '{cwd} {file} {rel} {line} {column}';

const LSP_LANGUAGE_OPTIONS = [
  { id: 'typescript', label: 'TypeScript / JavaScript', binary: 'typescript-language-server',
    docsUrl: 'https://github.com/typescript-language-server/typescript-language-server#workspace-configuration',
    installHint: 'Installs typescript-language-server and typescript via npm into ~/.kandev/lsp-servers/' },
  { id: 'go', label: 'Go', binary: 'gopls',
    docsUrl: 'https://github.com/golang/tools/blob/master/gopls/doc/settings.md',
    installHint: 'Runs "go install golang.org/x/tools/gopls@latest". Requires Go to be installed.' },
  { id: 'rust', label: 'Rust', binary: 'rust-analyzer',
    docsUrl: 'https://rust-analyzer.github.io/book/configuration.html',
    installHint: 'Downloads the rust-analyzer binary from GitHub releases into ~/.kandev/lsp-servers/' },
  { id: 'python', label: 'Python', binary: 'pyright-langserver',
    docsUrl: 'https://microsoft.github.io/pyright/#/settings',
    installHint: 'Installs pyright via npm into ~/.kandev/lsp-servers/' },
];

function getCustomKindLabel(kind: string) {
  return CUSTOM_KIND_OPTIONS.find((option) => option.value === kind)?.label ?? 'Custom';
}

function isCustomEditor(editor: EditorOption) {
  return editor.kind.startsWith('custom');
}

function editorConfigValue(editor: EditorOption, key: string) {
  if (!editor.config || typeof editor.config !== 'object') {
    return '';
  }
  const value = (editor.config as Record<string, unknown>)[key];
  return typeof value === 'string' ? value : '';
}

function getCustomEditorSummary(editor: EditorOption) {
  switch (editor.kind) {
    case 'custom_command': {
      return editorConfigValue(editor, 'command') || getCustomKindLabel(editor.kind);
    }
    case 'custom_hosted_url': {
      return editorConfigValue(editor, 'url') || getCustomKindLabel(editor.kind);
    }
    case 'custom_remote_ssh': {
      const host = editorConfigValue(editor, 'host');
      const user = editorConfigValue(editor, 'user');
      if (host && user) return `${user}@${host}`;
      if (host) return host;
      return getCustomKindLabel(editor.kind);
    }
    default:
      return getCustomKindLabel(editor.kind);
  }
}

function buildConfig(state: EditorFormState) {
  switch (state.kind) {
    case 'custom_command':
      return { command: state.command };
    case 'custom_remote_ssh':
      return {
        host: state.host,
        user: state.user || undefined,
        scheme: state.scheme || undefined,
      };
    case 'custom_hosted_url':
      return { url: state.url };
    default:
      return {};
  }
}

function defaultFormState(): EditorFormState {
  return {
    name: '',
    kind: 'custom_command',
    command: '',
    host: '',
    user: '',
    url: '',
    scheme: '',
    enabled: true,
  };
}

function resolveEditorName(state: EditorFormState) {
  const trimmed = state.name.trim();
  if (trimmed) return trimmed;
  switch (state.kind) {
    case 'custom_remote_ssh':
      return state.host.trim();
    case 'custom_hosted_url':
      return state.url.trim();
    case 'custom_command':
    default:
      return state.command.trim();
  }
}

function formStateFromEditor(editor: EditorOption): EditorFormState {
  return {
    name: editor.name,
    kind: (editor.kind as CustomKind) || 'custom_command',
    command: editorConfigValue(editor, 'command'),
    host: editorConfigValue(editor, 'host'),
    user: editorConfigValue(editor, 'user'),
    url: editorConfigValue(editor, 'url'),
    scheme: editorConfigValue(editor, 'scheme'),
    enabled: editor.enabled,
  };
}

type EditorFormProps = {
  title: string;
  initialState: EditorFormState;
  onCancel: () => void;
  onSave: (state: EditorFormState) => void;
  submitLabel: string;
  isSaving: boolean;
};

function EditorForm({
  title,
  initialState,
  onCancel,
  onSave,
  submitLabel,
  isSaving,
}: EditorFormProps) {
  const [state, setState] = useState<EditorFormState>(initialState);

  useEffect(() => {
    setState(initialState);
  }, [initialState]);

  const setField = <K extends keyof EditorFormState>(key: K, value: EditorFormState[K]) => {
    setState((prev) => ({ ...prev, [key]: value }));
  };

  const isValid = useMemo(() => {
    const resolvedName = resolveEditorName(state);
    if (!resolvedName) return false;
    if (state.kind === 'custom_command') return Boolean(state.command.trim());
    if (state.kind === 'custom_remote_ssh') return Boolean(state.host.trim());
    if (state.kind === 'custom_hosted_url') return Boolean(state.url.trim());
    return false;
  }, [state]);

  return (
    <div className="rounded-lg border border-border/70 bg-background p-4 space-y-4">
      <div className="text-sm font-medium text-foreground">{title}</div>
      <Input
        value={state.name}
        onChange={(event) => setField('name', event.target.value)}
        placeholder="Editor name"
      />
      <Select
        value={state.kind}
        onValueChange={(value) => setField('kind', value as CustomKind)}
      >
        <SelectTrigger>
          <SelectValue placeholder="Editor type" />
        </SelectTrigger>
        <SelectContent>
          <div className="px-2 py-1.5 text-xs text-muted-foreground border-b">Editor type</div>
          {CUSTOM_KIND_OPTIONS.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {state.kind === 'custom_command' && (
        <div className="space-y-2">
          <Input
            value={state.command}
            onChange={(event) => setField('command', event.target.value)}
            placeholder="code --goto {file}:{line}"
          />
          <p className="text-xs text-muted-foreground">
            Supports placeholders: {PLACEHOLDER_HINT}
          </p>
        </div>
      )}

      {state.kind === 'custom_remote_ssh' && (
        <div className="space-y-2">
          <Input
            value={state.host}
            onChange={(event) => setField('host', event.target.value)}
            placeholder="ssh-host.example.com"
          />
          <Input
            value={state.user}
            onChange={(event) => setField('user', event.target.value)}
            placeholder="optional username"
          />
          <Input
            value={state.scheme}
            onChange={(event) => setField('scheme', event.target.value)}
            placeholder="optional scheme (vscode, cursor)"
          />
          <p className="text-xs text-muted-foreground">
            Opens a VS Code Remote SSH URL using the scheme (example:
            {' '}vscode://vscode-remote/ssh-remote+user@host:/path/file:line).
          </p>
        </div>
      )}

      {state.kind === 'custom_hosted_url' && (
        <Input
          value={state.url}
          onChange={(event) => setField('url', event.target.value)}
          placeholder="https://code.example.com"
        />
      )}

      <div className="flex items-center justify-end gap-2">
        <Button type="button" variant="outline" onClick={onCancel} disabled={isSaving}>
          Cancel
        </Button>
        <Button
          type="button"
          onClick={() => {
            const resolvedName = resolveEditorName(state);
            onSave(resolvedName === state.name ? state : { ...state, name: resolvedName });
          }}
          disabled={isSaving || !isValid}
        >
          {submitLabel}
        </Button>
      </div>
    </div>
  );
}

function resolveAvailableEditors(editors: EditorOption[]) {
  return editors.filter((editor) => {
    if (!editor.enabled) return false;
    if (editor.kind === 'built_in') return editor.installed;
    return true;
  });
}

function resolveDefaultEditorId(editors: EditorOption[], desiredId: string) {
  const available = resolveAvailableEditors(editors);
  if (desiredId && available.some((editor) => editor.id === desiredId)) {
    return desiredId;
  }
  if (!desiredId && available.length > 0) {
    return available[0].id;
  }
  return '';
}

export function EditorsSettings() {
  const setEditors = useAppStore((state) => state.setEditors);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const currentUserSettings = useAppStore((state) => state.userSettings);
  const { editors: storeEditors } = useEditors();
  const [editors, setEditorItems] = useState<EditorOption[]>(() => storeEditors ?? []);
  const initialDefaultId = resolveDefaultEditorId(
    editors ?? [],
    currentUserSettings.defaultEditorId ?? ''
  );
  const [defaultEditorId, setDefaultEditorId] = useState(initialDefaultId);
  const [baselineDefaultId, setBaselineDefaultId] = useState(initialDefaultId);
  const [isAdding, setIsAdding] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  // LSP settings
  const [lspAutoStartLanguages, setLspAutoStartLanguages] = useState<string[]>(
    () => currentUserSettings.lspAutoStartLanguages ?? []
  );
  const [lspAutoInstallLanguages, setLspAutoInstallLanguages] = useState<string[]>(
    () => currentUserSettings.lspAutoInstallLanguages ?? []
  );
  const [baselineLspAutoStart, setBaselineLspAutoStart] = useState<string[]>(
    () => currentUserSettings.lspAutoStartLanguages ?? []
  );
  const [baselineLspAutoInstall, setBaselineLspAutoInstall] = useState<string[]>(
    () => currentUserSettings.lspAutoInstallLanguages ?? []
  );

  // LSP server configs (per-language JSON overrides)
  const initConfigStrings = useCallback((): Record<string, string> => {
    const configs = currentUserSettings.lspServerConfigs ?? {};
    const result: Record<string, string> = {};
    for (const [lang, config] of Object.entries(configs)) {
      if (config && Object.keys(config).length > 0) {
        result[lang] = JSON.stringify(config, null, 2);
      }
    }
    return result;
  }, [currentUserSettings.lspServerConfigs]);
  const [lspConfigStrings, setLspConfigStrings] = useState<Record<string, string>>(initConfigStrings);
  const [baselineLspConfigStrings, setBaselineLspConfigStrings] = useState<Record<string, string>>(initConfigStrings);
  const [expandedConfigLang, setExpandedConfigLang] = useState<string | null>(null);
  const [lspConfigErrors, setLspConfigErrors] = useState<Record<string, string>>({});

  const toggleAutoStart = useCallback((langId: string, checked: boolean) => {
    setLspAutoStartLanguages((prev) =>
      checked ? [...prev, langId] : prev.filter((id) => id !== langId)
    );
  }, []);

  const toggleAutoInstall = useCallback((langId: string, checked: boolean) => {
    setLspAutoInstallLanguages((prev) =>
      checked ? [...prev, langId] : prev.filter((id) => id !== langId)
    );
  }, []);

  const updateLspConfigString = useCallback((langId: string, value: string) => {
    setLspConfigStrings((prev) => {
      if (!value.trim()) {
        const next = { ...prev };
        delete next[langId];
        return next;
      }
      return { ...prev, [langId]: value };
    });
    // Validate JSON
    if (!value.trim()) {
      setLspConfigErrors((prev) => {
        const next = { ...prev };
        delete next[langId];
        return next;
      });
      return;
    }
    try {
      const parsed = JSON.parse(value);
      if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
        setLspConfigErrors((prev) => ({ ...prev, [langId]: 'Must be a JSON object' }));
      } else {
        setLspConfigErrors((prev) => {
          const next = { ...prev };
          delete next[langId];
          return next;
        });
      }
    } catch {
      setLspConfigErrors((prev) => ({ ...prev, [langId]: 'Invalid JSON' }));
    }
  }, []);

  const parseLspConfigs = useCallback((): Record<string, Record<string, unknown>> | null => {
    const result: Record<string, Record<string, unknown>> = {};
    for (const [lang, str] of Object.entries(lspConfigStrings)) {
      if (!str.trim()) continue;
      try {
        const parsed = JSON.parse(str);
        if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) return null;
        result[lang] = parsed as Record<string, unknown>;
      } catch {
        return null;
      }
    }
    return result;
  }, [lspConfigStrings]);


  const customEditors = useMemo(() => {
    const items = editors.filter(isCustomEditor);
    return items.slice().sort((a, b) => {
      const createdA = a.created_at ? Date.parse(a.created_at) : 0;
      const createdB = b.created_at ? Date.parse(b.created_at) : 0;
      if (createdA !== createdB) return createdB - createdA;
      const nameA = (a.name || '').toLowerCase();
      const nameB = (b.name || '').toLowerCase();
      if (nameA < nameB) return -1;
      if (nameA > nameB) return 1;
      return a.id.localeCompare(b.id);
    });
  }, [editors]);
  const builtInEditors = useMemo(() => editors.filter((editor) => !isCustomEditor(editor)), [editors]);
  const availableEditors = useMemo(() => resolveAvailableEditors(editors), [editors]);

  const applyEditors = useCallback(
    (updater: EditorOption[] | ((prev: EditorOption[]) => EditorOption[])) => {
      setEditorItems((prev) => {
        const next = typeof updater === 'function' ? updater(prev) : updater;
        const resolvedDefault = resolveDefaultEditorId(next, defaultEditorId);
        if (resolvedDefault !== defaultEditorId) {
          setDefaultEditorId(resolvedDefault);
          setBaselineDefaultId(resolvedDefault);
        }
        return next;
      });
    },
    [defaultEditorId]
  );

  useEffect(() => {
    setEditors(editors);
  }, [editors, setEditors]);

  const defaultOptions = useMemo<ComboboxOption[]>(() => {
    if (availableEditors.length === 0) return [];
    const selected = defaultEditorId
      ? availableEditors.filter((editor) => editor.id === defaultEditorId)
      : [];
    const rest = availableEditors.filter((editor) => editor.id !== defaultEditorId);
    const ordered = [...selected, ...rest];
    return ordered.map((editor) => ({
      value: editor.id,
      label: editor.name,
      renderLabel: () => (
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <span className="truncate">{editor.name}</span>
          {editor.kind === 'built_in' ? (
            <Badge variant={editor.installed ? 'secondary' : 'outline'} className="ml-auto">
              {editor.installed ? 'Installed' : 'Not installed'}
            </Badge>
          ) : (
            <Badge variant="secondary" className="ml-auto">
              {getCustomKindLabel(editor.kind)}
            </Badge>
          )}
        </div>
      ),
    }));
  }, [availableEditors, defaultEditorId]);

  const lspAutoStartDirty =
    lspAutoStartLanguages.length !== baselineLspAutoStart.length ||
    lspAutoStartLanguages.some((id) => !baselineLspAutoStart.includes(id));
  const lspAutoInstallDirty =
    lspAutoInstallLanguages.length !== baselineLspAutoInstall.length ||
    lspAutoInstallLanguages.some((id) => !baselineLspAutoInstall.includes(id));
  const lspConfigsDirty = JSON.stringify(lspConfigStrings) !== JSON.stringify(baselineLspConfigStrings);
  const isDirty = defaultEditorId !== baselineDefaultId || lspAutoStartDirty || lspAutoInstallDirty || lspConfigsDirty;

  const saveDefaultRequest = useRequest(async () => {
    const parsedConfigs = parseLspConfigs();
    if (parsedConfigs === null) return; // Invalid JSON, don't save
    const payload: Parameters<typeof updateUserSettings>[0] = {
      workspace_id: currentUserSettings.workspaceId ?? '',
      repository_ids: currentUserSettings.repositoryIds ?? [],
      default_editor_id: defaultEditorId || undefined,
      lsp_auto_start_languages: lspAutoStartLanguages,
      lsp_auto_install_languages: lspAutoInstallLanguages,
      lsp_server_configs: parsedConfigs,
    };
    const response = await updateUserSettings(payload, { cache: 'no-store' });
    setBaselineDefaultId(defaultEditorId);
    setBaselineLspAutoStart([...lspAutoStartLanguages]);
    setBaselineLspAutoInstall([...lspAutoInstallLanguages]);
    setBaselineLspConfigStrings({ ...lspConfigStrings });
    if (response?.settings) {
      setUserSettings({
        workspaceId: response.settings.workspace_id || null,
        workflowId: response.settings.workflow_filter_id || null,
        kanbanViewMode: response.settings.kanban_view_mode || null,
        repositoryIds: response.settings.repository_ids ?? [],
        preferredShell: response.settings.preferred_shell || null,
        shellOptions: currentUserSettings.shellOptions ?? [],
        defaultEditorId: response.settings.default_editor_id || null,
        enablePreviewOnClick: response.settings.enable_preview_on_click ?? false,
        chatSubmitKey: response.settings.chat_submit_key ?? 'cmd_enter',
        reviewAutoMarkOnScroll: response.settings.review_auto_mark_on_scroll ?? true,
        lspAutoStartLanguages: response.settings.lsp_auto_start_languages ?? [],
        lspAutoInstallLanguages: response.settings.lsp_auto_install_languages ?? [],
        lspServerConfigs: response.settings.lsp_server_configs ?? {},
        loaded: true,
      });
    }
  });

  const createRequest = useRequest(async (state: EditorFormState) => {
    const created = await createEditor(
      {
        name: state.name,
        kind: state.kind,
        config: buildConfig(state),
        enabled: state.enabled,
      },
      { cache: 'no-store' }
    );
    applyEditors((prev: EditorOption[]) => [...prev, created]);
    setIsAdding(false);
  });

  const updateRequest = useRequest(async (editorId: string, state: EditorFormState) => {
    const updated = await updateEditor(
      editorId,
      {
        name: state.name,
        kind: state.kind,
        config: buildConfig(state),
        enabled: state.enabled,
      },
      { cache: 'no-store' }
    );
    applyEditors((prev: EditorOption[]) =>
      prev.map((editor: EditorOption) => (editor.id === editorId ? updated : editor))
    );
  });

  const deleteRequest = useRequest(async (editorId: string) => {
    await deleteEditor(editorId, { cache: 'no-store' });
    applyEditors((prev: EditorOption[]) =>
      prev.filter((editor: EditorOption) => editor.id !== editorId)
    );
    if (defaultEditorId === editorId) {
      setDefaultEditorId('');
      setBaselineDefaultId('');
      setUserSettings({
        ...currentUserSettings,
        defaultEditorId: null,
        loaded: true,
      });
    }
  });

  return (
    <SettingsPageTemplate
      title="Editors"
      description="Configure the included code editor and external editors"
      isDirty={isDirty}
      saveStatus={saveDefaultRequest.status}
      onSave={() => {
        void saveDefaultRequest.run();
      }}
    >
      <div className="space-y-6">
        {/* ── File Editor ────────────────────────────────────── */}
        <div className="space-y-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            File Editor
          </div>

          <div className="space-y-3">
            <div>
              <div className="text-sm font-medium text-foreground">Language Servers</div>
              <div className="text-xs text-muted-foreground">
                Auto-start language servers when opening files to get diagnostics, hover info, and go-to-definition. You can also toggle each server on/off per file.
                <br />
                When enabled, install your project&apos;s dependencies (e.g. <code className="text-[11px] bg-muted px-1 rounded">npm install</code> via repository setup scripts) to avoid missing type errors.
              </div>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              {LSP_LANGUAGE_OPTIONS.map((lang) => (
                <div key={lang.id} className="rounded-lg border border-border/60 bg-background px-4 py-3 space-y-2.5">
                  <div>
                    <div className="text-sm font-medium text-foreground">{lang.label}</div>
                    <div className="text-xs text-muted-foreground">{lang.binary}</div>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">Auto-start</span>
                    <Switch
                      checked={lspAutoStartLanguages.includes(lang.id)}
                      onCheckedChange={(checked) => toggleAutoStart(lang.id, checked === true)}
                    />
                  </div>
                  <div className="flex items-center gap-2">
                    <Checkbox
                      id={`lsp-install-${lang.id}`}
                      checked={lspAutoInstallLanguages.includes(lang.id)}
                      onCheckedChange={(checked) => toggleAutoInstall(lang.id, checked === true)}
                      className="h-3.5 w-3.5"
                    />
                    <label htmlFor={`lsp-install-${lang.id}`} className="text-xs text-muted-foreground cursor-pointer">
                      Auto-install if not found
                    </label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground cursor-help" />
                      </TooltipTrigger>
                      <TooltipContent side="top" className="max-w-[260px] text-xs">
                        {lang.installHint}
                      </TooltipContent>
                    </Tooltip>
                  </div>
                </div>
              ))}
            </div>
          </div>

          <div className="space-y-3">
            <div>
              <div className="text-sm font-medium text-foreground">Server Configuration</div>
              <div className="text-xs text-muted-foreground">
                Override settings sent to each language server via <code className="text-[11px] bg-muted px-1 rounded">workspace/configuration</code>. JSON format.
              </div>
            </div>
            {LSP_LANGUAGE_OPTIONS.map((lang) => {
              const isExpanded = expandedConfigLang === lang.id;
              const configStr = lspConfigStrings[lang.id] ?? '';
              const defaultConfig = LSP_DEFAULT_CONFIGS[lang.id];
              const hasDefaults = defaultConfig && Object.keys(defaultConfig).length > 0;
              const error = lspConfigErrors[lang.id];
              return (
                <div key={lang.id} className="rounded-lg border border-border/60 bg-background overflow-hidden">
                  <button
                    type="button"
                    className="flex w-full items-center justify-between px-4 py-2.5 text-left hover:bg-muted/50 transition-colors"
                    onClick={() => setExpandedConfigLang(isExpanded ? null : lang.id)}
                  >
                    <div className="flex items-center gap-2">
                      <span className="text-sm text-foreground">{lang.label}</span>
                      {configStr.trim() && (
                        <Badge variant="secondary" className="text-[10px] px-1.5 py-0">custom</Badge>
                      )}
                    </div>
                    <IconChevronDown
                      className={`h-4 w-4 text-muted-foreground transition-transform ${isExpanded ? 'rotate-180' : ''}`}
                    />
                  </button>
                  {isExpanded && (
                    <div className="border-t border-border/60 px-4 py-3 space-y-2">
                      {hasDefaults && (
                        <div className="text-[11px] text-muted-foreground">
                          Defaults: <code className="bg-muted px-1 rounded">{JSON.stringify(defaultConfig)}</code>
                        </div>
                      )}
                      <a
                        href={lang.docsUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
                      >
                        View available settings
                        <IconExternalLink className="h-3 w-3" />
                      </a>
                      <Textarea
                        value={configStr}
                        onChange={(e) => updateLspConfigString(lang.id, e.target.value)}
                        placeholder={hasDefaults ? JSON.stringify(defaultConfig, null, 2) : '{\n  \n}'}
                        className="font-mono text-xs min-h-[80px] resize-y"
                        rows={4}
                      />
                      {error && (
                        <div className="text-xs text-destructive">{error}</div>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>

        <Separator />

        {/* ── External Editors ───────────────────────────────── */}
        <div className="space-y-6">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            External Editors
          </div>

          <div className="space-y-2">
            <div className="text-sm font-medium text-foreground">Default</div>
            <div className="min-w-[280px]">
              <Combobox
                options={defaultOptions}
                value={defaultEditorId}
                onValueChange={(value) => {
                  if (!value) return;
                  setDefaultEditorId(value);
                }}
                placeholder="Select a default editor"
                searchPlaceholder="Search editors..."
                emptyMessage="No editor found."
                disabled={availableEditors.length === 0}
              />
            </div>
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div className="text-sm font-medium text-foreground">Custom Editors</div>
              <Button type="button" variant="outline" onClick={() => setIsAdding(true)}>
                Add custom editor
              </Button>
            </div>

            {isAdding && (
              <EditorForm
                title="New custom editor"
                initialState={defaultFormState()}
                onCancel={() => setIsAdding(false)}
                onSave={(state) => {
                  void createRequest.run(state);
                }}
                submitLabel="Add editor"
                isSaving={createRequest.isLoading}
              />
            )}

            <div className="space-y-3">
              {customEditors.length === 0 && !isAdding && (
                <div className="rounded-lg border border-dashed border-border/70 bg-muted/30 p-4 text-sm text-muted-foreground">
                  No custom editors yet.
                </div>
              )}
              {customEditors.map((editor) => {
                return (
                  <EditableCard
                    key={editor.id}
                    isEditing={editingId === editor.id}
                    historyId={`editor-${editor.id}`}
                    onOpen={() => setEditingId(editor.id)}
                    onClose={() => setEditingId(null)}
                    renderEdit={({ close }) => (
                      <EditorForm
                        title={`Edit ${editor.name}`}
                        initialState={formStateFromEditor(editor)}
                        onCancel={close}
                        onSave={(state) => {
                          void updateRequest.run(editor.id, state).then(() => {
                            close();
                          });
                        }}
                        submitLabel="Save changes"
                        isSaving={updateRequest.isLoading}
                      />
                    )}
                    renderPreview={({ open }) => (
                      <div
                        className="rounded-lg border border-border/70 bg-background p-4 flex items-center justify-between gap-3 cursor-pointer"
                        onClick={open}
                      >
                        <div className="min-w-0">
                          <div className="font-medium text-sm text-foreground truncate">
                            {editor.name}
                          </div>
                          <div className="text-xs text-muted-foreground truncate">
                            {getCustomEditorSummary(editor)}
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            className="cursor-pointer"
                            onClick={(event) => {
                              event.stopPropagation();
                              open();
                            }}
                          >
                            <IconEdit className="h-4 w-4" />
                            Edit
                          </Button>
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            className="cursor-pointer"
                            onClick={(event) => {
                              event.stopPropagation();
                              void deleteRequest.run(editor.id);
                            }}
                          >
                            <IconTrash className="h-4 w-4" />
                            Remove
                          </Button>
                        </div>
                      </div>
                    )}
                  />
                );
              })}
            </div>
          </div>

          {builtInEditors.length > 0 && (
            <div className="space-y-2">
              <div className="text-sm font-medium text-foreground">Supported Editors</div>
              <div className="grid gap-2 md:grid-cols-2">
                {builtInEditors.map((editor) => (
                  <div
                    key={editor.id}
                    className="rounded-lg border border-border/60 bg-background px-3 py-2 flex items-center justify-between"
                  >
                    <span className="text-sm text-foreground truncate">{editor.name}</span>
                    <Badge variant={editor.installed ? 'secondary' : 'outline'}>
                      {editor.installed ? 'Installed' : 'Not installed'}
                    </Badge>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </SettingsPageTemplate>
  );
}
