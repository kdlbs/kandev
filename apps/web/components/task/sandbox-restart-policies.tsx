"use client";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { SandboxPermissions } from "@/components/settings/sandbox-permissions";
import type { SandboxRevision } from "@/lib/types/sandbox";
import type { useSandboxRestartReview } from "@/hooks/domains/session/use-sandbox-restart-review";
export function RestartReviewPolicies({
  model,
}: {
  model: ReturnType<typeof useSandboxRestartReview>;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="min-h-0 flex-1 space-y-5 overflow-y-auto overscroll-contain px-1 py-2 text-xs"
      data-testid="restart-review-body"
    >
      <p className="text-muted-foreground">{t("agents:restartReviewHelp")}</p>
      <CapturedRestartPolicy model={model} />
      <CurrentRestartPolicy model={model} />
      {model.error && (
        <p role="alert" className="text-destructive">
          {t(
            model.error === "stale"
              ? "agents:restartReviewStale"
              : "agents:restartReviewCurrentUnavailable",
          )}
        </p>
      )}
    </div>
  );
}
function CapturedRestartPolicy({ model }: { model: ReturnType<typeof useSandboxRestartReview> }) {
  const { t } = useTranslation();
  const captured = model.snapshot?.sandbox;
  return (
    <RestartPolicy
      title={t("agents:executionThisAttempt")}
      name={captured?.policy_name ?? ""}
      number={captured?.revision_number}
      digest={captured?.policy_digest ?? ""}
      ceiling={captured?.ceiling_digest ?? ""}
      revision={model.details.captured}
    />
  );
}
function CurrentRestartPolicy({ model }: { model: ReturnType<typeof useSandboxRestartReview> }) {
  const { t } = useTranslation();
  const captured = model.snapshot?.sandbox;
  const current = model.details.preview.data?.resolved;
  const samePolicy =
    current?.revision.digest === captured?.policy_digest &&
    current?.ceiling.digest === captured?.ceiling_digest;
  return (
    <section className="space-y-3">
      <h3 className="font-medium">{t("agents:executionNextLaunch")}</h3>
      {current ? (
        <>
          <p role="status">
            {t(samePolicy ? "agents:restartReviewSame" : "agents:restartReviewChanged")}
          </p>
          <RestartPolicy
            name={current.revision.policy.name}
            number={current.revision.number}
            digest={current.revision.digest}
            ceiling={current.ceiling.digest}
            revision={current.revision}
            expanded
          />
        </>
      ) : (
        <p className="text-muted-foreground">
          {t(
            model.details.access.canEdit
              ? "agents:restartReviewCurrentUnavailable"
              : "agents:restartReviewReadOnly",
          )}
        </p>
      )}
    </section>
  );
}
function RestartPolicy({
  title,
  name,
  number,
  digest,
  ceiling,
  revision,
  expanded = false,
}: {
  title?: string;
  name: string;
  number?: number;
  digest: string;
  ceiling: string;
  revision?: SandboxRevision;
  expanded?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <section className="min-w-0 space-y-3">
      {title && <h3 className="font-medium">{title}</h3>}
      <p className="break-words">
        {name} ({number})
      </p>
      <dl className="space-y-2">
        <div>
          <dt className="text-muted-foreground">{t("agents:executionPolicyDigest")}</dt>
          <dd className="break-all font-mono">{digest}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">{t("agents:sandboxExecutorCeiling")}</dt>
          <dd className="break-all font-mono">{ceiling}</dd>
        </div>
      </dl>
      {revision && (
        <details open={expanded} className="group/permissions">
          <summary className="flex min-h-12 cursor-pointer items-center gap-2 md:min-h-7 [@media(pointer:coarse)]:min-h-12">
            <IconChevronDown
              aria-hidden="true"
              stroke={1.5}
              className="size-3.5 shrink-0 group-open/permissions:rotate-180"
            />
            {t("agents:restartPermissions")}
          </summary>
          <SandboxPermissions grants={revision.policy.grants} />
        </details>
      )}
    </section>
  );
}
