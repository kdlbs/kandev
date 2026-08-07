import { useTranslation } from "react-i18next";
import type { SessionDeleteWarning } from "@/hooks/use-session-delete-warning";

/**
 * Renders the uncommitted-files / unpushed-commits warning lines for the
 * session-delete confirmation dialogs. Shared between the desktop tab dialog
 * and the mobile Sessions picker dialog so both stay in sync
 * (docs/specs/session-delete-resource-cleanup): each count is shown only
 * when non-zero, and a clean, fully-pushed session renders nothing.
 */
export function SessionDeleteWarningLines({ warning }: { warning: SessionDeleteWarning | null }) {
  const { t } = useTranslation();
  if (!warning) return null;
  const { uncommittedFiles, unpushedCommits } = warning;
  if (uncommittedFiles === 0 && unpushedCommits === 0) return null;
  return (
    <>
      {uncommittedFiles > 0 && (
        <p className="mt-2 font-medium" data-testid="session-delete-uncommitted-warning">
          {t("task:sessionDeleteUncommittedFilesWarning", { count: uncommittedFiles })}
        </p>
      )}
      {unpushedCommits > 0 && (
        <p className="mt-2 font-medium" data-testid="session-delete-unpushed-warning">
          {t("task:sessionDeleteUnpushedCommitsWarning", { count: unpushedCommits })}
        </p>
      )}
    </>
  );
}
