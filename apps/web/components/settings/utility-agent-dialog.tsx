"use client";

import { useState, useEffect, useMemo } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { ModelCombobox } from "@/components/settings/model-combobox";
import {
  type UtilityAgent,
  createUtilityAgent,
  updateUtilityAgent,
  getTemplateVariables,
  listInferenceAgents,
  type TemplateVariable,
  type InferenceAgent,
  type InferenceModel,
} from "@/lib/api/domains/utility-api";
import { ScriptEditor } from "./profile-edit/script-editor";
import type { ScriptPlaceholder } from "./profile-edit/script-editor-completions";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agent: UtilityAgent | null;
  onSuccess: () => void;
};

type FormState = {
  name: string;
  description: string;
  prompt: string;
  agent_id: string;
  model: string;
};

const defaultFormState: FormState = {
  name: "",
  description: "",
  prompt: "",
  agent_id: "claude-code",
  model: "",
};

function toScriptPlaceholders(variables: TemplateVariable[]): ScriptPlaceholder[] {
  return variables.map((v) => ({
    key: v.name,
    description: v.description,
    example: v.example,
    executor_types: [],
  }));
}

type AgentModelSelectProps = {
  agentId: string;
  model: string;
  inferenceAgents: InferenceAgent[];
  availableModels: InferenceModel[];
  onAgentChange: (agentId: string) => void;
  onModelChange: (model: string) => void;
};

function AgentModelSelect({
  agentId,
  model,
  inferenceAgents,
  availableModels,
  onAgentChange,
  onModelChange,
}: AgentModelSelectProps) {
  return (
    <div className="grid grid-cols-2 gap-4">
      <div className="space-y-2">
        <Label>Agent</Label>
        <Select value={agentId} onValueChange={onAgentChange}>
          <SelectTrigger className="cursor-pointer">
            <SelectValue placeholder="Select agent..." />
          </SelectTrigger>
          <SelectContent>
            {inferenceAgents.map((ia) => (
              <SelectItem key={ia.id} value={ia.id}>
                {ia.display_name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-2">
        <Label>Model</Label>
        <ModelCombobox
          value={model}
          onChange={onModelChange}
          models={availableModels}
          currentModelId={availableModels.find((m) => m.is_default)?.id}
          placeholder="Select model..."
          disabled={availableModels.length === 0}
        />
      </div>
    </div>
  );
}

type UtilityAgentFormProps = {
  form: FormState;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  isBuiltin: boolean;
  inferenceAgents: InferenceAgent[];
  availableModels: InferenceModel[];
  placeholders: ScriptPlaceholder[];
};

function UtilityAgentForm({
  form,
  setForm,
  isBuiltin,
  inferenceAgents,
  availableModels,
  placeholders,
}: UtilityAgentFormProps) {
  return (
    <div className="space-y-4 py-4">
      <div className="space-y-2">
        <Label htmlFor="name">Name</Label>
        <Input
          id="name"
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          placeholder="e.g., commit-message"
          disabled={isBuiltin}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="description">Description</Label>
        <Input
          id="description"
          value={form.description}
          onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
          placeholder="Brief description of what this agent does"
        />
      </div>
      <AgentModelSelect
        agentId={form.agent_id}
        model={form.model}
        inferenceAgents={inferenceAgents}
        availableModels={availableModels}
        onAgentChange={(v) => setForm((f) => ({ ...f, agent_id: v, model: "" }))}
        onModelChange={(v) => setForm((f) => ({ ...f, model: v }))}
      />
      <div className="space-y-2">
        <Label>Prompt Template</Label>
        <div className="border rounded-md overflow-hidden">
          <ScriptEditor
            value={form.prompt}
            onChange={(v) => setForm((f) => ({ ...f, prompt: v }))}
            language="plaintext"
            height="200px"
            placeholders={placeholders}
            lineNumbers="off"
          />
        </div>
        <p className="text-xs text-muted-foreground">
          Type {"{{"} to see available variables with autocomplete
        </p>
      </div>
    </div>
  );
}

export function UtilityAgentDialog({ open, onOpenChange, agent, onSuccess }: Props) {
  const [form, setForm] = useState<FormState>(defaultFormState);
  const [saving, setSaving] = useState(false);
  const [placeholders, setPlaceholders] = useState<ScriptPlaceholder[]>([]);
  const [inferenceAgents, setInferenceAgents] = useState<InferenceAgent[]>([]);
  const isEdit = Boolean(agent);

  // Fetch template variables and inference agents
  useEffect(() => {
    getTemplateVariables()
      .then(({ variables }) => setPlaceholders(toScriptPlaceholders(variables)))
      .catch(() => setPlaceholders([]));

    listInferenceAgents()
      .then(({ agents }) => setInferenceAgents(agents))
      .catch(() => setInferenceAgents([]));
  }, []);

  // Get models for the selected agent
  const selectedAgent = useMemo(
    () => inferenceAgents.find((a) => a.id === form.agent_id),
    [inferenceAgents, form.agent_id],
  );

  const availableModels = selectedAgent?.models ?? [];

  // Auto-select default model when agent changes
  useEffect(() => {
    if (selectedAgent && !form.model) {
      const defaultModel = (selectedAgent.models ?? []).find((m) => m.is_default);
      if (defaultModel) {
        setForm((f) => ({ ...f, model: defaultModel.id }));
      }
    }
  }, [selectedAgent, form.model]);

  useEffect(() => {
    if (agent) {
      setForm({
        name: agent.name,
        description: agent.description,
        prompt: agent.prompt,
        agent_id: agent.agent_id || "claude-code",
        model: agent.model || "",
      });
    } else {
      setForm(defaultFormState);
    }
  }, [agent, open]);

  const handleSubmit = async () => {
    setSaving(true);
    try {
      const data = {
        name: form.name,
        description: form.description,
        prompt: form.prompt,
        agent_id: form.agent_id,
        model: form.model,
      };

      if (isEdit && agent) {
        await updateUtilityAgent(agent.id, data);
      } else {
        await createUtilityAgent(data);
      }
      onSuccess();
    } catch (error) {
      console.error("Failed to save agent:", error);
    } finally {
      setSaving(false);
    }
  };

  const dialogTitle = isEdit ? "Edit Utility Agent" : "Create Utility Agent";
  const getSubmitLabel = () => {
    if (saving) return "Saving...";
    return isEdit ? "Save Changes" : "Create Agent";
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{dialogTitle}</DialogTitle>
        </DialogHeader>
        <UtilityAgentForm
          form={form}
          setForm={setForm}
          isBuiltin={agent?.builtin ?? false}
          inferenceAgents={inferenceAgents}
          availableModels={availableModels}
          placeholders={placeholders}
        />
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={saving || !form.name} className="cursor-pointer">
            {getSubmitLabel()}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
