"use client";

import { IconPencil, IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
import {
  configOptionToModelOptions,
  isModelConfigOption,
  ModelConfigSelector,
  type SelectConfigOption,
  usableConfigOptions,
} from "@/components/model-config-selector";
import { InferenceAgentStatusNote } from "@/components/settings/inference-agent-status";
import type { UtilityAgent, InferenceAgent } from "@/lib/api/domains/utility-api";

const USE_DEFAULT = "__USE_DEFAULT__";

export type ModelOption = { value: string; label: string; agentName: string; modelName: string };

type ModelGroup = { agentName: string; models: ModelOption[] };

function groupModelsByAgent(models: ModelOption[]): ModelGroup[] {
  const map = new Map<string, ModelOption[]>();
  for (const m of models) {
    const list = map.get(m.agentName);
    if (list) list.push(m);
    else map.set(m.agentName, [m]);
  }
  return Array.from(map, ([agentName, items]) => ({ agentName, models: items }));
}

function utilityConfigOptions(agent: InferenceAgent | undefined): SelectConfigOption[] {
  return usableConfigOptions(
    agent?.config_options?.map((option) => ({
      type: option.type,
      id: option.id,
      name: option.name,
      currentValue: option.current_value,
      category: option.category,
      options: option.options,
    })),
  );
}

// Default model selector section
type DefaultModelSectionProps = {
  inferenceAgents: InferenceAgent[];
  defaultAgentId: string;
  defaultModel: string;
  onDefaultChange: (agentId: string, model: string) => void;
  onRefreshAgent: (agentId: string) => Promise<unknown> | void;
};

export function DefaultModelSection({
  inferenceAgents,
  defaultAgentId,
  defaultModel,
  onDefaultChange,
  onRefreshAgent,
}: DefaultModelSectionProps) {
  const selectedAgent = inferenceAgents.find((a) => a.id === defaultAgentId);
  const modelOptions = selectedAgent?.models ?? [];
  const currentModelId = modelOptions.find((m) => m.is_default)?.id;
  const configOptions = utilityConfigOptions(selectedAgent);
  const modelConfig = configOptions.find(isModelConfigOption);
  const selectorModels = modelConfig
    ? configOptionToModelOptions(modelConfig)
    : modelOptions.map((model) => ({
        id: model.id,
        name: model.name,
        description: model.description || (model.id !== model.name ? model.id : undefined),
        usageMultiplier:
          typeof model.meta?.copilotUsage === "string" ? model.meta.copilotUsage : undefined,
      }));
  const selectedModel = defaultModel || modelConfig?.currentValue || currentModelId || null;
  const selectedConfigOptions = configOptions.map((option) => ({
    ...option,
    currentValue:
      isModelConfigOption(option) && selectedModel ? selectedModel : option.currentValue,
  }));

  return (
    <Card data-testid="utility-default-model-card">
      <CardHeader>
        <CardTitle className="text-base">
          <h3>Default utility agent model</h3>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-sm text-muted-foreground">
          Select the default model used by all built-in utility actions.
        </p>
        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="w-full sm:w-[180px]">
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
          <div className="w-full sm:w-[280px]">
            <Label className="text-xs text-muted-foreground mb-1 block">Model</Label>
            <ModelConfigSelector
              modelOptions={selectorModels}
              currentModel={selectedModel}
              configOptions={selectedConfigOptions}
              onModelChange={(v) => onDefaultChange(defaultAgentId, v)}
              disabled={!defaultAgentId}
              placeholder="Select model..."
              ariaLabel="Default utility model settings"
            />
          </div>
        </div>
        {defaultAgentId && (
          <InferenceAgentStatusNote
            agent={selectedAgent}
            fallbackName={defaultAgentId}
            onRefresh={() => onRefreshAgent(defaultAgentId)}
          />
        )}
      </CardContent>
    </Card>
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
    <div
      className="flex items-center gap-4 py-2 px-2 rounded hover:bg-muted/50"
      data-testid={`utility-action-row-${agent.id}`}
    >
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium truncate">{agent.name}</div>
        <p className="text-xs text-muted-foreground truncate">{agent.description}</p>
      </div>
      <Select value={currentValue} onValueChange={(v) => onModelChange(agent, v)}>
        <SelectTrigger className="w-[240px] shrink-0 cursor-pointer">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value={USE_DEFAULT}>{defaultLabel}</SelectItem>
          </SelectGroup>
          {groupModelsByAgent(allModels).map((group) => (
            <SelectGroup key={group.agentName}>
              <SelectSeparator />
              <SelectLabel>{group.agentName}</SelectLabel>
              {group.models.map((m) => (
                <SelectItem key={m.value} value={m.value}>
                  {m.modelName}
                </SelectItem>
              ))}
            </SelectGroup>
          ))}
        </SelectContent>
      </Select>
      <Button
        variant="ghost"
        size="sm"
        onClick={() => onEdit(agent)}
        className="h-7 w-7 p-0 shrink-0 cursor-pointer text-muted-foreground hover:text-foreground"
      >
        <IconPencil className="h-3.5 w-3.5" />
      </Button>
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
    <div className="flex items-center justify-between py-3 px-3 rounded hover:bg-muted/50">
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
          className="h-7 w-7 p-0 cursor-pointer text-muted-foreground hover:text-foreground"
        >
          <IconPencil className="h-3.5 w-3.5" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onDelete(agent)}
          className="h-7 w-7 p-0 cursor-pointer text-muted-foreground hover:text-destructive"
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
    <Card data-testid="utility-actions-card">
      <CardHeader>
        <CardTitle className="text-base">
          <h3>Actions</h3>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-0">
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
      </CardContent>
    </Card>
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
    <Card data-testid="utility-custom-agents-card">
      <CardHeader>
        <CardTitle className="text-base">
          <h3>Custom utility agents</h3>
        </CardTitle>
        <CardAction>
          <Button onClick={onAdd} size="sm" className="cursor-pointer">
            <IconPlus className="h-4 w-4 mr-1" />
            Add
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Create your own utility agents with custom prompts.
        </p>
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
      </CardContent>
    </Card>
  );
}

// Export the USE_DEFAULT constant
export { USE_DEFAULT };
