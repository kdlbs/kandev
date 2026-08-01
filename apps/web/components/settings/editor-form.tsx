"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { EditorOption } from "@/lib/types/http";
import { useSettingsSaveContributor } from "./settings-save-provider";
import { useTranslation } from "react-i18next";

type CustomKind = "custom_command" | "custom_remote_ssh" | "custom_hosted_url";

/** The `t` shape these helpers need; they are called from render, never at import. */
type TranslateKey = (key: string) => string;

export type EditorFormState = {
  name: string;
  kind: CustomKind;
  command: string;
  host: string;
  user: string;
  url: string;
  scheme: string;
  enabled: boolean;
};

/**
 * Holds catalog KEYS, not copy: a module-scope table is evaluated once at import,
 * so a `t()` here would freeze on the boot locale and never follow a switch.
 * `value` is the persisted editor kind and a string-literal union — it is
 * compared with `===`, used as a `<SelectItem value>`, and sent to the API, so it
 * must never be translated.
 */
const CUSTOM_KIND_OPTIONS: Array<{ value: CustomKind; labelKey: string }> = [
  { value: "custom_command", labelKey: "settings:command" },
  { value: "custom_remote_ssh", labelKey: "settings:vsCodeRemoteSsh" },
  { value: "custom_hosted_url", labelKey: "settings:hostedUrl" },
];

/** Placeholder tokens the launcher substitutes — identifiers, not copy. */
const PLACEHOLDER_HINT = "{cwd} {file} {rel} {line} {column}";
/** Example shell command shown in the field; a value the user types verbatim. */
const COMMAND_EXAMPLE = "code --goto {file}:{line}";
/** Example of the URL this kind builds — a URL, not translatable copy. */
const REMOTE_SSH_EXAMPLE = "vscode://vscode-remote/ssh-remote+user@host:/path/file:line";
/** Accepted URL schemes. The user types one of these verbatim into the field. */
const EDITOR_SCHEME_EXAMPLES = "vscode, cursor";

export function getCustomKindLabel(t: TranslateKey, kind: string) {
  const option = CUSTOM_KIND_OPTIONS.find((candidate) => candidate.value === kind);
  return option ? t(option.labelKey) : t("settings:customEditorType");
}

export function isCustomEditor(editor: EditorOption) {
  return editor.kind.startsWith("custom");
}

function editorConfigValue(editor: EditorOption, key: string) {
  if (!editor.config || typeof editor.config !== "object") {
    return "";
  }
  const value = (editor.config as Record<string, unknown>)[key];
  return typeof value === "string" ? value : "";
}

/**
 * The summary line is either the editor's own configuration (a command, a URL, a
 * host — user data, shown verbatim) or, when that is empty, the kind label,
 * which is copy. Only the latter goes through `t`.
 */
export function getCustomEditorSummary(t: TranslateKey, editor: EditorOption) {
  switch (editor.kind) {
    case "custom_command": {
      return editorConfigValue(editor, "command") || getCustomKindLabel(t, editor.kind);
    }
    case "custom_hosted_url": {
      return editorConfigValue(editor, "url") || getCustomKindLabel(t, editor.kind);
    }
    case "custom_remote_ssh": {
      const host = editorConfigValue(editor, "host");
      const user = editorConfigValue(editor, "user");
      if (host && user) return `${user}@${host}`;
      if (host) return host;
      return getCustomKindLabel(t, editor.kind);
    }
    default:
      return getCustomKindLabel(t, editor.kind);
  }
}

export function buildConfig(state: EditorFormState) {
  switch (state.kind) {
    case "custom_command":
      return { command: state.command };
    case "custom_remote_ssh":
      return {
        host: state.host,
        user: state.user || undefined,
        scheme: state.scheme || undefined,
      };
    case "custom_hosted_url":
      return { url: state.url };
    default:
      return {};
  }
}

export function defaultFormState(): EditorFormState {
  return {
    name: "",
    kind: "custom_command",
    command: "",
    host: "",
    user: "",
    url: "",
    scheme: "",
    enabled: true,
  };
}

function resolveEditorName(state: EditorFormState) {
  const trimmed = state.name.trim();
  if (trimmed) return trimmed;
  switch (state.kind) {
    case "custom_remote_ssh":
      return state.host.trim();
    case "custom_hosted_url":
      return state.url.trim();
    case "custom_command":
    default:
      return state.command.trim();
  }
}

function normalizeEditorState(state: EditorFormState) {
  const name = resolveEditorName(state);
  return name === state.name ? state : { ...state, name };
}

export function formStateFromEditor(editor: EditorOption): EditorFormState {
  return {
    name: editor.name,
    kind: (editor.kind as CustomKind) || "custom_command",
    command: editorConfigValue(editor, "command"),
    host: editorConfigValue(editor, "host"),
    user: editorConfigValue(editor, "user"),
    url: editorConfigValue(editor, "url"),
    scheme: editorConfigValue(editor, "scheme"),
    enabled: editor.enabled,
  };
}

export function resolveAvailableEditors(editors: EditorOption[]) {
  return editors.filter((editor) => {
    if (!editor.enabled) return false;
    if (editor.kind === "built_in") return editor.installed;
    return true;
  });
}

export function resolveDefaultEditorId(editors: EditorOption[], desiredId: string) {
  const available = resolveAvailableEditors(editors);
  if (desiredId && available.some((editor) => editor.id === desiredId)) {
    return desiredId;
  }
  if (!desiredId && available.length > 0) {
    return available[0].id;
  }
  return "";
}

type EditorFormProps = {
  title: string;
  initialState: EditorFormState;
  onCancel: () => void;
  onSave: (state: EditorFormState) => Promise<unknown> | void;
  onSaved?: () => void;
  submitLabel: string;
  isSaving: boolean;
  coordinatedSaveId?: string;
  dirtyWhenMounted?: boolean;
};

function EditorKindFields({
  state,
  baseline,
  setField,
}: {
  state: EditorFormState;
  baseline: EditorFormState;
  setField: <K extends keyof EditorFormState>(key: K, value: EditorFormState[K]) => void;
}) {
  const { t } = useTranslation();
  if (state.kind === "custom_command") {
    return (
      <div className="space-y-2">
        <Input
          value={state.command}
          data-settings-dirty={state.command !== baseline.command}
          onChange={(event) => setField("command", event.target.value)}
          placeholder={COMMAND_EXAMPLE}
        />
        <p className="text-xs text-muted-foreground">
          {t("settings:supportsPlaceholders", { placeholders: PLACEHOLDER_HINT })}
        </p>
      </div>
    );
  }
  if (state.kind === "custom_remote_ssh") {
    return (
      <div className="space-y-2">
        <Input
          value={state.host}
          data-settings-dirty={state.host !== baseline.host}
          onChange={(event) => setField("host", event.target.value)}
          placeholder="ssh-host.example.com"
        />
        <Input
          value={state.user}
          data-settings-dirty={state.user !== baseline.user}
          onChange={(event) => setField("user", event.target.value)}
          placeholder={t("settings:optionalUsername")}
        />
        <Input
          value={state.scheme}
          data-settings-dirty={state.scheme !== baseline.scheme}
          onChange={(event) => setField("scheme", event.target.value)}
          placeholder={t("settings:optionalScheme", { schemes: EDITOR_SCHEME_EXAMPLES })}
        />
        <p className="text-xs text-muted-foreground">
          {t("settings:opensAVsCodeRemoteSsh", { example: REMOTE_SSH_EXAMPLE })}
        </p>
      </div>
    );
  }
  if (state.kind === "custom_hosted_url") {
    return (
      <Input
        value={state.url}
        data-settings-dirty={state.url !== baseline.url}
        onChange={(event) => setField("url", event.target.value)}
        placeholder="https://code.example.com"
      />
    );
  }
  return null;
}

function EditorKindSelect({
  value,
  isDirty,
  onChange,
}: {
  value: CustomKind;
  isDirty: boolean;
  onChange: (kind: CustomKind) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select value={value} onValueChange={(next) => onChange(next as CustomKind)}>
      <SelectTrigger data-settings-dirty={isDirty}>
        <SelectValue placeholder={t("settings:editorType")} />
      </SelectTrigger>
      <SelectContent>
        <div className="px-2 py-1.5 text-xs text-muted-foreground border-b">
          {t("settings:editorType")}
        </div>
        {CUSTOM_KIND_OPTIONS.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {t(option.labelKey)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function EditorForm({
  title,
  initialState,
  onCancel,
  onSave,
  onSaved,
  submitLabel,
  isSaving,
  coordinatedSaveId,
  dirtyWhenMounted = false,
}: EditorFormProps) {
  const { t } = useTranslation();
  const [state, setState] = useState<EditorFormState>(initialState);
  const [baseline, setBaseline] = useState<EditorFormState>(initialState);
  const stateRef = useRef(state);
  stateRef.current = state;

  useEffect(() => {
    if (JSON.stringify(stateRef.current) !== JSON.stringify(baseline)) return;
    setState(initialState);
    setBaseline(initialState);
  }, [baseline, initialState]);

  const setField = <K extends keyof EditorFormState>(key: K, value: EditorFormState[K]) => {
    setState((prev) => ({ ...prev, [key]: value }));
  };

  const isValid = useMemo(() => {
    const resolvedName = resolveEditorName(state);
    if (!resolvedName) return false;
    if (state.kind === "custom_command") return Boolean(state.command.trim());
    if (state.kind === "custom_remote_ssh") return Boolean(state.host.trim());
    if (state.kind === "custom_hosted_url") return Boolean(state.url.trim());
    return false;
  }, [state]);

  const handleSave = async () => {
    const submitted = normalizeEditorState(state);
    await onSave(submitted);
    setBaseline(submitted);
    setState(submitted);
    onSaved?.();
  };
  const revision = JSON.stringify(state);
  const isCoordinatedDirty =
    Boolean(coordinatedSaveId) && (dirtyWhenMounted || revision !== JSON.stringify(baseline));
  const normalizedState = normalizeEditorState(state);
  useSettingsSaveContributor({
    id: coordinatedSaveId ?? `editor-form-local:${title}`,
    revision,
    isDirty: isCoordinatedDirty,
    canSave: isValid,
    invalidReason: isValid ? undefined : t("settings:completeTheRequiredEditorFieldsBefore"),
    save: async () => {
      const submitted = normalizedState;
      await onSave(submitted);
      setBaseline(submitted);
      if (JSON.stringify(stateRef.current) === JSON.stringify(state)) onSaved?.();
    },
    discard: () => setState(baseline),
  });

  return (
    <div
      className="rounded-lg border border-border/70 bg-background p-4 space-y-4"
      data-settings-dirty={isCoordinatedDirty}
    >
      <div className="text-sm font-medium text-foreground">{title}</div>
      <Input
        value={state.name}
        data-settings-dirty={state.name !== baseline.name}
        onChange={(event) => setField("name", event.target.value)}
        placeholder={t("settings:editorName")}
      />
      <EditorKindSelect
        value={state.kind}
        isDirty={state.kind !== baseline.kind}
        onChange={(kind) => setField("kind", kind)}
      />
      <EditorKindFields state={state} baseline={baseline} setField={setField} />
      <div className="flex items-center justify-end gap-2">
        <Button type="button" variant="outline" onClick={onCancel} disabled={isSaving}>
          {t("settings:cancel")}
        </Button>
        {!coordinatedSaveId && (
          <Button
            type="button"
            onClick={() => void handleSave()}
            disabled={isSaving || !isValid}
            className="cursor-pointer"
          >
            {submitLabel}
          </Button>
        )}
      </div>
    </div>
  );
}
