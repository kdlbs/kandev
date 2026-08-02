import type { AgentProfileOption } from "@/lib/state/slices";
import type { AvailableAgent, WorkflowStep } from "@/lib/types/http";
import type {
  ConfigureSessionOperation,
  ConfigureSessionRule,
  OnEnterAction,
} from "@/lib/types/workflow-actions";
import {
  isModelConfigOption,
  type DynamicConfigOption,
  type SelectConfigOption,
} from "@/components/model-config-selector";

export type AgentChoice = { name: string; label: string };

export type ConfigureSessionAction = Extract<OnEnterAction, { type: "configure_session" }>;

export function configureSessionAction(step: WorkflowStep): ConfigureSessionAction | undefined {
  return step.events?.on_enter?.find(
    (candidate): candidate is ConfigureSessionAction => candidate.type === "configure_session",
  );
}

export function buildAgentChoices(profiles: AgentProfileOption[]): AgentChoice[] {
  const choices = new Map<string, string>();
  for (const profile of profiles) {
    choices.set(profile.agent_name, profile.label.split(" • ")[0] || profile.agent_name);
  }
  return [...choices.entries()].map(([name, label]) => ({ name, label }));
}

export function withConfigureSessionRules(
  step: WorkflowStep,
  rules: ConfigureSessionRule[],
): Partial<WorkflowStep> {
  const events = step.events ?? {};
  const onEnter = events.on_enter ?? [];
  const nextAction: OnEnterAction = {
    type: "configure_session",
    config: { rules },
  };
  const nextOnEnter = onEnter.some((candidate) => candidate.type === "configure_session")
    ? onEnter.map((candidate) => (candidate.type === "configure_session" ? nextAction : candidate))
    : [...onEnter, nextAction];
  return { events: { ...events, on_enter: nextOnEnter } };
}

export function withoutConfigureSession(step: WorkflowStep): Partial<WorkflowStep> {
  const events = step.events ?? {};
  return {
    events: {
      ...events,
      on_enter: (events.on_enter ?? []).filter(
        (candidate) => candidate.type !== "configure_session",
      ),
    },
  };
}

export function defaultConfigureSessionRule(
  profiles: AgentProfileOption[],
  availableAgents: AvailableAgent[],
): ConfigureSessionRule | undefined {
  const selectedAgent = buildAgentChoices(profiles)[0]?.name;
  if (!selectedAgent) return undefined;
  const model = defaultModelForAgent(selectedAgent, availableAgents);
  return {
    agent_name: selectedAgent,
    operation: "set",
    ...(model ? { model } : {}),
  };
}

export function defaultModelForAgent(
  agentName: string,
  availableAgents: AvailableAgent[],
): string | undefined {
  const config = availableAgents.find((agent) => agent.name === agentName)?.model_config;
  return config?.current_model_id || config?.default_model || config?.available_models?.[0]?.id;
}

export function modelConfigOptions(
  modelConfig: AvailableAgent["model_config"] | undefined,
  rule: ConfigureSessionRule,
): SelectConfigOption[] {
  return (modelConfig?.config_options ?? []).map((option) => {
    const dynamicOption: DynamicConfigOption = {
      type: option.type,
      id: option.id,
      name: option.name,
      description: option.description,
      currentValue: isModelConfigOption(option)
        ? (rule.operation === "set" ? rule.model : undefined) || option.current_value
        : (rule.operation === "set" ? rule.config_options?.[option.id] : undefined) ||
          option.current_value,
      category: option.category,
      options: option.options,
    };
    return { ...dynamicOption, options: option.options ?? [] };
  });
}

export function operationRule(
  rule: ConfigureSessionRule,
  operation: ConfigureSessionOperation,
  availableAgents: AvailableAgent[] = [],
): ConfigureSessionRule {
  if (operation === "set") {
    const model =
      (rule.operation === "set" ? rule.model : undefined) ||
      defaultModelForAgent(rule.agent_name, availableAgents);
    return {
      agent_name: rule.agent_name,
      operation,
      ...(model ? { model } : {}),
      ...(rule.operation === "set" && rule.config_options
        ? { config_options: rule.config_options }
        : {}),
    };
  }
  return { agent_name: rule.agent_name, operation };
}
