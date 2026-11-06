"use client";

import { IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { workspaceHomeHref } from "@/lib/navigation/workspace-home";
import { useSettingsReturnHref } from "./settings-navigation-provider";

export function SettingsCloseButton() {
  const { t } = useTranslation();
  const router = useRouter();
  const returnHref = useSettingsReturnHref();
  const workspace = useAppStore((s) =>
    s.workspaces.items.find((item) => item.id === s.workspaces.activeId),
  );
  const startupPage = useAppStore((s) => s.userSettings.startupPage);

  return (
    <Button
      variant="ghost"
      size="icon"
      className="size-11 shrink-0 cursor-pointer md:hidden"
      aria-label={t("sidebar:closeSettings")}
      data-testid="settings-close-button"
      onClick={() => router.push(returnHref ?? workspaceHomeHref(workspace, startupPage))}
    >
      <IconX className="size-4" />
    </Button>
  );
}
