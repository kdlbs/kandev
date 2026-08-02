"use client";

import { useEffect, useMemo } from "react";
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
  useLspLanguageToggles,
  useApplyEditors,
  useEditorRequests,
  useSaveRequest,
  buildDefaultEditorOptions,
  sortCustomEditors,
  resolveAvailableEditors,
  isCustomEditor,
  type EditorsSettingsState,
} from "@/components/settings/editors-settings-state";
import { isDraftEntryDirty, isEditorsSettingsDirty } from "./settings-dirty";
import { LSP_LANGUAGE_OPTIONS } from "./lsp-language-options";
import { Trans, useTranslation } from "react-i18next";

/**
 * Code identifiers rendered inside `<Trans>` copy. They are passed as
 * interpolation VALUES rather than left in the tag body: text inside a `<n>` tag
 * is part of the message a translator edits, and translating a command name or
 * an LSP method breaks the thing it names.
 */
const NPM_INSTALL_COMMAND = "npm install";
const LSP_CONFIG_METHOD = "workspace/configuration";

type LspLanguageCardsProps = {
  lspAutoStartLanguages: string[];
  lspAutoInstallLanguages: string[];
  baselineLspAutoStart: string[];
  baselineLspAutoInstall: string[];
  toggleAutoStart: (langId: string, checked: boolean) => void;
  toggleAutoInstall: (langId: string, checked: boolean) => void;
};

function LspLanguageCards({
  lspAutoStartLanguages,
  lspAutoInstallLanguages,
  baselineLspAutoStart,
  baselineLspAutoInstall,
  toggleAutoStart,
  toggleAutoInstall,
}: LspLanguageCardsProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-medium text-foreground">{t("settings:languageServers")}</div>
        <div className="text-xs text-muted-foreground">
          {t("settings:autoStartLanguageServersWhenOpening")}
          <br />
          <Trans
            i18nKey="settings:whenEnabledInstallYourProjectS"
            values={{ command: NPM_INSTALL_COMMAND }}
          >
            When enabled, install your project&apos;s dependencies (e.g.{" "}
            <code className="text-[11px] bg-muted px-1 rounded">{NPM_INSTALL_COMMAND}</code> via
            repository setup scripts) to avoid missing type errors.
          </Trans>
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {LSP_LANGUAGE_OPTIONS.map((lang) => {
          const autoStartDirty =
            lspAutoStartLanguages.includes(lang.id) !== baselineLspAutoStart.includes(lang.id);
          const autoInstallDirty =
            lspAutoInstallLanguages.includes(lang.id) !== baselineLspAutoInstall.includes(lang.id);
          return (
            <div
              key={lang.id}
              className="rounded-lg border border-border/60 bg-background px-4 py-3 space-y-2.5"
              data-settings-dirty={autoStartDirty || autoInstallDirty}
            >
              <div>
                <div className="text-sm font-medium text-foreground">{lang.label}</div>
                <div className="text-xs text-muted-foreground">{lang.binary}</div>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-xs text-muted-foreground">{t("settings:autoStart")}</span>
                <Switch
                  checked={lspAutoStartLanguages.includes(lang.id)}
                  onCheckedChange={(checked) => toggleAutoStart(lang.id, checked === true)}
                  data-settings-dirty={autoStartDirty}
                />
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id={`lsp-install-${lang.id}`}
                  checked={lspAutoInstallLanguages.includes(lang.id)}
                  onCheckedChange={(checked) => toggleAutoInstall(lang.id, checked === true)}
                  className="h-3.5 w-3.5"
                  data-settings-dirty={autoInstallDirty}
                />
                <label
                  htmlFor={`lsp-install-${lang.id}`}
                  className="text-xs text-muted-foreground cursor-pointer"
                >
                  {t("settings:autoInstallIfNotFound")}
                </label>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground cursor-help" />
                  </TooltipTrigger>
                  <TooltipContent side="top" className="max-w-[260px] text-xs">
                    {t(lang.installHintKey, lang.installHintValues)}
                  </TooltipContent>
                </Tooltip>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

type LspServerConfigSectionProps = {
  lspConfigStrings: Record<string, string>;
  baselineLspConfigStrings: Record<string, string>;
  lspConfigErrors: Record<string, string>;
  expandedConfigLang: string | null;
  setExpandedConfigLang: (lang: string | null) => void;
  updateLspConfigString: (langId: string, value: string) => void;
};

function LspServerConfigSection({
  lspConfigStrings,
  baselineLspConfigStrings,
  lspConfigErrors,
  expandedConfigLang,
  setExpandedConfigLang,
  updateLspConfigString,
}: LspServerConfigSectionProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <div>
        <div className="text-sm font-medium text-foreground">
          {t("settings:serverConfiguration")}
        </div>
        <div className="text-xs text-muted-foreground">
          <Trans
            i18nKey="settings:overrideSettingsSentToEachLanguage"
            values={{ method: LSP_CONFIG_METHOD }}
          >
            Override settings sent to each language server via{" "}
            <code className="text-[11px] bg-muted px-1 rounded">{LSP_CONFIG_METHOD}</code>. JSON
            format.
          </Trans>
        </div>
      </div>
      {LSP_LANGUAGE_OPTIONS.map((lang) => {
        const isExpanded = expandedConfigLang === lang.id;
        const configStr = lspConfigStrings[lang.id] ?? "";
        const defaultConfig = LSP_DEFAULT_CONFIGS[lang.id];
        const hasDefaults = defaultConfig && Object.keys(defaultConfig).length > 0;
        const error = lspConfigErrors[lang.id];
        const isDirty = isDraftEntryDirty(lspConfigStrings, baselineLspConfigStrings, lang.id);
        return (
          <div
            key={lang.id}
            className="rounded-lg border border-border/60 bg-background overflow-hidden"
            data-settings-dirty={isDirty}
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
                    {t("settings:lspConfigCustomBadge")}
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
                    {t("settings:defaults")}{" "}
                    <code className="bg-muted px-1 rounded">{JSON.stringify(defaultConfig)}</code>
                  </div>
                )}
                <a
                  href={lang.docsUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
                >
                  {t("settings:viewAvailableSettings")}
                  <IconExternalLink className="h-3 w-3" />
                </a>
                <Textarea
                  value={configStr}
                  onChange={(e) => updateLspConfigString(lang.id, e.target.value)}
                  placeholder={hasDefaults ? JSON.stringify(defaultConfig, null, 2) : "{\n  \n}"}
                  className="font-mono text-xs min-h-[80px] resize-y"
                  rows={4}
                  data-settings-dirty={isDirty}
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
  const { t } = useTranslation();
  return (
    <EditableCard
      key={editor.id}
      isEditing={editingId === editor.id}
      historyId={`editor-${editor.id}`}
      onOpen={() => setEditingId(editor.id)}
      onClose={() => setEditingId(null)}
      renderEdit={({ close }) => (
        <EditorForm
          title={t("settings:editEditor", { name: editor.name })}
          initialState={formStateFromEditor(editor)}
          onCancel={close}
          onSave={(state) => updateRequest.run(editor.id, state)}
          onSaved={close}
          submitLabel={t("settings:saveChanges")}
          isSaving={updateRequest.isLoading}
          coordinatedSaveId={`custom-editor:${editor.id}`}
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
              {getCustomEditorSummary(t, editor)}
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
              {t("settings:edit")}
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
              {t("settings:remove")}
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
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="text-sm font-medium text-foreground">{t("settings:customEditors")}</div>
        <Button type="button" variant="outline" onClick={() => setIsAdding(true)}>
          {t("settings:addCustomEditor")}
        </Button>
      </div>
      {isAdding && (
        <EditorForm
          title={t("settings:newCustomEditor")}
          initialState={defaultFormState()}
          onCancel={() => setIsAdding(false)}
          onSave={(state) => createRequest.run(state)}
          onSaved={() => setIsAdding(false)}
          submitLabel={t("settings:addEditor")}
          isSaving={createRequest.isLoading}
          coordinatedSaveId="custom-editor:new"
          dirtyWhenMounted
        />
      )}
      <div className="space-y-3">
        {customEditors.length === 0 && !isAdding && (
          <div className="rounded-lg border border-dashed border-border/70 bg-muted/30 p-4 text-sm text-muted-foreground">
            {t("settings:noCustomEditorsYet")}
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

type EditorsSectionProps = {
  defaultOptions: ComboboxOption[];
  defaultEditorId: string;
  baselineDefaultId: string;
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

function EditorsSection({
  defaultOptions,
  defaultEditorId,
  baselineDefaultId,
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
}: EditorsSectionProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-6">
      <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {t("settings:editors")}
      </div>
      <div className="space-y-2">
        <div className="text-sm font-medium text-foreground">{t("settings:default")}</div>
        <div
          className="min-w-[280px] rounded-md border border-transparent"
          data-settings-dirty={defaultEditorId !== baselineDefaultId}
        >
          <Combobox
            options={defaultOptions}
            value={defaultEditorId}
            onValueChange={(value) => {
              if (!value) return;
              onDefaultEditorChange(value);
            }}
            placeholder={t("settings:selectADefaultEditor")}
            searchPlaceholder={t("settings:searchEditors")}
            emptyMessage={t("settings:noEditorFound")}
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
          <div className="text-sm font-medium text-foreground">
            {t("settings:supportedEditors")}
          </div>
          <div className="grid gap-2 md:grid-cols-2">
            {builtInEditors.map((editor) => (
              <div
                key={editor.id}
                className="rounded-lg border border-border/60 bg-background px-3 py-2 flex items-center justify-between"
              >
                <span className="text-sm text-foreground truncate">{editor.name}</span>
                <Badge variant={editor.installed ? "secondary" : "outline"}>
                  {editor.installed ? t("settings:installed") : t("settings:notInstalled")}
                </Badge>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function getEditorsSaveRevision(state: EditorsSettingsState): string {
  return JSON.stringify({
    defaultEditorId: state.defaultEditorId,
    lspAutoStartLanguages: state.lspAutoStartLanguages,
    lspAutoInstallLanguages: state.lspAutoInstallLanguages,
    lspConfigStrings: state.lspConfigStrings,
  });
}

function useSyncEditors(editors: EditorOption[], setEditors: (editors: EditorOption[]) => void) {
  useEffect(() => setEditors(editors), [editors, setEditors]);
}

export function EditorsSettings() {
  const { t } = useTranslation();
  const state = useEditorsSettingsState();
  const { setLspConfigStrings, setLspConfigErrors, setEditors, editors } = state;
  const applyEditors = useApplyEditors(state);
  const saveDefaultRequest = useSaveRequest(state);
  const { createRequest, updateRequest, deleteRequest } = useEditorRequests(state, applyEditors);
  const { updateLspConfigString } = useLspConfigActions(setLspConfigStrings, setLspConfigErrors);
  const { toggleAutoStart, toggleAutoInstall } = useLspLanguageToggles(state);
  const isDirty = isEditorsSettingsDirty(state);
  const saveRevision = getEditorsSaveRevision(state);
  const hasInvalidConfig = Object.keys(state.lspConfigErrors).length > 0;

  const customEditors = useMemo(() => sortCustomEditors(editors.filter(isCustomEditor)), [editors]);
  const builtInEditors = useMemo(
    () => editors.filter((editor) => !isCustomEditor(editor)),
    [editors],
  );
  const availableEditors = useMemo(() => resolveAvailableEditors(editors), [editors]);
  const defaultOptions = useMemo<ComboboxOption[]>(
    () => buildDefaultEditorOptions(availableEditors, state.defaultEditorId, t),
    [availableEditors, state.defaultEditorId, t],
  );

  useSyncEditors(editors, setEditors);

  return (
    <SettingsPageTemplate
      title={t("settings:editors")}
      description={t("settings:configureTheIncludedCodeEditorAnd")}
      // Explicit, because the template otherwise derives the save-contributor id
      // from `title` — which is now translated, and an identity must not be.
      saveId="settings-page:editors"
      isDirty={isDirty}
      saveStatus={saveDefaultRequest.status}
      saveRevision={saveRevision}
      canSave={!hasInvalidConfig}
      invalidReason={
        hasInvalidConfig ? t("settings:fixInvalidLspServerConfigurationBefore") : undefined
      }
      onSave={() => saveDefaultRequest.run()}
    >
      <div className="space-y-6">
        <div className="space-y-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {t("settings:fileEditor")}
          </div>
          <LspLanguageCards
            lspAutoStartLanguages={state.lspAutoStartLanguages}
            lspAutoInstallLanguages={state.lspAutoInstallLanguages}
            baselineLspAutoStart={state.baselineLspAutoStart}
            baselineLspAutoInstall={state.baselineLspAutoInstall}
            toggleAutoStart={toggleAutoStart}
            toggleAutoInstall={toggleAutoInstall}
          />
          <LspServerConfigSection
            lspConfigStrings={state.lspConfigStrings}
            baselineLspConfigStrings={state.baselineLspConfigStrings}
            lspConfigErrors={state.lspConfigErrors}
            expandedConfigLang={state.expandedConfigLang}
            setExpandedConfigLang={state.setExpandedConfigLang}
            updateLspConfigString={updateLspConfigString}
          />
        </div>
        <Separator />
        <EditorsSection
          defaultOptions={defaultOptions}
          defaultEditorId={state.defaultEditorId}
          baselineDefaultId={state.baselineDefaultId}
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
