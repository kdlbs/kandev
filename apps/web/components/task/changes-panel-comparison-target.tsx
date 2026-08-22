import { useTranslation } from "react-i18next";

export function ComparisonTargetDisplay({ targets }: { targets: string[] }) {
  const { t } = useTranslation();
  if (targets.length === 0) return null;

  return (
    <div className="border-t pt-2 text-muted-foreground" data-testid="changes-comparison-target">
      <p className="font-medium text-foreground">{t("task:comparisonTargetLabel")}</p>
      <p
        className="max-w-[min(80vw,28rem)] break-words font-mono text-[11px]"
        title={targets.join(", ")}
      >
        {targets.join(", ")}
      </p>
    </div>
  );
}
