"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { resolveRuntimeOperation } from "@/lib/agent-runtime-update";
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
      <RuntimeUpdateScopeNotes preview={preview} job={job} />
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

function RuntimeUpdateScopeNotes({
  preview,
  job,
}: {
  preview: AgentUpdatePreview | null;
  job?: AgentUpdateJob;
}) {
  const { t } = useTranslation();
  if (
    !preview ||
    preview.update_mode !== "pinned" ||
    resolveRuntimeOperation(preview, job) === "migrate" ||
    preview.managed_fallback
  )
    return null;
  return (
    <div className="space-y-1 text-muted-foreground">
      <p>{t("agents:runtimeUpdateExplainer")}</p>
      <p>{t("agents:runtimeUpdateSessionsNote")}</p>
    </div>
  );
}

function RuntimeTechnicalIdentity({
  preview,
  status,
}: {
  preview: AgentUpdatePreview | null;
  status?: AgentUpdateStatus;
}) {
  const packageName = preview?.package || status?.package;
  const source = status?.source || status?.runtime_id;
  const managedFallback = Boolean(preview?.managed_fallback || status?.managed_fallback);
  if (!packageName && !source) return null;
  return (
    <dl className="my-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2">
      <RuntimePackageRow packageName={packageName} />
      <RuntimeSourceRow
        source={source}
        packageName={packageName}
        managedFallback={managedFallback}
      />
      <RuntimeOwnerRow owner={status?.owner} />
    </dl>
  );
}

function RuntimePackageRow({ packageName }: { packageName?: string }) {
  const { t } = useTranslation();
  if (!packageName) return null;
  return (
    <>
      <dt className="text-muted-foreground">{t("agents:runtimePackageLabel")}</dt>
      <dd className="wrap-anywhere text-end font-mono">{packageName}</dd>
    </>
  );
}

function RuntimeSourceRow({
  source,
  packageName,
  managedFallback,
}: {
  source?: string;
  packageName?: string;
  managedFallback: boolean;
}) {
  const { t } = useTranslation();
  if (!source || (source === packageName && !managedFallback)) return null;
  return (
    <>
      <dt className="text-muted-foreground">{t("agents:runtimeSourceLabel")}</dt>
      <dd className="wrap-anywhere text-end font-mono">{source}</dd>
    </>
  );
}

function RuntimeOwnerRow({ owner }: { owner?: string }) {
  const { t } = useTranslation();
  if (!owner) return null;
  return (
    <>
      <dt className="text-muted-foreground">{t("agents:runtimeManagedByLabel")}</dt>
      <dd className="text-end">
        {t(owner === "kandev" ? "agents:runtimeOwnerKandev" : "agents:runtimeExternalOwner")}
      </dd>
    </>
  );
}
