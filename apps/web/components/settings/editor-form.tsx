"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { EditorOption } from "@/lib/types/http";
import { useSettingsSaveContributor } from "./settings-save-provider";

type CustomKind = "custom_command" | "custom_remote_ssh" | "custom_hosted_url";

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

const CUSTOM_KIND_OPTIONS: Array<{ value: CustomKind; label: string }> = [
  { value: "custom_command", label: "Command" },
  { value: "custom_remote_ssh", label: "VS Code Remote SSH" },
  { value: "custom_hosted_url", label: "Hosted URL" },
];

const PLACEHOLDER_HINT = "{cwd} {file} {rel} {line} {column}";

export function getCustomKindLabel(kind: string) {
  return CUSTOM_KIND_OPTIONS.find((option) => option.value === kind)?.label ?? "Custom";
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

export function getCustomEditorSummary(editor: EditorOption) {
  switch (editor.kind) {
    case "custom_command": {
      return editorConfigValue(editor, "command") || getCustomKindLabel(editor.kind);
    }
    case "custom_hosted_url": {
      return editorConfigValue(editor, "url") || getCustomKindLabel(editor.kind);
    }
    case "custom_remote_ssh": {
      const host = editorConfigValue(editor, "host");
      const user = editorConfigValue(editor, "user");
      if (host && user) return `${user}@${host}`;
      if (host) return host;
      return getCustomKindLabel(editor.kind);
    }
    default:
      return getCustomKindLabel(editor.kind);
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
  if (state.kind === "custom_command") {
    return (
      <div className="space-y-2">
        <Input
          value={state.command}
          data-settings-dirty={state.command !== baseline.command}
          onChange={(event) => setField("command", event.target.value)}
          placeholder="code --goto {file}:{line}"
        />
        <p className="text-xs text-muted-foreground">Supports placeholders: {PLACEHOLDER_HINT}</p>
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
          placeholder="optional username"
        />
        <Input
          value={state.scheme}
          data-settings-dirty={state.scheme !== baseline.scheme}
          onChange={(event) => setField("scheme", event.target.value)}
          placeholder="optional scheme (vscode, cursor)"
        />
        <p className="text-xs text-muted-foreground">
          Opens a VS Code Remote SSH URL using the scheme (example:{" "}
          vscode://vscode-remote/ssh-remote+user@host:/path/file:line).
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

export function EditorForm({
  title,
  initialState,
  onCancel,
  onSave,
  onSaved,
  submitLabel,
  isSaving,
  coordinatedSaveId,
}: EditorFormProps) {
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
    const resolvedName = resolveEditorName(state);
    const submitted = resolvedName === state.name ? state : { ...state, name: resolvedName };
    await onSave(submitted);
    setBaseline(submitted);
    setState(submitted);
    onSaved?.();
  };
  const revision = JSON.stringify(state);
  const normalizedState =
    resolveEditorName(state) === state.name ? state : { ...state, name: resolveEditorName(state) };
  useSettingsSaveContributor({
    id: coordinatedSaveId ?? `editor-form-local:${title}`,
    revision,
    isDirty: Boolean(coordinatedSaveId) && revision !== JSON.stringify(baseline),
    canSave: isValid,
    invalidReason: isValid ? undefined : "Complete the required editor fields before saving.",
    save: async () => {
      const submitted = normalizedState;
      await onSave(submitted);
      setBaseline(submitted);
      if (JSON.stringify(stateRef.current) === JSON.stringify(state)) onSaved?.();
    },
    discard: () => setState(baseline),
  });

  return (
    <div className="rounded-lg border border-border/70 bg-background p-4 space-y-4">
      <div className="text-sm font-medium text-foreground">{title}</div>
      <Input
        value={state.name}
        data-settings-dirty={state.name !== baseline.name}
        onChange={(event) => setField("name", event.target.value)}
        placeholder="Editor name"
      />
      <Select value={state.kind} onValueChange={(value) => setField("kind", value as CustomKind)}>
        <SelectTrigger data-settings-dirty={state.kind !== baseline.kind}>
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
      <EditorKindFields state={state} baseline={baseline} setField={setField} />
      <div className="flex items-center justify-end gap-2">
        <Button type="button" variant="outline" onClick={onCancel} disabled={isSaving}>
          Cancel
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
