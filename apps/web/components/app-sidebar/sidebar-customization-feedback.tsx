import { useTranslation } from "react-i18next";

export function SidebarCustomizationFeedback({
  status,
  className = "text-xs text-destructive",
}: {
  status: "saving" | "error" | "conflict" | null;
  className?: string;
}) {
  const { t } = useTranslation();
  if (status !== "error" && status !== "conflict") return null;
  return (
    <p role="alert" className={className}>
      {t(status === "conflict" ? "settings:sidebarLayoutConflict" : "settings:sidebarSaveError")}
    </p>
  );
}
