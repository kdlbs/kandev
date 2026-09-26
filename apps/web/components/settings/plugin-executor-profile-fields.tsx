"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type {
  ExecutorProfileField,
  ExecutorProfileFieldErrors,
  ExecutorProfileValues,
} from "@/lib/plugins/executor-profile-schema";
import type { ExecutorProvider } from "@/lib/types/http";
import { localizedExecutorProviderMessage } from "@/lib/executor-provider-display";

// i18n-exempt: private Select sentinel, never rendered to the user.
const NOT_SET = "__kandev_executor_profile_not_set__";

type Translate = (key: string, options?: Record<string, unknown>) => string;

function fieldErrorMessage(
  field: ExecutorProfileField,
  error: ExecutorProfileFieldErrors[string] | undefined,
  t: Translate,
) {
  if (!error) return undefined;
  const key = {
    required: "executors:providerRequiredField",
    type: "executors:providerFieldInvalid",
    enum: "executors:providerFieldOption",
    minimum: "executors:providerFieldRangeMin",
    maximum: "executors:providerFieldRangeMax",
  }[error.code];
  return t(key, { field: field.label, value: error.value });
}

function describedById(id: string, message?: string, description?: string): string | undefined {
  if (message) return `${id}-error`;
  return description ? `${id}-description` : undefined;
}

function fieldInputType(field: ExecutorProfileField): string {
  if (field.secret) return "password";
  return field.type === "string" ? "text" : "number";
}

function FieldSecretStatus({
  field,
  label,
  configured,
  cleared,
  disabled,
  onRestore,
}: {
  field: ExecutorProfileField;
  label: string;
  configured: boolean;
  cleared: boolean;
  disabled: boolean;
  onRestore?: (name: string) => void;
}) {
  const { t } = useTranslation();
  if (!field.secret || !configured) return null;
  if (cleared) {
    return (
      <div className="flex flex-wrap items-center gap-2">
        <p className="text-xs text-muted-foreground">
          {t("executors:providerFieldClearPending", { field: label })}
        </p>
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          className="min-h-11 cursor-pointer"
          onClick={() => onRestore?.(field.name)}
        >
          {t("executors:providerFieldKeep")}
        </Button>
      </div>
    );
  }
  return (
    <p
      className="text-xs text-muted-foreground"
      data-testid={`executor-profile-secret-configured-${field.name}`}
    >
      {t("executors:providerFieldConfigured", { field: label })}
    </p>
  );
}

function fieldStep(field: ExecutorProfileField): string | undefined {
  if (field.type === "integer") return "1";
  if (field.type === "number") return "any";
  return undefined;
}

function choiceLabel(field: ExecutorProfileField, option: string, t: Translate): string {
  if (field.type !== "boolean") return option;
  return t(option === "true" ? "common:enabled" : "common:disabled");
}

function FieldChoiceControl({
  field,
  id,
  value,
  disabled,
  error,
  onValueChange,
}: {
  field: ExecutorProfileField;
  id: string;
  value: string;
  disabled: boolean;
  error?: string;
  onValueChange: (name: string, value: string) => void;
}) {
  const { t } = useTranslation();
  const options = field.enumValues.length > 0 ? field.enumValues : ["true", "false"];
  return (
    <Select
      value={value || NOT_SET}
      disabled={disabled}
      onValueChange={(next) => onValueChange(field.name, next === NOT_SET ? "" : next)}
    >
      <SelectTrigger
        id={id}
        className="min-h-11 w-full max-w-md cursor-pointer"
        aria-invalid={Boolean(error)}
      >
        <SelectValue placeholder={t("executors:providerFieldSelectPlaceholder")} />
      </SelectTrigger>
      <SelectContent>
        {!field.required && (
          <SelectItem value={NOT_SET}>{t("executors:providerFieldNotSet")}</SelectItem>
        )}
        {options.map((option) => (
          <SelectItem key={option} value={option}>
            {choiceLabel(field, option, t)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function FieldInputControl({
  field,
  id,
  value,
  configured,
  cleared,
  disabled,
  error,
  describedBy,
  onValueChange,
  onClearSecret,
}: {
  field: ExecutorProfileField;
  id: string;
  value: string;
  configured: boolean;
  cleared: boolean;
  disabled: boolean;
  error?: string;
  describedBy?: string;
  onValueChange: (name: string, value: string) => void;
  onClearSecret?: (name: string) => void;
}) {
  const { t } = useTranslation();
  const placeholder =
    field.secret && configured ? t("executors:providerSecretReplacePlaceholder") : undefined;
  return (
    <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center">
      <Input
        id={id}
        type={fieldInputType(field)}
        inputMode={field.type === "number" || field.type === "integer" ? "decimal" : undefined}
        step={fieldStep(field)}
        min={field.minimum}
        max={field.maximum}
        value={value}
        disabled={disabled || cleared}
        autoComplete={field.secret ? "new-password" : undefined}
        placeholder={placeholder}
        aria-invalid={Boolean(error)}
        aria-describedby={describedBy}
        className="min-h-11 w-full max-w-md"
        onChange={(event) => onValueChange(field.name, event.target.value)}
      />
      {field.secret && configured && !cleared && (
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          className="min-h-11 cursor-pointer"
          data-testid={`executor-profile-secret-clear-${field.name}`}
          onClick={() => onClearSecret?.(field.name)}
        >
          {t("executors:providerFieldClear")}
        </Button>
      )}
    </div>
  );
}

function FieldControl(props: {
  field: ExecutorProfileField;
  id: string;
  value: string;
  configured: boolean;
  cleared: boolean;
  disabled: boolean;
  error?: string;
  describedBy?: string;
  onValueChange: (name: string, value: string) => void;
  onClearSecret?: (name: string) => void;
}) {
  if (props.field.type === "boolean" || props.field.enumValues.length > 0) {
    return <FieldChoiceControl {...props} />;
  }
  return <FieldInputControl {...props} />;
}

function PluginExecutorProfileField({
  field,
  value,
  provider,
  configuredSecrets,
  clearedSecrets,
  error,
  disabled,
  onValueChange,
  onClearSecret,
  onRestoreSecret,
}: {
  field: ExecutorProfileField;
  value: string;
  provider?: ExecutorProvider;
  configuredSecrets: Record<string, boolean>;
  clearedSecrets: Record<string, boolean>;
  error?: ExecutorProfileFieldErrors[string];
  disabled: boolean;
  onValueChange: (name: string, value: string) => void;
  onClearSecret?: (name: string) => void;
  onRestoreSecret?: (name: string) => void;
}) {
  const { t } = useTranslation();
  const label = localizedExecutorProviderMessage(provider, field.name, field.label, t);
  const description = field.description
    ? localizedExecutorProviderMessage(provider, `${field.name}_description`, field.description, t)
    : undefined;
  const message = fieldErrorMessage({ ...field, label }, error, t);
  const id = `executor-profile-${field.name}`;
  const describedBy = describedById(id, message, description);
  return (
    <div className="min-w-0 space-y-1.5" data-testid={`executor-profile-field-${field.name}`}>
      <Label htmlFor={id}>
        {label}
        {field.required && <span className="text-destructive"> *</span>}
      </Label>
      <FieldSecretStatus
        field={field}
        label={label}
        configured={configuredSecrets[field.name] === true}
        cleared={clearedSecrets[field.name] === true}
        disabled={disabled}
        onRestore={onRestoreSecret}
      />
      <FieldControl
        field={field}
        id={id}
        value={value}
        configured={configuredSecrets[field.name] === true}
        cleared={clearedSecrets[field.name] === true}
        disabled={disabled}
        error={message}
        describedBy={describedBy}
        onValueChange={onValueChange}
        onClearSecret={onClearSecret}
      />
      {description && (
        <p id={`${id}-description`} className="text-xs text-muted-foreground">
          {description}
        </p>
      )}
      {message && (
        <p id={`${id}-error`} role="alert" className="text-sm text-destructive">
          {message}
        </p>
      )}
    </div>
  );
}

export function PluginExecutorProfileFields({
  fields,
  values,
  configuredSecrets = {},
  clearedSecrets = {},
  errors = {},
  disabled = false,
  unavailableReason,
  provider,
  onValueChange,
  onClearSecret,
  onRestoreSecret,
}: {
  fields: ExecutorProfileField[];
  values: ExecutorProfileValues;
  configuredSecrets?: Record<string, boolean>;
  clearedSecrets?: Record<string, boolean>;
  errors?: ExecutorProfileFieldErrors;
  disabled?: boolean;
  unavailableReason?: string;
  provider?: ExecutorProvider;
  onValueChange: (name: string, value: string) => void;
  onClearSecret?: (name: string) => void;
  onRestoreSecret?: (name: string) => void;
}) {
  return (
    <div className="space-y-5">
      {unavailableReason && (
        <p
          role="status"
          className="rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-sm"
        >
          {unavailableReason}
        </p>
      )}
      {fields.map((field) => (
        <PluginExecutorProfileField
          key={field.name}
          field={field}
          value={values[field.name] ?? ""}
          provider={provider}
          configuredSecrets={configuredSecrets}
          clearedSecrets={clearedSecrets}
          error={errors[field.name]}
          disabled={disabled}
          onValueChange={onValueChange}
          onClearSecret={onClearSecret}
          onRestoreSecret={onRestoreSecret}
        />
      ))}
    </div>
  );
}
