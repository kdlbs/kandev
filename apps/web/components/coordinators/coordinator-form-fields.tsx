"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { AgentProfilePicker } from "@/components/settings/agent-profile-picker";
import {
  ExecutorProfileSelector,
  type ExecutorProfileSelectorProps,
} from "@/components/task-create-dialog-selectors";
import { useExecutorProfileOptions } from "@/components/task-create-dialog-options";
import { COORDINATOR_NAME_MAX_LENGTH } from "@/lib/coordinators/validate-form";
import {
  coordinatorProfileStatusMessageKey,
  type CoordinatorProfileField,
} from "@/lib/coordinators/profile-status-message";
import { withUnavailableExecutorOption } from "@/lib/coordinators/executor-option";
import { flattenExecutorProfiles } from "@/lib/coordinators/profile-lookup";
import type { CoordinatorFormState } from "@/lib/coordinators/coordinator-form";
import type { CoordinatorErrorField, CoordinatorFieldError } from "@/lib/coordinators/field-error";
import type { ProfileStatus } from "@/lib/api/domains/coordinator-api";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor } from "@/lib/types/http";
import { LONG_TEXT_FIELD_CLASS } from "./long-text-field";

export type CoordinatorFormFieldsProps = {
  form: CoordinatorFormState;
  onChange: <K extends keyof CoordinatorFormState>(key: K, value: CoordinatorFormState[K]) => void;
  disabled: boolean;
  agentProfiles: readonly AgentProfileOption[];
  executors: readonly Executor[];
  agentProfileStatus?: ProfileStatus;
  executorProfileStatus?: ProfileStatus;
  taskAgentProfileStatus?: ProfileStatus;
  taskExecutorProfileStatus?: ProfileStatus;
  fieldError: CoordinatorFieldError | null;
  /** Setup mode: show only these fields, in the phase-1 order. */
  only?: readonly CoordinatorFieldKey[];
  /** Setup mode: one message per field, replacing `fieldError`. */
  messages?: Partial<Record<CoordinatorErrorField, string>>;
  onLeave?: (field: CoordinatorErrorField) => void;
};

export type CoordinatorFieldKey = "name" | "agent" | "executor" | "taskPair" | "context";

function FieldError({ testId, message }: { testId: string; message: string | undefined }) {
  if (!message) return null;
  return (
    <p data-testid={testId} className="text-sm text-destructive">
      {message}
    </p>
  );
}

function ProfileStatusMessage({
  testId,
  field,
  status,
}: {
  testId: string;
  field: CoordinatorProfileField;
  status: ProfileStatus | undefined;
}) {
  const { t } = useTranslation();
  const key = coordinatorProfileStatusMessageKey(field, status);
  if (!key) return null;
  return (
    <p data-testid={testId} className="text-sm text-amber-600 dark:text-amber-500">
      {t(key)}
    </p>
  );
}

function useCoordinatorExecutorOptions(
  executors: readonly Executor[],
  executorProfileId: string,
): ExecutorProfileSelectorProps["options"] {
  const { t } = useTranslation();
  const flatProfiles = useMemo(() => flattenExecutorProfiles(executors), [executors]);
  const baseOptions = useExecutorProfileOptions(flatProfiles);
  return useMemo(
    () => [
      ...withUnavailableExecutorOption(
        baseOptions,
        executorProfileId,
        t("coordinator:executorProfileMissingWarning"),
      ),
    ],
    [baseOptions, executorProfileId, t],
  );
}

type AgentFieldProps = {
  form: CoordinatorFormState;
  onChange: CoordinatorFormFieldsProps["onChange"];
  disabled: boolean;
  agentProfiles: CoordinatorFormFieldsProps["agentProfiles"];
  status: ProfileStatus | undefined;
  message: string | undefined;
};

function AgentField({ form, onChange, disabled, agentProfiles, status, message }: AgentFieldProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label>{t("coordinator:agentProfileLabel")}</Label>
      <AgentProfilePicker
        profiles={[...agentProfiles]}
        value={form.agentProfileId}
        onValueChange={(value) => onChange("agentProfileId", value)}
        testId="coordinator-agent-profile-picker"
        placeholder={t("coordinator:agentProfilePlaceholder")}
        disabledOptionReason={(profile) =>
          profile.cli_passthrough ? t("coordinator:passthroughDisabledReason") : undefined
        }
        disabled={disabled}
      />
      <ProfileStatusMessage
        testId="coordinator-agent-profile-status"
        field="agent"
        status={status}
      />
      <FieldError testId="coordinator-agent-profile-error" message={message} />
    </div>
  );
}

type TaskPairFieldsetProps = {
  form: CoordinatorFormState;
  onChange: CoordinatorFormFieldsProps["onChange"];
  disabled: boolean;
  agentProfiles: CoordinatorFormFieldsProps["agentProfiles"];
  executorOptions: ReturnType<typeof useCoordinatorExecutorOptions>;
  agentStatus: ProfileStatus | undefined;
  executorStatus: ProfileStatus | undefined;
  messageOf: (field: CoordinatorErrorField) => string | undefined;
};

function TaskPairFieldset({
  form,
  onChange,
  disabled,
  agentProfiles,
  executorOptions,
  agentStatus,
  executorStatus,
  messageOf,
}: TaskPairFieldsetProps) {
  const { t } = useTranslation();
  return (
    <fieldset className="space-y-3" data-testid="coordinator-task-pair">
      <legend className="text-sm font-medium">{t("coordinator:taskPairLabel")}</legend>
      <p className="text-sm text-muted-foreground">{t("coordinator:taskPairHelp")}</p>
      <div className="space-y-1.5">
        <Label>{t("coordinator:agentProfileLabel")}</Label>
        <AgentProfilePicker
          profiles={[...agentProfiles]}
          value={form.taskAgentProfileId}
          onValueChange={(value) => onChange("taskAgentProfileId", value)}
          testId="coordinator-task-agent-profile-picker"
          placeholder={t("coordinator:agentProfilePlaceholder")}
          disabledOptionReason={(profile) =>
            profile.cli_passthrough ? t("coordinator:taskPassthroughDisabledReason") : undefined
          }
          disabled={disabled}
        />
        <ProfileStatusMessage
          testId="coordinator-task-agent-profile-status"
          field="taskAgent"
          status={agentStatus}
        />
        <FieldError
          testId="coordinator-task-agent-profile-error"
          message={messageOf("task_agent_profile_id")}
        />
      </div>
      <div className="space-y-1.5">
        <Label>{t("coordinator:executorLabel")}</Label>
        <ExecutorProfileSelector
          options={executorOptions}
          value={form.taskExecutorProfileId}
          onValueChange={(value) => onChange("taskExecutorProfileId", value)}
          disabled={disabled}
          placeholder={t("coordinator:executorPlaceholder")}
        />
        <ProfileStatusMessage
          testId="coordinator-task-executor-status"
          field="taskExecutor"
          status={executorStatus}
        />
        <FieldError
          testId="coordinator-task-executor-error"
          message={messageOf("task_executor_profile_id")}
        />
      </div>
    </fieldset>
  );
}

export function CoordinatorFormFields({
  form,
  onChange,
  disabled,
  agentProfiles,
  executors,
  agentProfileStatus,
  executorProfileStatus,
  taskAgentProfileStatus,
  taskExecutorProfileStatus,
  fieldError,
  only,
  messages,
  onLeave,
}: CoordinatorFormFieldsProps) {
  const { t } = useTranslation();
  const executorOptions = useCoordinatorExecutorOptions(executors, form.executorProfileId);
  const taskExecutorOptions = useCoordinatorExecutorOptions(executors, form.taskExecutorProfileId);
  const shows = (key: CoordinatorFieldKey) => !only || only.includes(key);
  const messageOf = (field: CoordinatorErrorField) => {
    if (messages) return messages[field];
    return fieldError?.field === field ? fieldError.message : undefined;
  };

  return (
    <div className="space-y-4">
      {shows("name") && (
        <div className="space-y-1.5">
          <Label htmlFor="coordinator-name">{t("coordinator:nameLabel")}</Label>
          <Input
            id="coordinator-name"
            value={form.name}
            maxLength={COORDINATOR_NAME_MAX_LENGTH}
            disabled={disabled}
            onChange={(event) => onChange("name", event.target.value)}
            onBlur={() => onLeave?.("name")}
          />
          <FieldError testId="coordinator-name-error" message={messageOf("name")} />
        </div>
      )}
      {shows("agent") && (
        <AgentField
          form={form}
          onChange={onChange}
          disabled={disabled}
          agentProfiles={agentProfiles}
          status={agentProfileStatus}
          message={messageOf("agent_profile_id")}
        />
      )}
      {shows("executor") && (
        <div className="space-y-1.5">
          <Label>{t("coordinator:executorLabel")}</Label>
          <ExecutorProfileSelector
            options={executorOptions}
            value={form.executorProfileId}
            onValueChange={(value) => onChange("executorProfileId", value)}
            disabled={disabled}
            placeholder={t("coordinator:executorPlaceholder")}
          />
          <ProfileStatusMessage
            testId="coordinator-executor-status"
            field="executor"
            status={executorProfileStatus}
          />
          <FieldError
            testId="coordinator-executor-error"
            message={messageOf("executor_profile_id")}
          />
        </div>
      )}
      {shows("taskPair") && (
        <TaskPairFieldset
          form={form}
          onChange={onChange}
          disabled={disabled}
          agentProfiles={agentProfiles}
          executorOptions={taskExecutorOptions}
          agentStatus={taskAgentProfileStatus}
          executorStatus={taskExecutorProfileStatus}
          messageOf={messageOf}
        />
      )}
      {shows("context") && (
        <div className="space-y-1.5">
          <Label htmlFor="coordinator-context">{t("coordinator:contextLabel")}</Label>
          <Textarea
            id="coordinator-context"
            className={LONG_TEXT_FIELD_CLASS}
            value={form.context}
            disabled={disabled}
            onChange={(event) => onChange("context", event.target.value)}
            onBlur={() => onLeave?.("context")}
          />
          <FieldError testId="coordinator-context-error" message={messageOf("context")} />
        </div>
      )}
    </div>
  );
}
