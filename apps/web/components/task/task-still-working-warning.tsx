import { IconAlertTriangle } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

export function StillWorkingWarning({ count }: { count?: number }) {
  const { t } = useTranslation();
  const subject =
    count && count > 1 ? t("task:stillWorkingSubjectMany") : t("task:stillWorkingSubjectOne");
  return (
    <div
      data-testid="still-working-warning"
      role="alert"
      className="flex items-start gap-2 rounded-md border border-yellow-500/40 bg-yellow-500/10 p-3 text-sm text-yellow-700 dark:text-yellow-300"
    >
      <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-yellow-500" aria-hidden />
      <span>{t("task:stillWorkingWarning", { subject })}</span>
    </div>
  );
}
