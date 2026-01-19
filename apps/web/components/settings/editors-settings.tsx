'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { IconEdit, IconTrash } from '@tabler/icons-react';
import { Badge } from '@kandev/ui/badge';
import { Button } from '@kandev/ui/button';
import { Input } from '@kandev/ui/input';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@kandev/ui/select';
import { SettingsPageTemplate } from '@/components/settings/settings-page-template';
import { Combobox, type ComboboxOption } from '@/components/combobox';
import { useAppStore } from '@/components/state-provider';
import { createEditor, deleteEditor, updateEditor, updateUserSettings } from '@/lib/http';
import { useRequest } from '@/lib/http/use-request';
import type { EditorOption, UserSettings } from '@/lib/types/http';

type EditorsSettingsProps = {
  initialEditors: EditorOption[];
  initialSettings: UserSettings | null;
};

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

export function EditorsSettings({ initialEditors, initialSettings }: EditorsSettingsProps) {
  const setEditors = useAppStore((state) => state.setEditors);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const currentUserSettings = useAppStore((state) => state.userSettings);
  const [editors, setEditorItems] = useState<EditorOption[]>(() => initialEditors ?? []);
  const initialDefaultId = resolveDefaultEditorId(
    initialEditors ?? [],
    initialSettings?.default_editor_id ?? ''
  );
  const [defaultEditorId, setDefaultEditorId] = useState(initialDefaultId);
  const [baselineDefaultId, setBaselineDefaultId] = useState(initialDefaultId);
  const [isAdding, setIsAdding] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const customEditors = useMemo(() => editors.filter(isCustomEditor), [editors]);
  const builtInEditors = useMemo(() => editors.filter((editor) => !isCustomEditor(editor)), [editors]);
  const availableEditors = useMemo(() => resolveAvailableEditors(editors), [editors]);

  const applyEditors = useCallback(
    (updater: EditorOption[] | ((prev: EditorOption[]) => EditorOption[])) => {
      setEditorItems((prev) => {
        const next = typeof updater === 'function' ? updater(prev) : updater;
        setEditors(next);
        const resolvedDefault = resolveDefaultEditorId(next, defaultEditorId);
        if (resolvedDefault !== defaultEditorId) {
          setDefaultEditorId(resolvedDefault);
          setBaselineDefaultId(resolvedDefault);
        }
        return next;
      });
    },
    [defaultEditorId, setEditors]
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

  const isDirty = defaultEditorId !== baselineDefaultId;

  const saveDefaultRequest = useRequest(async () => {
    const fallbackSettings = {
      workspace_id: currentUserSettings.workspaceId ?? '',
      board_id: currentUserSettings.boardId ?? '',
      repository_ids: currentUserSettings.repositoryIds ?? [],
    };
    const payload = {
      workspace_id: initialSettings?.workspace_id ?? fallbackSettings.workspace_id,
      board_id: initialSettings?.board_id ?? fallbackSettings.board_id,
      repository_ids: initialSettings?.repository_ids ?? fallbackSettings.repository_ids,
      default_editor_id: defaultEditorId || undefined,
    };
    const response = await updateUserSettings(payload, { cache: 'no-store' });
    setBaselineDefaultId(defaultEditorId);
    if (response?.settings) {
      setUserSettings({
        workspaceId: response.settings.workspace_id || null,
        boardId: response.settings.board_id || null,
        repositoryIds: response.settings.repository_ids ?? [],
        preferredShell: response.settings.preferred_shell || null,
        defaultEditorId: response.settings.default_editor_id || null,
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
    setEditingId(null);
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
      description="Choose which editor opens files and worktrees"
      isDirty={isDirty}
      saveStatus={saveDefaultRequest.status}
      onSave={() => {
        void saveDefaultRequest.run();
      }}
    >
      <div className="space-y-8">
        <div className="space-y-2">
          <div className="text-sm font-medium text-foreground">Default editor</div>
          <div className="flex flex-wrap items-center gap-3">
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
        </div>

        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <div className="text-sm font-medium text-foreground">Custom editors</div>
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
              if (editingId === editor.id) {
                return (
                  <EditorForm
                    key={editor.id}
                    title={`Edit ${editor.name}`}
                    initialState={formStateFromEditor(editor)}
                    onCancel={() => setEditingId(null)}
                    onSave={(state) => {
                      void updateRequest.run(editor.id, state);
                    }}
                    submitLabel="Save changes"
                    isSaving={updateRequest.isLoading}
                  />
                );
              }
              return (
                <div
                  key={editor.id}
                  className="rounded-lg border border-border/70 bg-background p-4 flex items-center justify-between gap-3"
                >
                  <div className="min-w-0">
                    <div className="font-medium text-sm text-foreground truncate">{editor.name}</div>
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
                      onClick={() => setEditingId(editor.id)}
                    >
                      <IconEdit className="h-4 w-4" />
                      Edit
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="cursor-pointer"
                      onClick={() => {
                        void deleteRequest.run(editor.id);
                      }}
                    >
                      <IconTrash className="h-4 w-4" />
                      Remove
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {builtInEditors.length === 0 ? null : (
          <div className="space-y-2">
            <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Supported Editors
            </div>
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
    </SettingsPageTemplate>
  );
}
