"use client";

import { useRef, useState, type ReactNode } from "react";
import {
  IconChevronDown,
  IconCloudDownload,
  IconGitBranch,
  IconPlus,
  IconStack2,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

export function RepositorySourceMenu({
  isMobile,
  onAdd,
}: {
  isMobile: boolean;
  onAdd: (kind: "saved_repository" | "local_repository" | "remote_repository") => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const touchPending = useRef(false);
  const itemClass = cn("cursor-pointer items-start gap-3", isMobile ? "min-h-11" : "py-2");
  return (
    <DropdownMenu open={open} onOpenChange={setOpen} modal={!isMobile}>
      <DropdownMenuTrigger
        asChild
        onPointerDown={(event) => {
          // Defer opening until the touch's synthetic click has a stable target.
          touchPending.current = event.pointerType === "touch";
          if (touchPending.current) event.preventDefault();
        }}
        onPointerCancel={() => {
          touchPending.current = false;
        }}
        onClick={() => {
          if (!touchPending.current) return;
          touchPending.current = false;
          setOpen((value) => !value);
        }}
      >
        <Button
          type="button"
          variant="outline"
          className={cn("cursor-pointer", isMobile ? "min-h-11" : "h-9 px-3")}
        >
          <IconPlus className="h-4 w-4" />
          {t("task:addRepository")}
          <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-80 max-w-[calc(100vw-2rem)]">
        <RepositorySourceMenuItem
          label={t("task:workspaceRepository")}
          description={t("task:chooseFromSavedOrDiscoveredRepositories")}
          icon={<IconStack2 className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          className={itemClass}
          onSelect={() => onAdd("saved_repository")}
        />
        <RepositorySourceMenuItem
          label={t("task:localGitRepository")}
          description={t("task:useAnExistingCheckoutOnThis")}
          icon={<IconGitBranch className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          className={itemClass}
          onSelect={() => onAdd("local_repository")}
        />
        <RepositorySourceMenuItem
          label={t("task:remoteRepository")}
          description={t("task:cloneFromAProviderOrGit")}
          icon={<IconCloudDownload className="mt-0.5 h-4 w-4 text-muted-foreground" />}
          className={itemClass}
          onSelect={() => onAdd("remote_repository")}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function RepositorySourceMenuItem({
  label,
  description,
  icon,
  className,
  onSelect,
}: {
  label: string;
  description: string;
  icon: ReactNode;
  className: string;
  onSelect: () => void;
}) {
  return (
    <DropdownMenuItem aria-label={label} className={className} onSelect={onSelect}>
      {icon}
      <span className="min-w-0">
        <span className="block text-sm font-medium text-foreground">{label}</span>
        <span className="block text-xs text-muted-foreground">{description}</span>
      </span>
    </DropdownMenuItem>
  );
}
