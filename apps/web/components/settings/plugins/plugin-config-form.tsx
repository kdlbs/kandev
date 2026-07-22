"use client";

import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import { useUtilityAgents } from "@/hooks/domains/settings/use-utility-agents";
import type { PluginConfigField } from "@/lib/plugins/config-schema";

/** Sentinel for the "Not set" select item — an actual enum option can never
 * collide with it, and Radix rejects value="" items. */
const ENUM_UNSET_SENTINEL = "__kandev_enum_unset__";
const UTILITY_AGENT_UNSET_SENTINEL = "__kandev_utility_agent_unset__";

type PluginConfigFormProps = {
  fields: PluginConfigField[];
  values: Record<string, string | boolean>;
  initialValues: Record<string, string | boolean>;
  disabled: boolean;
  onChange: (name: string, value: string | boolean) => void;
};

/**
 * Schema-driven settings form: one control per config_schema property.
 * Secret fields render as password inputs pre-filled with the backend's
 * mask; editing replaces the value, leaving it untouched keeps the stored
 * secret. Purely controlled — load/save/dirty state lives in PluginDetail.
 */
export function PluginConfigForm({
  fields,
  values,
  initialValues,
  disabled,
  onChange,
}: PluginConfigFormProps) {
  return (
    <div className="space-y-5">
      {fields.map((field) => (
        <ConfigFieldRow
          key={field.name}
          field={field}
          value={values[field.name] ?? ""}
          isDirty={values[field.name] !== initialValues[field.name]}
          disabled={disabled}
          onChange={onChange}
        />
      ))}
    </div>
  );
}

type ConfigFieldRowProps = {
  field: PluginConfigField;
  value: string | boolean;
  isDirty: boolean;
  disabled: boolean;
  onChange: (name: string, value: string | boolean) => void;
};

function ConfigFieldRow({ field, value, isDirty, disabled, onChange }: ConfigFieldRowProps) {
  const inputId = `plugin-config-${field.name}`;
  return (
    <div className="space-y-1.5" data-testid={`plugin-config-field-${field.name}`}>
      <Label htmlFor={inputId} className="text-sm">
        {field.label}
        {field.required && <span className="text-destructive"> *</span>}
      </Label>
      <ConfigFieldControl
        field={field}
        inputId={inputId}
        value={value}
        isDirty={isDirty}
        disabled={disabled}
        onChange={onChange}
      />
      {field.description && <p className="text-xs text-muted-foreground">{field.description}</p>}
    </div>
  );
}

type ConfigFieldControlProps = ConfigFieldRowProps & { inputId: string };

function ConfigFieldControl({
  field,
  inputId,
  value,
  isDirty,
  disabled,
  onChange,
}: ConfigFieldControlProps) {
  if (field.type === "boolean") {
    return (
      <div>
        <Switch
          id={inputId}
          checked={value === true}
          disabled={disabled}
          data-settings-dirty={isDirty}
          onCheckedChange={(checked) => onChange(field.name, checked)}
        />
      </div>
    );
  }

  if (field.type === "enum") {
    // Radix Select forbids an item with value="", so an explicit "Not set"
    // sentinel lets optional enums be cleared back to unset (serialization
    // omits empty strings).
    return (
      <Select
        value={typeof value === "string" ? value : ""}
        disabled={disabled}
        onValueChange={(next) => onChange(field.name, next === ENUM_UNSET_SENTINEL ? "" : next)}
      >
        <SelectTrigger
          id={inputId}
          className="max-w-md cursor-pointer"
          data-settings-dirty={isDirty}
        >
          <SelectValue placeholder="Select..." />
        </SelectTrigger>
        <SelectContent>
          {!field.required && (
            <SelectItem
              value={ENUM_UNSET_SENTINEL}
              className="cursor-pointer text-muted-foreground"
            >
              Not set
            </SelectItem>
          )}
          {(field.enumValues ?? []).map((option) => (
            <SelectItem key={option} value={option} className="cursor-pointer">
              {option}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  if (field.type === "utility_agent") {
    return (
      <UtilityAgentSelect
        field={field}
        inputId={inputId}
        value={value}
        isDirty={isDirty}
        disabled={disabled}
        onChange={onChange}
      />
    );
  }

  return (
    <Input
      id={inputId}
      type={inputType(field)}
      // step="1" on integer fields nudges the browser to flag non-integral
      // input in-place; serializeConfigValues still rejects it as a backstop.
      step={!field.secret && field.type === "integer" ? "1" : undefined}
      value={typeof value === "string" ? value : ""}
      disabled={disabled}
      data-settings-dirty={isDirty}
      autoComplete={field.secret ? "off" : undefined}
      className="max-w-md"
      onChange={(event) => onChange(field.name, event.target.value)}
    />
  );
}

function UtilityAgentSelect({
  field,
  inputId,
  value,
  isDirty,
  disabled,
  onChange,
}: ConfigFieldControlProps) {
  const { agents, loading, error } = useUtilityAgents();
  const selectedID = typeof value === "string" ? value : "";
  const selectedAgent = agents.find((agent) => agent.id === selectedID);
  const selectedLabel = selectedAgent?.name ?? selectedUtilityAgentFallback(selectedID, loading);
  const placeholder = utilityAgentPlaceholder(loading, error);

  return (
    <Select
      value={selectedID}
      disabled={disabled || loading || error !== null}
      onValueChange={(next) =>
        onChange(field.name, next === UTILITY_AGENT_UNSET_SENTINEL ? "" : next)
      }
    >
      <SelectTrigger id={inputId} className="max-w-md cursor-pointer" data-settings-dirty={isDirty}>
        <SelectValue placeholder={placeholder}>{selectedLabel}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        {!field.required && (
          <SelectItem
            value={UTILITY_AGENT_UNSET_SENTINEL}
            className="cursor-pointer text-muted-foreground"
          >
            Not set
          </SelectItem>
        )}
        {agents.map((agent) => (
          <SelectItem
            key={agent.id}
            value={agent.id}
            className="cursor-pointer"
            disabled={!agent.enabled}
          >
            {agent.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function selectedUtilityAgentFallback(selectedID: string, loading: boolean): string | undefined {
  if (selectedID === "") return undefined;
  return loading ? "Loading selected utility agent..." : "Selected utility agent unavailable";
}

function utilityAgentPlaceholder(loading: boolean, error: Error | null): string {
  if (loading) return "Loading utility agents...";
  if (error) return "Utility agents unavailable";
  return "Select a utility agent...";
}

function inputType(field: PluginConfigField): string {
  if (field.secret) return "password";
  if (field.type === "number" || field.type === "integer") return "number";
  return "text";
}
