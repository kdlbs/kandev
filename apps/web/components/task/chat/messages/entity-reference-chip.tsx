import { useState, type MouseEvent } from "react";
import type { TFunction } from "i18next";
import { IconHash } from "@tabler/icons-react";
import type { ComponentPropsWithoutRef } from "react";
import type { Components, ExtraProps } from "react-markdown";
import type { EntityReference } from "@/lib/types/entity-reference";
import {
  entityReferenceHref,
  entityReferenceLabel,
  matchEntityReferenceLink,
} from "@/lib/entity-references/message-references";
import { markdownComponents } from "@/components/shared/markdown-components";
import { useTranslation } from "react-i18next";
import { actTaskChangeRegistration } from "@/lib/api/domains/task-change-registration-api";

type EntityReferenceChipProps = {
  reference: EntityReference;
  taskId?: string;
};

const TERMINAL_REGISTRATION_ERRORS = [
  "identity_unresolved",
  "reference_invalid",
  "repository_changed",
  "origin_unavailable",
] as const; // i18n-exempt: backend error codes, not copy.

function registrationNumber(reference: EntityReference): string {
  const tail = reference.url.split(/[/?#]/u).filter(Boolean).at(-1);
  return tail && /^\d+$/u.test(tail) ? tail : reference.id;
}

function getRegistrationLabel(
  reference: EntityReference,
  workingAction: "retry" | "relink" | null,
  t: TFunction,
): string | null {
  if (reference.association_mode === "context_only") return t("task:referenceContextOnly");
  if (workingAction || reference.registration_status === "pending") {
    return t("task:referenceRegistrationPending");
  }
  switch (reference.registration_status) {
    case "linked":
      return t(
        reference.provider === "gitlab"
          ? "task:referenceRegistrationLinkedMergeRequest"
          : "task:referenceRegistrationLinkedPullRequest",
        { number: registrationNumber(reference) },
      );
    case "failed":
      return t("task:referenceRegistrationFailed");
    case "suppressed":
      return t("task:referenceRegistrationSuppressed");
    case "unlinked":
      return t("task:referenceRegistrationUnlinked");
    case "uncertain":
      return t("task:referenceRegistrationUncertain");
    default:
      return reference.association_mode === "link_on_send" ? t("task:referenceLinkOnSend") : null;
  }
}

function canRetryRegistration(
  reference: EntityReference,
  taskId: string | undefined,
  workingAction: "retry" | "relink" | null,
): boolean {
  if (!taskId || !reference.selection_id || workingAction !== null) return false;
  if (reference.registration_status === "uncertain") return true;
  return (
    reference.registration_status === "failed" &&
    !TERMINAL_REGISTRATION_ERRORS.includes(
      reference.registration_error_code as (typeof TERMINAL_REGISTRATION_ERRORS)[number],
    )
  );
}

function canRelinkRegistration(
  reference: EntityReference,
  taskId: string | undefined,
  workingAction: "retry" | "relink" | null,
): boolean {
  return Boolean(
    taskId &&
    reference.selection_id &&
    reference.registration_status === "unlinked" &&
    workingAction === null,
  );
}

function useRegistrationActions(reference: EntityReference, taskId: string | undefined) {
  const [workingAction, setWorkingAction] = useState<"retry" | "relink" | null>(null);
  const [retryFailed, setRetryFailed] = useState(false);
  const [relinkFailed, setRelinkFailed] = useState(false);

  const runAction = async (action: "retry" | "relink", event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault();
    event.stopPropagation();
    if (!taskId || !reference.selection_id || workingAction !== null) return;
    setWorkingAction(action);
    setRetryFailed(false);
    setRelinkFailed(false);
    try {
      await actTaskChangeRegistration(taskId, reference.selection_id, action);
    } catch {
      if (action === "retry") setRetryFailed(true);
      else setRelinkFailed(true);
    } finally {
      setWorkingAction(null);
    }
  };

  return { workingAction, retryFailed, relinkFailed, runAction };
}

export function EntityReferenceChip({ reference, taskId }: EntityReferenceChipProps) {
  const { t } = useTranslation();
  const { workingAction, retryFailed, relinkFailed, runAction } = useRegistrationActions(
    reference,
    taskId,
  );
  const label = entityReferenceLabel(reference);
  const isInternal = reference.url.startsWith("/") || reference.url.startsWith("#");
  const registrationLabel = getRegistrationLabel(reference, workingAction, t);
  const canRetry = canRetryRegistration(reference, taskId, workingAction);
  const canRelink = canRelinkRegistration(reference, taskId, workingAction);

  return (
    <span className="inline-flex max-w-full flex-wrap items-center gap-x-1 gap-y-0.5 align-baseline">
      <a
        href={entityReferenceHref(reference)}
        target={isInternal ? "_self" : "_blank"}
        rel={isInternal ? undefined : "noopener noreferrer"}
        aria-label={t("task:openEntityReference", { label, title: reference.title })}
        title={`${reference.provider} ${reference.kind}: ${reference.title}`}
        data-testid="entity-reference-chip"
        data-entity-ref={reference.ref}
        data-entity-provider={reference.provider}
        data-entity-kind={reference.kind}
        className="inline-flex max-w-full items-center gap-0.5 rounded-md bg-primary/15 px-1.5 py-0.5 align-baseline text-[0.88em] font-medium text-primary no-underline ring-1 ring-inset ring-primary/20 hover:bg-primary/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
      >
        <IconHash className="h-3 w-3 shrink-0" aria-hidden="true" />
        <span className="min-w-0 truncate">{reference.key || reference.title}</span>
      </a>
      {registrationLabel ? (
        <span
          className="text-[0.78em] text-muted-foreground"
          aria-live={reference.registration_status === "pending" ? "polite" : undefined}
          data-testid="entity-reference-registration-status"
        >
          {registrationLabel}
        </span>
      ) : null}
      {canRetry ? (
        <button
          type="button"
          className="min-h-7 cursor-pointer rounded px-2 text-xs font-medium text-primary underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          aria-label={t("task:referenceRegistrationRetry")}
          data-testid="entity-reference-registration-retry"
          disabled={workingAction !== null}
          onClick={(event) => runAction("retry", event)}
        >
          {workingAction === "retry"
            ? t("task:referenceRegistrationRetrying")
            : t("task:referenceRegistrationRetry")}
        </button>
      ) : null}
      {canRelink ? (
        <button
          type="button"
          className="min-h-7 cursor-pointer rounded px-2 text-xs font-medium text-primary underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          aria-label={t("task:referenceRegistrationLinkAgain")}
          data-testid="entity-reference-registration-link-again"
          disabled={workingAction !== null}
          onClick={(event) => runAction("relink", event)}
        >
          {workingAction === "relink"
            ? t("task:referenceRegistrationPending")
            : t("task:referenceRegistrationLinkAgain")}
        </button>
      ) : null}
      {retryFailed ? (
        <span className="text-xs text-destructive" role="alert">
          {t("task:referenceRegistrationRetryFailed")}
        </span>
      ) : null}
      {relinkFailed ? (
        <span className="text-xs text-destructive" role="alert">
          {t("task:referenceRegistrationRelinkFailed")}
        </span>
      ) : null}
    </span>
  );
}

type EntityReferenceAnchorProps = ComponentPropsWithoutRef<"a"> &
  ExtraProps & {
    references: readonly EntityReference[];
    taskId?: string;
  };

function EntityReferenceAnchor({
  references,
  taskId,
  children,
  href,
  ...props
}: EntityReferenceAnchorProps) {
  const node = props.node;
  const textChild = node?.children.length === 1 ? node.children[0] : null;
  const label = textChild?.type === "text" ? textChild.value : null;
  const reference = label === null ? null : matchEntityReferenceLink(references, label, href);
  if (reference) return <EntityReferenceChip reference={reference} taskId={taskId} />;
  const MarkdownAnchor = markdownComponents.a;
  return (
    <MarkdownAnchor href={href} {...props}>
      {children}
    </MarkdownAnchor>
  );
}

export function buildEntityReferenceMarkdownComponents(
  references: readonly EntityReference[],
  taskId?: string,
): Components {
  if (references.length === 0) return markdownComponents;
  return {
    ...markdownComponents,
    a: (props) => <EntityReferenceAnchor {...props} references={references} taskId={taskId} />,
  };
}
