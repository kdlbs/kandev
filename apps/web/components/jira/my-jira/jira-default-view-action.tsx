import { IconStar } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

export function DefaultViewAction({
  viewId,
  viewName,
  isDefault,
  disabled,
  onSetDefault,
  isFinePointer,
}: {
  viewId: string;
  viewName: string;
  isDefault: boolean;
  disabled: boolean;
  onSetDefault: (id: string) => void;
  isFinePointer: boolean;
}) {
  const { t } = useTranslation();
  const label = isDefault
    ? t("jira:clearViewAsDefault", { name: viewName })
    : t("jira:setViewAsDefault", { name: viewName });
  const finePointerVisibility = isDefault
    ? "opacity-100"
    : "opacity-0 group-hover:opacity-100 focus:opacity-100 disabled:opacity-50";
  const className = cn(
    "flex shrink-0 cursor-pointer items-center justify-center rounded hover:bg-muted disabled:cursor-not-allowed",
    isFinePointer ? cn("h-7 w-7", finePointerVisibility) : "h-12 w-12 opacity-100",
    isDefault && "text-amber-500",
  );
  return (
    <button
      type="button"
      disabled={disabled}
      aria-label={label}
      onClick={() => onSetDefault(isDefault ? "" : viewId)}
      className={className}
      title={label}
    >
      <IconStar className={cn("h-3.5 w-3.5", isDefault && "fill-amber-500")} />
    </button>
  );
}
