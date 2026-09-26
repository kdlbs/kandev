import { IconLink } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";

export function SymlinkIndicator({
  isSymlink,
  showLabel = false,
}: {
  isSymlink?: boolean;
  showLabel?: boolean;
}) {
  const { t } = useTranslation();
  if (!isSymlink) return null;
  return (
    <span
      className="inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground"
      data-testid="symlink-indicator"
    >
      <IconLink className="size-3.5" aria-hidden="true" />
      <span className={showLabel ? undefined : "sr-only"}>{t("editors:symlink")}</span>
    </span>
  );
}
