"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconArrowLeft, IconChevronRight, IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerDescription,
} from "@kandev/ui/drawer";
import Link from "@/components/routing/app-link";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useMCPUsage } from "@/hooks/domains/workspace/use-mcp-usage";
import { mcpUsageOwnerHref } from "@/lib/mcp/management-links";
import type { MCPOwnerUsage, MCPSelectionScope } from "@/lib/types/http-mcp";

const SCOPE_KEYS = {
  profile: "settings:mcpUsageProfiles",
  repository: "settings:mcpUsageRepositories",
  task: "settings:mcpUsageTasks",
  task_session: "settings:mcpUsageSessions",
} as const;
const SCOPES: MCPSelectionScope[] = ["profile", "repository", "task", "task_session"];
const ACTION_CLASS = controlSizingClassName("standard", "cursor-pointer");

function UsageOwner({
  owner,
  workspaceId,
  stale,
}: {
  owner: MCPOwnerUsage;
  workspaceId: string;
  stale: boolean;
}) {
  const { t } = useTranslation();
  const href = stale ? null : mcpUsageOwnerHref(owner, workspaceId);
  const content = (
    <>
      <span className="min-w-0 flex-1 wrap-anywhere">
        {owner.display_name || t("settings:mcpUnnamedOwner")}
        {owner.owner_state !== "available" && (
          <span className="ml-2 text-xs text-muted-foreground">
            {t(owner.owner_state === "disabled" ? "settings:mcpDisabled" : "tasks:archived")}
          </span>
        )}
      </span>
      {href ? (
        <IconChevronRight className="size-4 shrink-0" aria-hidden />
      ) : (
        <span className="text-xs text-muted-foreground">{t("common:unavailable")}</span>
      )}
    </>
  );
  const className = controlSizingClassName(
    "standard",
    "flex h-auto w-full min-w-0 items-center gap-3 rounded-md px-3 py-2 text-sm",
  );
  return href ? (
    <Link
      href={href}
      className={`${className} cursor-pointer hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring`}
    >
      {content}
    </Link>
  ) : (
    <div className={className}>{content}</div>
  );
}

function UsageBody({
  workspaceId,
  usage,
  invalid,
}: {
  workspaceId: string;
  usage: ReturnType<typeof useMCPUsage>;
  invalid: boolean;
}) {
  const { t } = useTranslation();
  const unavailable = invalid || usage.unavailable;
  return (
    <div
      className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain p-4"
      data-testid="mcp-usage-body"
    >
      {unavailable ? (
        <p role="alert">{t("settings:mcpOwnerUnavailable")}</p>
      ) : (
        <>
          {usage.loading && <p role="status">{t("settings:mcpLoading")}</p>}
          {Boolean(usage.error) && (
            <p role="alert" className="text-sm text-muted-foreground">
              {t(usage.items.length ? "settings:mcpUsageStale" : "settings:mcpLoadFailed")}
            </p>
          )}
          {!usage.loading && !usage.error && !usage.hasMore && usage.items.length === 0 && (
            <p className="text-sm text-muted-foreground">{t("settings:mcpUsageEmpty")}</p>
          )}
          {SCOPES.map((scope) => {
            const owners = usage.items.filter((owner) => owner.scope === scope);
            return (
              owners.length > 0 && (
                <section key={scope} className="space-y-1">
                  <h3 className="px-3 text-xs font-medium text-muted-foreground">
                    {t(SCOPE_KEYS[scope])}
                  </h3>
                  {owners.map((owner) => (
                    <UsageOwner
                      key={`${owner.scope}:${owner.owner_id}`}
                      owner={owner}
                      workspaceId={workspaceId}
                      stale={usage.stale}
                    />
                  ))}
                </section>
              )
            );
          })}
          {Boolean(usage.pageError) && (
            <p role="alert" className="text-sm text-muted-foreground">
              {t("settings:mcpLoadFailed")}
            </p>
          )}
          {usage.hasMore && (
            <Button
              variant="outline"
              className={`${ACTION_CLASS} w-full`}
              disabled={usage.busy || usage.stale}
              onClick={() => void usage.loadMore()}
            >
              {t(usage.pageError ? "agents:retry" : "settings:mcpUsageLoadMore")}
            </Button>
          )}
          <Button
            variant="ghost"
            className={ACTION_CLASS}
            disabled={usage.busy}
            onClick={() => void usage.reload()}
          >
            <IconRefresh className="size-4" />
            {t(usage.error ? "agents:retry" : "agents:refresh")}
          </Button>
        </>
      )}
    </div>
  );
}

export function MCPUsage({
  workspaceId,
  definitionId,
  name,
  invalid = false,
  onClose,
}: {
  workspaceId: string;
  definitionId: string | null;
  name?: string;
  invalid?: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [returnTarget] = useState(() => document.activeElement);
  const restoreFocus = (event: Event) => {
    if (
      returnTarget instanceof HTMLElement &&
      returnTarget.isConnected &&
      returnTarget !== document.body
    ) {
      event.preventDefault();
      returnTarget.focus({ preventScroll: true });
    }
  };
  const { isMobile } = useResponsiveBreakpoint();
  const usage = useMCPUsage(workspaceId, invalid ? null : definitionId);
  const title = name ? t("settings:mcpUsedInTitle", { name }) : t("settings:mcpUsedIn");
  const body = <UsageBody workspaceId={workspaceId} usage={usage} invalid={invalid} />;
  if (isMobile)
    return (
      <Drawer open onOpenChange={(open) => !open && onClose()}>
        <DrawerContent
          onCloseAutoFocus={restoreFocus}
          data-testid="mcp-usage"
          className="h-[100dvh] overflow-hidden data-[vaul-drawer-direction=bottom]:mt-0 data-[vaul-drawer-direction=bottom]:max-h-[100dvh] pb-[max(0.5rem,env(safe-area-inset-bottom))]"
        >
          <DrawerHeader className="shrink-0 border-b text-left">
            <Button variant="ghost" className={`${ACTION_CLASS} w-fit`} onClick={onClose}>
              <IconArrowLeft className="size-4" />
              {t("common:back")}
            </Button>
            <DrawerTitle>{title}</DrawerTitle>
            <DrawerDescription>{t("settings:mcpUsageDescription")}</DrawerDescription>
          </DrawerHeader>
          {body}
        </DrawerContent>
      </Drawer>
    );
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        onCloseAutoFocus={restoreFocus}
        data-testid="mcp-usage"
        className="flex max-h-[80dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-xl"
      >
        <DialogHeader className="shrink-0 border-b p-4 pr-12">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{t("settings:mcpUsageDescription")}</DialogDescription>
        </DialogHeader>
        {body}
      </DialogContent>
    </Dialog>
  );
}
