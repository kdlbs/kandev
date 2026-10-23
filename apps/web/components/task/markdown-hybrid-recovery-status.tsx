import { useTranslation } from "react-i18next";

export function MarkdownHybridRecoveryStatus({
  unavailable,
  failed,
}: {
  unavailable: boolean;
  failed: boolean;
}) {
  const { t } = useTranslation();
  if (!unavailable && !failed) return null;
  return (
    <p role="status" className="shrink-0 px-3 py-2 text-xs text-muted-foreground">
      {t(unavailable ? "task:markdownEditUnavailable" : "task:markdownEditRecovery")}
    </p>
  );
}
