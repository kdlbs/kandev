import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { FailedInboxRow } from "@/components/needs-you-inbox/failed-inbox-row";
import { useIntegratedControl } from "./integrated-control-provider";
import { useControlInbox } from "./control-inbox-provider";
import {
  groupInboxSection,
  type ControlInboxGroup,
  type ControlInboxReadStatus,
  type ControlInboxSectionKey,
} from "./control-inbox-model";
import {
  ControlAnswerableRow,
  ControlEarlierQuestionRow,
  ControlRecoveryBlockRow,
} from "./control-inbox-rows";

const STATUS_KEYS: Record<Exclude<ControlInboxReadStatus, "ready">, string> = {
  denied: "sidebar:controlInbox.readDenied",
  unsupported: "sidebar:controlInbox.readUnsupported",
  error: "sidebar:controlInbox.readError",
};

/** Every accessible workspace's Needs-you items, one section per kind. */
export function ControlInbox() {
  const { t } = useTranslation();
  const inbox = useControlInbox();
  if (!inbox?.enabled)
    return (
      <p className="text-sm text-muted-foreground" data-testid="control-inbox-disabled">
        {t("sidebar:controlInbox.disabled")}
      </p>
    );
  if (inbox.loading)
    return (
      <p role="status" className="text-sm text-muted-foreground">
        {t("common:loading")}
      </p>
    );
  const reads = inbox.reads;
  return (
    <div className="space-y-6" data-testid="control-inbox">
      <p className="text-sm text-muted-foreground">
        {t("sidebar:controlInbox.summary", {
          answerable: inbox.counts.answerable,
          earlier: inbox.counts.earlier,
          recovery: inbox.counts.recovery,
        })}
      </p>
      <InboxSection kind="answerable" groups={groupInboxSection(reads, (read) => read.answerable)}>
        {(bundle, workspaceId) => (
          <ControlAnswerableRow key={bundle.pending_id} bundle={bundle} workspaceId={workspaceId} />
        )}
      </InboxSection>
      <InboxSection kind="earlier" groups={groupInboxSection(reads, (read) => read.earlier)}>
        {(bundle, workspaceId) => (
          <ControlEarlierQuestionRow
            key={bundle.pending_id}
            bundle={bundle}
            workspaceId={workspaceId}
          />
        )}
      </InboxSection>
      <InboxSection kind="recovery" groups={groupInboxSection(reads, (read) => read.recovery)}>
        {(block) => <ControlRecoveryBlockRow key={block.block_id} block={block} />}
      </InboxSection>
      <InboxSection kind="failed" groups={groupInboxSection(reads, (read) => read.failed)}>
        {(row) => <FailedInboxRow key={row.task_id} row={row} />}
      </InboxSection>
    </div>
  );
}

function InboxSection<T>({
  kind,
  groups,
  children,
}: {
  kind: ControlInboxSectionKey;
  groups: ControlInboxGroup<T>[];
  children: (item: T, workspaceId: string) => ReactNode;
}) {
  const { t } = useTranslation();
  const count = groups.reduce((sum, group) => sum + group.items.length, 0);
  return (
    <section className="space-y-2" data-testid={`control-inbox-section-${kind}`}>
      <div>
        <h2 className="text-sm font-semibold">
          {t(`sidebar:controlInbox.section.${kind}`, { count })}
        </h2>
        <p className="text-xs text-muted-foreground">
          {t(`sidebar:controlInbox.sectionHint.${kind}`)}
        </p>
      </div>
      {groups.length === 0 ? (
        <p className="rounded-lg border px-4 py-3 text-sm text-muted-foreground">
          {t(`sidebar:controlInbox.empty.${kind}`)}
        </p>
      ) : (
        groups.map((group) => (
          <WorkspaceGroup key={group.workspaceId} group={group}>
            {children}
          </WorkspaceGroup>
        ))
      )}
    </section>
  );
}

function WorkspaceGroup<T>({
  group,
  children,
}: {
  group: ControlInboxGroup<T>;
  children: (item: T, workspaceId: string) => ReactNode;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const name =
    control?.data?.workspaces.find((workspace) => workspace.id === group.workspaceId)?.name ??
    group.workspaceId;
  return (
    <div
      className="space-y-1"
      data-testid="control-inbox-workspace"
      data-workspace-id={group.workspaceId}
    >
      <h3 className="text-xs font-medium text-muted-foreground">
        {t("sidebar:controlInbox.workspaceHeading", { name, count: group.items.length })}
      </h3>
      {group.status !== "ready" && (
        <p role="alert" className="rounded-lg border px-4 py-2 text-xs text-destructive">
          {t(STATUS_KEYS[group.status])}
        </p>
      )}
      {group.items.length > 0 && (
        <div className="divide-y divide-border overflow-hidden rounded-lg border border-border">
          {group.items.map((item) => children(item, group.workspaceId))}
        </div>
      )}
      {group.truncated && (
        <p className="text-xs text-muted-foreground">{t("sidebar:controlInbox.truncated")}</p>
      )}
    </div>
  );
}
