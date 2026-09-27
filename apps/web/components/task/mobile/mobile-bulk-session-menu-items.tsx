import { DropdownMenuItem, DropdownMenuSeparator } from "@kandev/ui/dropdown-menu";
import { useTranslation } from "react-i18next";

export function MobileBulkSessionMenuItems({
  onRemoveScope,
}: {
  onRemoveScope: (scope: "others" | "all") => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DropdownMenuSeparator />
      <DropdownMenuItem
        className="cursor-pointer text-destructive focus:text-destructive"
        onSelect={() => onRemoveScope("others")}
      >
        {t("task:removeOthers")}
      </DropdownMenuItem>
      <DropdownMenuItem
        className="cursor-pointer text-destructive focus:text-destructive"
        onSelect={() => onRemoveScope("all")}
      >
        {t("task:removeAll")}
      </DropdownMenuItem>
    </>
  );
}
