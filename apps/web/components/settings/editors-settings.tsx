"use client";

import { useEffect, useMemo } from "react";
import { IconEdit, IconTrash, IconChevronDown, IconExternalLink } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { Textarea } from "@kandev/ui/textarea";
import { SettingsPageTemplate } from "@/components/settings/settings-page-template";
import { SettingsTarget } from "@/components/settings/settings-target";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import { Combobox, type ComboboxOption } from "@/components/combobox";
import { EditableCard } from "@/components/settings/editable-card";
import {
  EditorForm,
  type EditorFormState,
  defaultFormState,
  formStateFromEditor,
  getCustomEditorSummary,
} from "@/components/settings/editor-form";
import { LSP_DEFAULT_CONFIGS } from "@/lib/lsp/lsp-client-config";
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
import { LspLanguageCards } from "./lsp-language-cards";
import { LSP_LANGUAGE_OPTIONS } from "./lsp-language-options";
import { Trans, useTranslation } from "react-i18next";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { SETTINGS_TYPOGRAPHY } from "@/components/settings/settings-typography";

/**
 * Code identifiers rendered inside `<Trans>` copy. They are passed as
 * interpolation VALUES rather than left in the tag body: text inside a `<n>` tag
 * is part of the message a translator edits, and translating a command name or
 * an LSP method breaks the thing it names.
 */
const LSP_CONFIG_METHOD = "workspace/configuration";

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
          className="rounded-lg border border-border/70 bg-background p-4 flex flex-col gap-3 cursor-pointer md:flex-row md:items-center md:justify-between"
          onClick={open}
        >
          <div className="min-w-0">
            <div className="break-words font-medium text-sm text-foreground">{editor.name}</div>
            <div className="text-xs text-muted-foreground truncate">
              {getCustomEditorSummary(t, editor)}
            </div>
          </div>
          <div className="flex w-full flex-col gap-2 md:w-auto md:flex-row">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className={settingsActionClassName("cursor-pointer")}
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
              className={settingsActionClassName("cursor-pointer")}
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
      <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
        <div className="text-sm font-medium text-foreground">{t("settings:customEditors")}</div>
        <Button
          type="button"
          variant="outline"
          onClick={() => setIsAdding(true)}
          className={settingsActionClassName()}
        >
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
  embedded: boolean;
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
  embedded,
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
      {embedded ? (
        <h3 className={SETTINGS_TYPOGRAPHY.sectionTitle}>{t("settings:editors")}</h3>
      ) : (
        <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          {t("settings:editors")}
        </div>
      )}
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
    lspStatusHiddenLanguages: state.lspStatusHiddenLanguages,
    lspStatusLocation: state.lspStatusLocation,
    lspConfigStrings: state.lspConfigStrings,
  });
}

function useSyncEditors(editors: EditorOption[], setEditors: (editors: EditorOption[]) => void) {
  useEffect(() => setEditors(editors), [editors, setEditors]);
}

export function EditorsSettings({ embedded = false }: { embedded?: boolean }) {
  const { t } = useTranslation();
  const state = useEditorsSettingsState();
  const { setLspConfigStrings, setLspConfigErrors, setEditors, editors } = state;
  const applyEditors = useApplyEditors(state);
  const saveDefaultRequest = useSaveRequest(state);
  const { createRequest, updateRequest, deleteRequest } = useEditorRequests(state, applyEditors);
  const { updateLspConfigString } = useLspConfigActions(setLspConfigStrings, setLspConfigErrors);
  const { toggleAutoStart, toggleAutoInstall, toggleStatusVisibility } =
    useLspLanguageToggles(state);
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
      showPageChrome={!embedded}
    >
      <div className="space-y-6">
        <SettingsTarget targetId={GENERAL_SETTINGS_TARGETS.fileEditor} className="space-y-4">
          <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {t("settings:fileEditor")}
          </div>
          <LspLanguageCards
            lspAutoStartLanguages={state.lspAutoStartLanguages}
            lspAutoInstallLanguages={state.lspAutoInstallLanguages}
            baselineLspAutoStart={state.baselineLspAutoStart}
            baselineLspAutoInstall={state.baselineLspAutoInstall}
            lspStatusHiddenLanguages={state.lspStatusHiddenLanguages}
            baselineLspStatusHiddenLanguages={state.baselineLspStatusHiddenLanguages}
            toggleAutoStart={toggleAutoStart}
            toggleAutoInstall={toggleAutoInstall}
            toggleStatusVisibility={toggleStatusVisibility}
          />
          <LspServerConfigSection
            lspConfigStrings={state.lspConfigStrings}
            baselineLspConfigStrings={state.baselineLspConfigStrings}
            lspConfigErrors={state.lspConfigErrors}
            expandedConfigLang={state.expandedConfigLang}
            setExpandedConfigLang={state.setExpandedConfigLang}
            updateLspConfigString={updateLspConfigString}
          />
        </SettingsTarget>
        <Separator />
        <EditorsSection
          embedded={embedded}
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
