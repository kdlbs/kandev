import { useTranslation } from "react-i18next";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { useBulkSessionRemoval } from "../session-bulk-removal";

export function MobileBulkSessionRemovalConfirmation({
  taskId,
  bulkRemoval,
}: {
  taskId: string;
  bulkRemoval: ReturnType<typeof useBulkSessionRemoval>;
}) {
  const { t } = useTranslation();
  return (
    <MobileActionConfirmation
      open={!!bulkRemoval.snapshot}
      targetKey={`${taskId}:${bulkRemoval.snapshot?.scope ?? "none"}`}
      title={t("task:removeSessionsTitle", { count: bulkRemoval.snapshot?.targetIds.length ?? 0 })}
      description={t("task:removeSessionsDescription", {
        count: bulkRemoval.snapshot?.targetIds.length ?? 0,
      })}
      cancelLabel={t("common:cancel")}
      confirmLabel={
        bulkRemoval.snapshot?.scope === "others" ? t("task:removeOthers") : t("task:removeAll")
      }
      confirmTestId="mobile-bulk-session-remove-confirm"
      completionPolicy="await-with-retry"
      testId="mobile-bulk-session-remove-confirmation"
      onOpenChange={(open) => !open && bulkRemoval.cancel()}
      onCancel={bulkRemoval.cancel}
      onConfirm={async () => {
        const result = await bulkRemoval.confirm();
        if (result?.stale) return Promise.reject();
      }}
    >
      {bulkRemoval.pending && (
        <p role="status">
          {t("task:bulkRemovalProgress", {
            removed: bulkRemoval.removedCount,
            total: bulkRemoval.snapshot?.targetIds.length ?? 0,
          })}
        </p>
      )}
      {bulkRemoval.wasRefreshed && <p role="status">{t("task:sessionsChangedReviewAgain")}</p>}
    </MobileActionConfirmation>
  );
}
