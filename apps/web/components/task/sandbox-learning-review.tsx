"use client";

import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowLeft, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { DialogClose, DialogContent, DialogDescription, DialogTitle } from "@kandev/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { useSandboxLearning } from "@/hooks/domains/session/use-sandbox-learning";
import type { SandboxLearningProposal } from "@/lib/api/domains/sandbox-learning-api";

export type LearningModel = ReturnType<typeof useSandboxLearning>;
const actionClass = "h-7 min-h-7 max-md:min-h-12 pointer-coarse:min-h-11 cursor-pointer";

export function SandboxLearningReview({
  model,
  selectedIds,
}: {
  model: LearningModel;
  selectedIds: string[];
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [name, setName] = useState(() => t("agents:learningDefaultName"));
  const [scope, setScope] = useState<"workspace" | "installation">("workspace");
  return (
    <DialogContent
      showCloseButton={false}
      enterConfirms={false}
      className={
        isMobile
          ? "inset-0 h-dvh max-h-dvh w-full max-w-none sm:max-w-none translate-x-0 translate-y-0 rounded-none border-0 flex flex-col overflow-hidden pt-[max(1rem,env(safe-area-inset-top))] pb-[max(1rem,env(safe-area-inset-bottom))]"
          : "flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-xl"
      }
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onDoubleClick={(event) => event.stopPropagation()}
      onKeyDown={(event) => event.stopPropagation()}
    >
      <div className="flex shrink-0 items-start gap-3">
        {isMobile && (
          <DialogClose asChild>
            <Button type="button" variant="ghost" className="h-12 min-h-12 px-3 cursor-pointer">
              <IconArrowLeft aria-hidden className="size-4" />
              {t("common:back")}
            </Button>
          </DialogClose>
        )}
        <div className="min-w-0 flex-1">
          <DialogTitle>{t("agents:learningTitle")}</DialogTitle>
          <DialogDescription className="mt-2">{t("agents:learningHelp")}</DialogDescription>
        </div>
        {!isMobile && (
          <DialogClose asChild>
            <Button type="button" variant="ghost" size="icon-sm" aria-label={t("common:close")}>
              <IconX aria-hidden className="size-4" />
            </Button>
          </DialogClose>
        )}
      </div>
      <div
        className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-1 py-3 text-sm"
        data-testid="sandbox-learning-body"
      >
        {model.failed && <p role="alert">{t("agents:learningFailed")}</p>}
        {model.isBusy && <p role="status">{t("common:loading")}</p>}
        <LearningContent
          model={model}
          selectedIds={selectedIds}
          name={name}
          setName={setName}
          scope={scope}
          setScope={setScope}
        />
        {!model.canPropose && !model.canPublish && !model.isBusy && !model.revision && (
          <p>{t("agents:learningUnavailable")}</p>
        )}
      </div>
      <div className="shrink-0 border-t pt-3 flex justify-end gap-2">
        <LearningActions model={model} selectedIds={selectedIds} name={name} scope={scope} />
      </div>
    </DialogContent>
  );
}

function LearningDifference({
  proposal,
  baseName,
}: {
  proposal: SandboxLearningProposal;
  baseName: string;
}) {
  const { t } = useTranslation();
  const grants = proposal.added_grants;
  return (
    <div className="space-y-4">
      <p className="font-medium break-words">{proposal.policy.name}</p>
      <p>
        {t("agents:learningScope")}:{" "}
        {t(
          proposal.policy.workspace_id ? "agents:learningWorkspace" : "agents:learningInstallation",
        )}
      </p>
      {!proposal.report_complete && (
        <p className="text-muted-foreground">{t("agents:reportPartial")}</p>
      )}
      <section className="space-y-2">
        <h3 className="font-medium">{t("agents:learningAddedAccess")}</h3>
        <ul className="space-y-2 break-all">
          {(grants.filesystem ?? []).map((grant) => (
            <li key={grant.path ?? grant.reference}>
              {t(grant.writable ? "agents:sandboxAccessWrite" : "agents:sandboxAccessRead")}:{" "}
              <code>{grant.path ?? grant.reference}</code>
            </li>
          ))}
          {(grants.network ?? []).map((grant) => (
            <li key={`${grant.protocol}:${grant.host}:${grant.port}`}>
              {t("agents:sandboxNetwork")}:{" "}
              <code>
                {grant.protocol}://{grant.host}:{grant.port}
              </code>
            </li>
          ))}
          {(grants.environment_keys ?? []).map((key) => (
            <li key={key}>
              {t("agents:sandboxEnvironment")}: <code>{key}</code>
            </li>
          ))}
          {grants.kandev_mcp && <li>{t("agents:sandboxMCPAllowed")}</li>}
        </ul>
      </section>
      <dl className="space-y-3 break-all text-xs">
        <div>
          <dt className="font-medium">{t("agents:learningSource")}</dt>
          <dd>{baseName}</dd>
          <dd>{proposal.base_revision_id}</dd>
          <dd className="font-mono">{proposal.base_digest}</dd>
        </div>
        <div>
          <dt className="font-medium">{t("agents:executionCapturedCeiling")}</dt>
          <dd className="font-mono">{proposal.ceiling_digest}</dd>
        </div>
        <div>
          <dt className="font-medium">{t("agents:reportDigest")}</dt>
          <dd className="font-mono">{proposal.report_digest}</dd>
        </div>
      </dl>
      <p>{t("agents:learningSelectionHelp")}</p>
    </div>
  );
}

type LearningDraftProps = {
  model: LearningModel;
  selectedIds: string[];
  name: string;
  scope: "workspace" | "installation";
};
function LearningContent({
  model,
  selectedIds,
  name,
  setName,
  scope,
  setScope,
}: LearningDraftProps & {
  setName: (value: string) => void;
  setScope: (value: "workspace" | "installation") => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  if (model.revision)
    return (
      <div className="space-y-3">
        <p role="status">{t("agents:learningCreated", { name: model.revision.policy.name })}</p>
        <p>{t("agents:learningSelectionHelp")}</p>
      </div>
    );
  if (model.proposal)
    return (
      <LearningDifference
        proposal={model.proposal}
        baseName={model.snapshot?.sandbox?.policy_name ?? ""}
      />
    );
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor={`${id}-name`}>{t("agents:sandboxPolicyName")}</Label>
        <Input
          id={`${id}-name`}
          value={name}
          onChange={(event) => setName(event.target.value)}
          maxLength={128}
          className="h-7 max-md:min-h-11 pointer-coarse:min-h-11"
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${id}-scope`}>{t("agents:learningScope")}</Label>
        <Select value={scope} onValueChange={(value) => setScope(value as typeof scope)}>
          <SelectTrigger id={`${id}-scope`} className="h-7 max-md:min-h-11 pointer-coarse:min-h-11">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="workspace">{t("agents:learningWorkspace")}</SelectItem>
            <SelectItem value="installation">{t("agents:learningInstallation")}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <p>{t("agents:learningSelected", { count: selectedIds.length })}</p>
    </div>
  );
}
function LearningActions({ model, selectedIds, name, scope }: LearningDraftProps) {
  const { t } = useTranslation();
  if (model.revision)
    return (
      <DialogClose asChild>
        <Button type="button" className={actionClass}>
          {t("common:close")}
        </Button>
      </DialogClose>
    );
  if (model.proposal)
    return (
      <Button
        type="button"
        className={actionClass}
        disabled={!model.canPublish}
        onClick={() => void model.publish()}
      >
        {t("agents:learningPublish")}
      </Button>
    );
  return (
    <Button
      type="button"
      className={actionClass}
      disabled={!model.canPropose || !name.trim() || selectedIds.length === 0}
      onClick={() => void model.propose({ name, scope, record_ids: selectedIds })}
    >
      {t("agents:learningReviewChanges")}
    </Button>
  );
}
