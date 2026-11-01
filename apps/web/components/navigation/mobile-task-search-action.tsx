import { Button } from "@kandev/ui/button";
import { IconSearch } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { useCommandPanelOpen } from "@/lib/commands/command-registry";

export function MobileTaskSearchAction({ closeMenu }: { closeMenu: () => void }) {
  const { t } = useTranslation();
  const { openMode } = useCommandPanelOpen();
  return (
    <Button
      variant="outline"
      className="min-h-11 w-full justify-start gap-2"
      onClick={() => {
        closeMenu();
        requestAnimationFrame(() => openMode("search-tasks"));
      }}
    >
      <IconSearch className="size-4" aria-hidden />
      {t("common:conversationSearch_open")}
    </Button>
  );
}
