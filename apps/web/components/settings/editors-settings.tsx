"use client";

import { useCallback, useEffect, useMemo } from "react";
import {
  IconEdit,
  IconTrash,
  IconChevronDown,
  IconExternalLink,
  IconInfoCircle,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Separator } from "@kandev/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Switch } from "@kandev/ui/switch";
import { Textarea } from "@kandev/ui/textarea";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { EditableCard } from "@/components/settings/editable-card";
import {
  EditorForm,
  type EditorFormState,
  defaultFormState,
  formStateFromEditor,
  getCustomEditorSummary,
} from "@/components/settings/editor-form";
import { LSP_DEFAULT_CONFIGS } from "@/lib/lsp/lsp-client-manager";
import type { EditorOption } from "@/lib/types/http";
import type { RequestStatus } from "@/lib/http/use-request";
import {
  useEditorsSettingsState,
  useLspConfigActions,
  useApplyEditors,
  useEditorRequests,
  useSaveRequest,
  buildDefaultEditorOptions,
  sortCustomEditors,
  resolveAvailableEditors,
  isCustomEditor,
  type EditorsSettingsState,
} from "@/components/settings/editors-settings-state";

const LSP_LANGUAGE_OPTIONS = [
  {
    id: "typescript",
    label: "TypeScript / JavaScript",
    binary: "typescript-language-server",
    docsUrl:
      "https://github.com/typescript-language-server/typescript-language-server#workspace-configuration",
    installHint:
      "Installs typescript-language-server and typescript via npm into ~/.kandev/lsp-servers/",
  },
  {
    id: "go",
    label: "Go",
    binary: "gopls",
    docsUrl: "https://github.com/golang/tools/blob/master/gopls/doc/settings.md",
    installHint: 'Runs "go install golang.org/x/tools/gopls@latest". Requires Go to be installed.',
  },
  {
    id: "rust",
    label: "Rust",
    binary: "rust-analyzer",
    docsUrl: "https://rust-analyzer.github.io/book/configuration.html",
    installHint:
      "Downloads the rust-analyzer binary from GitHub releases into ~/.kandev/lsp-servers/",
  },
  {
    id: "python",
    label: "Python",
    binary: "pyright-langserver",
    docsUrl: "https://microsoft.github.io/pyright/#/settings",
    installHint: "Installs pyright via npm into ~/.kandev/lsp-servers/",
  },
];

type LspLanguageCardsProps = {
  lspAutoStartLanguages: string[];
  lspAutoInstallLanguages: string[];
  toggleAutoStart: (langId: string, checked: boolean) => void;
  toggleAutoInstall: (langId: string, checked: boolean) => void;
};

function LspLanguageCards({
  lspAutoStartLanguages,
  lspAutoInstallLanguages,
  toggleAutoStart,
  toggleAutoInstall,
}: LspLanguageCardsProps) {
  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-medium text-foreground">Language Servers</div>
        <div className="text-xs text-muted-foreground">
          Auto-start language servers when opening files to get diagnostics, hover info, and
          go-to-definition. You can also toggle each server on/off per file.
          <br />
          When enabled, install your project&apos;s dependencies (e.g.{" "}
          <code className="text-[11px] bg-muted px-1 rounded">npm install</code> via repository
          setup scripts) to avoid missing type errors.
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {LSP_LANGUAGE_OPTIONS.map((lang) => (
          <div
            key={lang.id}
            className="rounded-lg border border-border/60 bg-background px-4 py-3 space-y-2.5"
          >
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
              <label
                htmlFor={`lsp-install-${lang.id}`}
                className="text-xs text-muted-foreground cursor-pointer"
              >
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
  );
}

type LspServerConfigSectionProps = {
  lspConfigStrings: Record<string, string>;
  lspConfigErrors: Record<string, string>;
  expandedConfigLang: string | null;
  setExpandedConfigLang: (lang: string | null) => void;
  updateLspConfigString: (langId: string, value: string) => void;
};

function LspServerConfigSection({
  lspConfigStrings,
  lspConfigErrors,
  expandedConfigLang,
  setExpandedConfigLang,
  updateLspConfigString,
}: LspServerConfigSectionProps) {
  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-medium text-foreground">Server Configuration</div>
        <div className="text-xs text-muted-foreground">
          Override settings sent to each language server via{" "}
          <code className="text-[11px] bg-muted px-1 rounded">workspace/configuration</code>. JSON
          format.
        </div>
      </div>
      {LSP_LANGUAGE_OPTIONS.map((lang) => {
        const isExpanded = expandedConfigLang === lang.id;
        const configStr = lspConfigStrings[lang.id] ?? "";
        const defaultConfig = LSP_DEFAULT_CONFIGS[lang.id];
        const hasDefaults = defaultConfig && Object.keys(defaultConfig).length > 0;
        const error = lspConfigErrors[lang.id];
        return (
          <div
            key={lang.id}
            className="rounded-lg border border-border/60 bg-background overflow-hidden"
          >
            <button
              type="button"
              className="flex w-full items-center justify-between px-4 py-2.5 text-left hover:bg-muted/50 transition-colors"
              onClick={() => setExpandedConfigLang(isExpanded ? null : lang.id)}
            >
              <div className="flex items-center gap-2">
                <span className="text-sm text-foreground">{lang.label}</span>
                {configStr.trim() && (
                  <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
                    custom
                  </Badge>
                )}
              </div>
              <IconChevronDown
                className={`h-4 w-4 text-muted-foreground transition-transform ${isExpanded ? "rotate-180" : ""}`}
              />
            </button>
            {isExpanded && (
              <div className="border-t border-border/60 px-4 py-3 space-y-2">
                {hasDefaults && (
                  <div className="text-[11px] text-muted-foreground">
                    Defaults:{" "}
                    <code className="bg-muted px-1 rounded">{JSON.stringify(defaultConfig)}</code>
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
                  placeholder={hasDefaults ? JSON.stringify(defaultConfig, null, 2) : "{\n  \n}"}
                  className="font-mono text-xs min-h-[80px] resize-y"
                  rows={4}
                />
                {error && <div className="text-xs text-destructive">{error}</div>}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

type EditorRequestProps = { isLoading: boolean; status: RequestStatus };
type CreateReq = EditorRequestProps & { run: (state: EditorFormState) => Promise<void> };
type UpdateReq = EditorRequestProps & {
  run: (id: string, state: EditorFormState) => Promise<void>;
};
type DeleteReq = EditorRequestProps & { run: (id: string) => Promise<void> };

type CustomEditorsListProps = {
  customEditors: EditorOption[];
  editingId: string | null;
  setEditingId: (id: string | null) => void;
  isAdding: boolean;
  setIsAdding: (adding: boolean) => void;
  createRequest: CreateReq;
  updateRequest: UpdateReq;
  deleteRequest: DeleteReq;
};

type CustomEditorRowProps = {
  editor: EditorOption;
  editingId: string | null;
  setEditingId: (id: string | null) => void;
  updateRequest: UpdateReq;
  deleteRequest: DeleteReq;
};

function CustomEditorRow({
  editor,
  editingId,
  setEditingId,
  updateRequest,
  deleteRequest,
}: CustomEditorRowProps) {
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
}

function CustomEditorsList({
  customEditors,
  editingId,
  setEditingId,
  isAdding,
  setIsAdding,
  createRequest,
  updateRequest,
  deleteRequest,
}: CustomEditorsListProps) {
  return (
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
        {customEditors.map((editor) => (
          <CustomEditorRow
            key={editor.id}
            editor={editor}
            editingId={editingId}
            setEditingId={setEditingId}
            updateRequest={updateRequest}
            deleteRequest={deleteRequest}
          />
        ))}
      </div>
    </div>
  );
}

type ExternalEditorsSectionProps = {
  defaultOptions: ComboboxOption[];
  defaultEditorId: string;
  availableEditors: EditorOption[];
  builtInEditors: EditorOption[];
  onDefaultEditorChange: (value: string) => void;
  customEditors: EditorOption[];
  editingId: string | null;
  setEditingId: (id: string | null) => void;
  isAdding: boolean;
  setIsAdding: (adding: boolean) => void;
  createRequest: CreateReq;
  updateRequest: UpdateReq;
  deleteRequest: DeleteReq;
};

function ExternalEditorsSection({
  defaultOptions,
  defaultEditorId,
  availableEditors,
  builtInEditors,
  onDefaultEditorChange,
  customEditors,
  editingId,
  setEditingId,
  isAdding,
  setIsAdding,
  createRequest,
  updateRequest,
  deleteRequest,
}: ExternalEditorsSectionProps) {
  return (
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
              onDefaultEditorChange(value);
            }}
            placeholder="Select a default editor"
            searchPlaceholder="Search editors..."
            emptyMessage="No editor found."
            disabled={availableEditors.length === 0}
          />
        </div>
      </div>
      <CustomEditorsList
        customEditors={customEditors}
        editingId={editingId}
        setEditingId={setEditingId}
        isAdding={isAdding}
        setIsAdding={setIsAdding}
        createRequest={createRequest}
        updateRequest={updateRequest}
        deleteRequest={deleteRequest}
      />
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
                <Badge variant={editor.installed ? "secondary" : "outline"}>
                  {editor.installed ? "Installed" : "Not installed"}
                </Badge>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function useEditorsIsDirty(state: EditorsSettingsState) {
  const {
    defaultEditorId,
    baselineDefaultId,
    lspAutoStartLanguages,
    baselineLspAutoStart,
    lspAutoInstallLanguages,
    baselineLspAutoInstall,
    lspConfigStrings,
    baselineLspConfigStrings,
  } = state;
  const lspAutoStartDirty =
    lspAutoStartLanguages.length !== baselineLspAutoStart.length ||
    lspAutoStartLanguages.some((id) => !baselineLspAutoStart.includes(id));
  const lspAutoInstallDirty =
    lspAutoInstallLanguages.length !== baselineLspAutoInstall.length ||
    lspAutoInstallLanguages.some((id) => !baselineLspAutoInstall.includes(id));
  const lspConfigsDirty =
    JSON.stringify(lspConfigStrings) !== JSON.stringify(baselineLspConfigStrings);
  return (
    defaultEditorId !== baselineDefaultId ||
    lspAutoStartDirty ||
    lspAutoInstallDirty ||
    lspConfigsDirty
  );
}

export function EditorsSettings() {
  const state = useEditorsSettingsState();
  const {
    setLspAutoStartLanguages,
    setLspAutoInstallLanguages,
    setLspConfigStrings,
    setLspConfigErrors,
    setEditors,
    editors,
  } = state;
  const applyEditors = useApplyEditors(state);
  const saveDefaultRequest = useSaveRequest(state);
  const { createRequest, updateRequest, deleteRequest } = useEditorRequests(state, applyEditors);
  const { updateLspConfigString } = useLspConfigActions(setLspConfigStrings, setLspConfigErrors);
  const isDirty = useEditorsIsDirty(state);

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

  const customEditors = useMemo(() => sortCustomEditors(editors.filter(isCustomEditor)), [editors]);
  const builtInEditors = useMemo(() => editors.filter((editor) => !isCustomEditor(editor)), [editors]);
  const availableEditors = useMemo(() => resolveAvailableEditors(editors), [editors]);
  const defaultOptions = useMemo<ComboboxOption[]>(
    () => buildDefaultEditorOptions(availableEditors, state.defaultEditorId),
    [availableEditors, state.defaultEditorId],
  );

  useEffect(() => {
    setEditors(editors);
  }, [editors, setEditors]);

  return (
    <SettingsPageTemplate
      title="Editors"
      description="Configure the included code editor and external editors"
      isDirty={isDirty}
      saveStatus={saveDefaultRequest.status}
      onSave={() => void saveDefaultRequest.run()}
    >
      <div className="space-y-6">
        <div className="space-y-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            File Editor
          </div>
          <LspLanguageCards
            lspAutoStartLanguages={state.lspAutoStartLanguages}
            lspAutoInstallLanguages={state.lspAutoInstallLanguages}
            toggleAutoStart={toggleAutoStart}
            toggleAutoInstall={toggleAutoInstall}
          />
          <LspServerConfigSection
            lspConfigStrings={state.lspConfigStrings}
            lspConfigErrors={state.lspConfigErrors}
            expandedConfigLang={state.expandedConfigLang}
            setExpandedConfigLang={state.setExpandedConfigLang}
            updateLspConfigString={updateLspConfigString}
          />
        </div>
        <Separator />
        <ExternalEditorsSection
          defaultOptions={defaultOptions}
          defaultEditorId={state.defaultEditorId}
          availableEditors={availableEditors}
          builtInEditors={builtInEditors}
          onDefaultEditorChange={state.setDefaultEditorId}
          customEditors={customEditors}
          editingId={state.editingId}
          setEditingId={state.setEditingId}
          isAdding={state.isAdding}
          setIsAdding={state.setIsAdding}
          createRequest={createRequest}
          updateRequest={updateRequest}
          deleteRequest={deleteRequest}
        />
      </div>
    </SettingsPageTemplate>
  );
}
