"use client";

import { useEffect, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import {
  type UtilityAgent,
  createUtilityAgent,
  updateUtilityAgent,
  getTemplateVariables,
} from "@/lib/api/domains/utility-api";
import { useAppStore } from "@/components/state-provider";
import { SettingsPromptEditor } from "./settings-prompt-editor";
import { UtilityAgentProfilePicker } from "./utility-agent-profile-picker";
import type { ScriptPlaceholder } from "./profile-edit/script-editor-completions";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agent: UtilityAgent | null;
  onSuccess: () => void;
};
type FormState = { name: string; description: string; prompt: string; profile_id: string };
const PROMPT_VARIABLE_SIGIL = "{{";
const defaultFormState: FormState = { name: "", description: "", prompt: "", profile_id: "" };

function toScriptPlaceholders(
  variables: { name: string; description: string; example: string }[],
): ScriptPlaceholder[] {
  return variables.map((variable) => ({
    key: variable.name,
    description: variable.description,
    example: variable.example,
    executor_types: [],
  }));
}

// eslint-disable-next-line max-lines-per-function
export function UtilityAgentDialog({ open, onOpenChange, agent, onSuccess }: Props) {
  const { t } = useTranslation();
  const profiles = useAppStore((state) => state.agentProfiles.items);
  const [form, setForm] = useState<FormState>(defaultFormState);
  const [saving, setSaving] = useState(false);
  const [placeholders, setPlaceholders] = useState<ScriptPlaceholder[]>([]);
  const isEdit = Boolean(agent);
  useEffect(() => {
    getTemplateVariables()
      .then(({ variables }) => setPlaceholders(toScriptPlaceholders(variables)))
      .catch(() => setPlaceholders([]));
  }, [open]);
  useEffect(() => {
    setForm(
      agent
        ? {
            name: agent.name,
            description: agent.description,
            prompt: agent.prompt,
            profile_id: agent.agent_profile_id || "",
          }
        : defaultFormState,
    );
  }, [agent, open]);
  const eligible = profiles.filter(
    (profile) => profile.enabled !== false && !profile.cli_passthrough && !profile.workspace_id,
  );
  let submitLabel = t("settings:utilityAgentDialogCreate");
  if (saving) submitLabel = t("settings:saving");
  else if (isEdit) submitLabel = t("settings:utilityAgentDialogSave");
  const submit = async () => {
    if (!form.name.trim() || !form.prompt.trim() || !form.profile_id) return;
    setSaving(true);
    try {
      const data = {
        name: form.name,
        description: form.description,
        prompt: form.prompt,
        agent_profile_id: form.profile_id,
        profile_binding_state: "explicit",
      };
      if (agent) await updateUtilityAgent(agent.id, data);
      else await createUtilityAgent(data);
      onSuccess();
    } catch (error) {
      console.error("Failed to save utility agent", error);
    } finally {
      setSaving(false);
    }
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? t("settings:utilityAgentDialogEditTitle")
              : t("settings:utilityAgentDialogCreateTitle")}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="name">{t("settings:name")}</Label>
            <Input
              id="name"
              value={form.name}
              onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
              placeholder={t("settings:utilityAgentNamePlaceholder")}
              disabled={agent?.builtin ?? false}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="description">{t("settings:utilityAgentDescriptionLabel")}</Label>
            <Input
              id="description"
              value={form.description}
              onChange={(event) =>
                setForm((current) => ({ ...current, description: event.target.value }))
              }
              placeholder={t("settings:utilityAgentDescriptionPlaceholder")}
            />
          </div>
          <div className="space-y-2">
            <Label>{t("settings:utilityAgentProfile")}</Label>
            <UtilityAgentProfilePicker
              profiles={profiles}
              value={form.profile_id}
              onValueChange={(value) => setForm((current) => ({ ...current, profile_id: value }))}
              unavailableValue={
                form.profile_id && !eligible.some((profile) => profile.id === form.profile_id)
                  ? form.profile_id
                  : undefined
              }
              testId="utility-profile-picker-custom"
              triggerClassName="w-full"
            />
            {form.profile_id && !eligible.some((profile) => profile.id === form.profile_id) && (
              <p className="text-xs text-destructive">{t("settings:utilityProfileNeedsRepair")}</p>
            )}
          </div>
          <div className="space-y-2">
            <Label>{t("settings:utilityAgentPromptTemplate")}</Label>
            <SettingsPromptEditor
              value={form.prompt}
              onChange={(value) => setForm((current) => ({ ...current, prompt: value }))}
              language="plaintext"
              height="200px"
              placeholders={placeholders}
              testId="utility-agent-prompt-editor"
              help={
                <p className="text-xs text-muted-foreground">
                  <Trans
                    i18nKey="settings:utilityAgentPromptHint"
                    values={{ sigil: PROMPT_VARIABLE_SIGIL }}
                  >
                    Type <code>{PROMPT_VARIABLE_SIGIL}</code> to see available variables with
                    autocomplete
                  </Trans>
                </p>
              }
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            {t("settings:cancel")}
          </Button>
          <Button
            onClick={submit}
            disabled={saving || !form.name.trim() || !form.prompt.trim() || !form.profile_id}
            className="cursor-pointer"
          >
            {submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
