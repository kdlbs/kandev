"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import type { AgentUpdateJob, AgentUpdatePreview, AgentUpdateStatus } from "@/lib/api";
import { RuntimeGuidanceAction } from "./agent-runtime-policies";
import { RuntimeVersionFacts } from "./runtime-version-summary";
import { settingsActionClassName } from "./settings-control";

function RuntimeUpdateDisclosure({
  title,
  testId,
  className,
  children,
}: {
  title: string;
  testId: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <details className={className} data-testid={testId}>
      <summary
        role="button"
        className={settingsActionClassName(
          "flex cursor-pointer list-none items-center justify-between gap-2 focus-visible:outline-ring [&::-webkit-details-marker]:hidden [details[open]>&>svg]:rotate-180",
        )}
      >
        {title}
        <IconChevronDown className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
      </summary>
      {children}
    </details>
  );
}

export function RuntimeUpdateTechnicalDetails({
  agentName,
  preview,
  status,
  job,
}: {
  agentName: string;
  preview: AgentUpdatePreview | null;
  status?: AgentUpdateStatus;
  job?: AgentUpdateJob;
}) {
  const { t } = useTranslation();
  const command = preview?.command_string;
  const output = job?.status === "succeeded" ? job.output : undefined;
  if (!status && !preview?.package && !command && !output) return null;
  return (
    <RuntimeUpdateDisclosure
      title={t("agents:runtimeDetailsLabel")}
      testId={`agent-update-runtime-details-${agentName}`}
      className="border-t py-1"
    >
      <RuntimeTechnicalIdentity preview={preview} status={status} />
      {preview && (
        <div className="space-y-1 text-muted-foreground empty:hidden">
          <RuntimeVersionFacts preview={preview} job={job} />
        </div>
      )}
      {command && (
        <RuntimeUpdateDisclosure
          title={t("agents:commandPreview")}
          testId={`agent-update-command-${agentName}`}
          className="mt-2 border-t pt-1"
        >
          <pre className="wrap-anywhere my-1 whitespace-pre-wrap rounded-md bg-muted p-2 font-mono text-xs">
            {command}
          </pre>
        </RuntimeUpdateDisclosure>
      )}
      {output && (
        <RuntimeUpdateDisclosure
          title={t("agents:runtimeUpdateOutput")}
          testId={`agent-update-output-${agentName}`}
        >
          <pre
            data-testid={`agent-update-log-${agentName}`}
            className="wrap-anywhere my-1 whitespace-pre-wrap rounded-md bg-muted p-2 font-mono text-xs"
          >
            {output}
          </pre>
        </RuntimeUpdateDisclosure>
      )}
      {status && <RuntimeGuidanceAction status={status} inline />}
    </RuntimeUpdateDisclosure>
  );
}

function RuntimeTechnicalIdentity({
  preview,
  status,
}: {
  preview: AgentUpdatePreview | null;
  status?: AgentUpdateStatus;
}) {
  const { t } = useTranslation();
  const packageName = preview?.package || status?.package;
  const source = packageName || status?.source || status?.runtime_id;
  const kandevManaged = status?.owner === "kandev" || preview?.update_mode === "pinned";
  if (!source) return null;
  return (
    <dl className="my-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2">
      <dt className="text-muted-foreground">
        {t(packageName ? "agents:runtimePackageLabel" : "agents:runtimeSourceLabel")}
      </dt>
      <dd className="wrap-anywhere text-end font-mono">{source}</dd>
      <dt className="text-muted-foreground">{t("agents:runtimeManagedByLabel")}</dt>
      <dd className="text-end">
        {t(kandevManaged ? "agents:runtimeOwnerKandev" : "agents:runtimeExternalOwner")}
      </dd>
    </dl>
  );
}
