"use client";

import { IconPencil, IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Separator } from "@kandev/ui/separator";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { ModelCombobox } from "@/components/settings/model-combobox";
import type { UtilityAgent, InferenceAgent } from "@/lib/api/domains/utility-api";

const USE_DEFAULT = "__USE_DEFAULT__";

export type ModelOption = { value: string; label: string; agentName: string; modelName: string };

// Default model selector section
type DefaultModelSectionProps = {
  inferenceAgents: InferenceAgent[];
  defaultAgentId: string;
  defaultModel: string;
  onDefaultChange: (agentId: string, model: string) => void;
};

export function DefaultModelSection({
  inferenceAgents,
  defaultAgentId,
  defaultModel,
  onDefaultChange,
}: DefaultModelSectionProps) {
  const selectedAgent = inferenceAgents.find((a) => a.id === defaultAgentId);
  const modelOptions = selectedAgent?.models ?? [];
  const currentModelId = modelOptions.find((m) => m.is_default)?.id;

  return (
    <div className="space-y-3">
      <div>
        <h3 className="text-base font-medium">Default utility agent model</h3>
        <p className="text-sm text-muted-foreground">
          Select the default model used by all built-in utility actions.
        </p>
      </div>
      <div className="flex gap-2">
        <div className="w-[180px]">
          <Label className="text-xs text-muted-foreground mb-1 block">Agent</Label>
          <Select value={defaultAgentId} onValueChange={(v) => onDefaultChange(v, "")}>
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
        <div className="w-[220px]">
          <Label className="text-xs text-muted-foreground mb-1 block">Model</Label>
          <ModelCombobox
            value={defaultModel}
            onChange={(v) => onDefaultChange(defaultAgentId, v)}
            models={modelOptions}
            currentModelId={currentModelId}
            placeholder="Select model..."
            disabled={!defaultAgentId}
          />
        </div>
      </div>
    </div>
  );
}

// Builtin action row
type BuiltinActionRowProps = {
  agent: UtilityAgent;
  allModels: ModelOption[];
  defaultLabel: string;
  onModelChange: (agent: UtilityAgent, value: string) => void;
  onEdit: (agent: UtilityAgent) => void;
};

export function BuiltinActionRow({
  agent,
  allModels,
  defaultLabel,
  onModelChange,
  onEdit,
}: BuiltinActionRowProps) {
  const currentValue =
    agent.agent_id && agent.model ? `${agent.agent_id}|${agent.model}` : USE_DEFAULT;

  return (
    <div className="flex items-center gap-4 py-2 px-2 rounded hover:bg-muted/50 group">
      <div className="w-[420px] shrink-0 cursor-pointer" onClick={() => onEdit(agent)}>
        <div className="text-sm font-medium">{agent.name}</div>
        <p className="text-xs text-muted-foreground truncate">{agent.description}</p>
      </div>
      <Select value={currentValue} onValueChange={(v) => onModelChange(agent, v)}>
        <SelectTrigger className="w-[240px] cursor-pointer">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={USE_DEFAULT}>{defaultLabel}</SelectItem>
          {allModels.map((m) => (
            <SelectItem key={m.value} value={m.value}>
              {m.agentName} / {m.modelName}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// Custom agent row
type CustomAgentRowProps = {
  agent: UtilityAgent;
  onEdit: (agent: UtilityAgent) => void;
  onDelete: (agent: UtilityAgent) => void;
};

export function CustomAgentRow({ agent, onEdit, onDelete }: CustomAgentRowProps) {
  return (
    <div className="flex items-center justify-between py-3 px-3 rounded hover:bg-muted/50 group">
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium">{agent.name}</div>
        <p className="text-xs text-muted-foreground truncate">{agent.description}</p>
      </div>
      <div className="flex items-center gap-2">
        <span className="text-xs text-muted-foreground">{agent.model || "Not configured"}</span>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onEdit(agent)}
          className="h-7 w-7 p-0 opacity-0 group-hover:opacity-100 cursor-pointer"
        >
          <IconPencil className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onDelete(agent)}
          className="h-7 w-7 p-0 opacity-0 group-hover:opacity-100 text-destructive cursor-pointer"
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </div>
    </div>
  );
}

// Per-action overrides section
type PerActionOverridesSectionProps = {
  builtins: UtilityAgent[];
  allModels: ModelOption[];
  defaultModel: string;
  onModelChange: (agent: UtilityAgent, value: string) => void;
  onEdit: (agent: UtilityAgent) => void;
};

export function PerActionOverridesSection({
  builtins,
  allModels,
  defaultModel,
  onModelChange,
  onEdit,
}: PerActionOverridesSectionProps) {
  if (builtins.length === 0) return null;

  const defaultLabel = defaultModel ? `Default (${defaultModel})` : "Default";

  return (
    <div className="space-y-2">
      <Separator />
      <h3 className="text-base font-medium pt-2">Actions</h3>
      <div className="space-y-0">
        {builtins.map((agent) => (
          <BuiltinActionRow
            key={agent.id}
            agent={agent}
            allModels={allModels}
            defaultLabel={defaultLabel}
            onModelChange={onModelChange}
            onEdit={onEdit}
          />
        ))}
      </div>
    </div>
  );
}

// Custom agents section
type CustomAgentsSectionProps = {
  agents: UtilityAgent[];
  onAdd: () => void;
  onEdit: (agent: UtilityAgent) => void;
  onDelete: (agent: UtilityAgent) => void;
};

export function CustomAgentsSection({ agents, onAdd, onEdit, onDelete }: CustomAgentsSectionProps) {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-base font-medium">Custom utility agents</h3>
          <p className="text-sm text-muted-foreground">
            Create your own utility agents with custom prompts.
          </p>
        </div>
        <Button onClick={onAdd} size="sm" className="cursor-pointer">
          <IconPlus className="h-4 w-4 mr-1" />
          Add
        </Button>
      </div>
      {agents.length === 0 && (
        <p className="text-sm text-muted-foreground py-4">No custom utility agents.</p>
      )}
      {agents.length > 0 && (
        <div className="space-y-2">
          {agents.map((agent) => (
            <CustomAgentRow key={agent.id} agent={agent} onEdit={onEdit} onDelete={onDelete} />
          ))}
        </div>
      )}
    </div>
  );
}

// Export the USE_DEFAULT constant
export { USE_DEFAULT };
