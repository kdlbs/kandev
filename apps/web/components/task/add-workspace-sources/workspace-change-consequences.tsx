import { IconAlertTriangle } from "@tabler/icons-react";
import { Trans, useTranslation } from "react-i18next";

export function WorkspaceChangeConsequences({ restartsWorkspace }: { restartsWorkspace: boolean }) {
  const { t } = useTranslation();
  return (
    <section
      role="note"
      aria-labelledby="workspace-change-consequences-title"
      data-testid="workspace-change-consequences"
      className="rounded-lg border border-amber-500/35 bg-amber-500/8 p-3 text-xs/relaxed"
    >
      <div className="flex items-start gap-2.5">
        <IconAlertTriangle
          aria-hidden="true"
          className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400"
        />
        <div className="min-w-0 space-y-2.5">
          <div className="space-y-1">
            <h3 id="workspace-change-consequences-title" className="font-medium text-foreground">
              {restartsWorkspace
                ? t("task:thisRestartsTheTaskWorkspace")
                : t("task:thisUpdatesTheLiveTaskWorkspace")}
            </h3>
            <p className="text-muted-foreground">
              {t("task:reviewTheseChangesBeforeAddingSources")}
            </p>
          </div>
          {restartsWorkspace ? <RestartSummary /> : <LiveUpdateSummary />}
          <details className="group">
            <summary className="flex min-h-11 cursor-pointer items-center font-medium text-foreground">
              {t("task:fullImpactDetails")}
            </summary>
            <div className="pb-1">
              {restartsWorkspace ? <RestartConsequences /> : <LiveUpdateConsequences />}
            </div>
          </details>
          <p className="border-t border-amber-500/20 pt-2 text-muted-foreground">
            <Trans i18nKey="task:cancelLeavesWorkspaceUnchangedDetail">
              <strong className="font-medium text-foreground">
                Cancel leaves the workspace unchanged.
              </strong>{" "}
              If you continue, the batch is all-or-nothing: when any source fails, none of the new
              sources are attached.
            </Trans>
          </p>
        </div>
      </div>
    </section>
  );
}

function RestartSummary() {
  const { t } = useTranslation();
  return (
    <ul className="list-disc space-y-1.5 pl-4 text-muted-foreground">
      <li>{t("task:theIdleAgentRestartsAtThe")}</li>
      <li>{t("task:providerPrivateContextThatKandevDid")}</li>
    </ul>
  );
}

function LiveUpdateSummary() {
  const { t } = useTranslation();
  return (
    <ul className="list-disc space-y-1.5 pl-4 text-muted-foreground">
      <li>{t("task:repositoriesAreAddedAsTopLevel")}</li>
      <li>{t("task:theAgentAndRunningWorkspaceProcesses")}</li>
    </ul>
  );
}

function RestartConsequences() {
  return (
    <ul className="list-disc space-y-1.5 pl-4 text-muted-foreground">
      <li>
        <Trans i18nKey="task:restartConsequenceWorkspace">
          <strong className="font-medium text-foreground">Workspace:</strong> The task root becomes
          the agent&apos;s working directory. For a single-repository task, this moves the CWD up
          one level and shows every source as a named top-level entry. Existing files and Git
          changes are not moved or discarded.
        </Trans>
      </li>
      <li>
        <Trans i18nKey="task:restartConsequenceSessionContext">
          <strong className="font-medium text-foreground">Session context:</strong> Kandev restarts
          the idle agent while preserving the task, session, task state, messages, plan, attached
          sources, and selected model and mode. Providers that support cross-directory resume keep
          their native session. Other providers start a new session and receive Kandev&apos;s
          recorded conversation with the next prompt. Provider-private context that Kandev did not
          record may not carry over.
        </Trans>
      </li>
      <li>
        <Trans i18nKey="task:restartConsequenceRunningProcesses">
          <strong className="font-medium text-foreground">Running processes:</strong> Open
          terminals, dev servers, and other workspace processes stop. This includes the task editor
          server. Save unsaved work, then reopen or restart those processes after the sources are
          attached.
        </Trans>
      </li>
    </ul>
  );
}

function LiveUpdateConsequences() {
  return (
    <ul className="list-disc space-y-1.5 pl-4 text-muted-foreground">
      <li>
        <Trans i18nKey="task:liveUpdateConsequenceWorkspace">
          <strong className="font-medium text-foreground">Workspace:</strong> Repositories are
          cloned as named top-level entries under the current remote workspace. The agent&apos;s
          working directory does not change, and existing files and Git changes are not moved or
          discarded.
        </Trans>
      </li>
      <li>
        <Trans i18nKey="task:liveUpdateConsequenceSessionAndProcesses">
          <strong className="font-medium text-foreground">Session and processes:</strong> The agent
          and running workspace processes continue while Kandev rescans the workspace. The task,
          session, task state, messages, plan, attached sources, model, and mode remain in place.
        </Trans>
      </li>
    </ul>
  );
}
