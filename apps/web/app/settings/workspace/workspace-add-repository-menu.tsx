"use client";

import { type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconCloudDownload, IconGitBranch, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { settingsActionClassName } from "@/components/settings/settings-control";

export type AddRepositorySourceKind = "local" | "remote";

/**
 * The Repositories page's "Add repository" control. It only chooses which flow
 * opens: a discovered or manually validated local checkout, or a remote
 * repository from a connected provider.
 */
export function AddRepositoryMenu({ onAdd }: { onAdd: (kind: AddRepositorySourceKind) => void }) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button className={settingsActionClassName("cursor-pointer")}>
          <IconPlus className="h-4 w-4" />
          {t("workspaces:addRepository")}
          <IconChevronDown className="h-3.5 w-3.5 opacity-70" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-80 max-w-[calc(100vw-2rem)]">
        <AddRepositoryMenuItem
          label={t("workspaces:localRepository")}
          description={t("workspaces:localRepositoryMenuDescription")}
          icon={<IconGitBranch className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          onSelect={() => onAdd("local")}
        />
        <AddRepositoryMenuItem
          label={t("workspaces:remoteRepository")}
          description={t("workspaces:remoteRepositoryMenuDescription")}
          icon={<IconCloudDownload className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          onSelect={() => onAdd("remote")}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function AddRepositoryMenuItem({
  label,
  description,
  icon,
  onSelect,
}: {
  label: string;
  description: string;
  icon: ReactNode;
  onSelect: () => void;
}) {
  return (
    <DropdownMenuItem
      aria-label={label}
      className="cursor-pointer items-start gap-3 py-2 [@media(pointer:coarse)]:min-h-11"
      onSelect={onSelect}
    >
      {icon}
      <span className="min-w-0">
        <span className="block text-sm font-medium text-foreground">{label}</span>
        <span className="block text-xs text-muted-foreground">{description}</span>
      </span>
    </DropdownMenuItem>
  );
}
