"use client";

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
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
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useMCPAssignmentTasks } from "@/hooks/domains/workspace/use-mcp-assignment-tasks";
import {
  useMCPAssignmentTargets,
  type MCPAssignmentScope,
} from "@/hooks/domains/workspace/use-mcp-assignment-targets";
import { useRouter } from "@/lib/routing/client-router";
import type { MCPAssignmentTarget } from "@/lib/mcp/assignment-targets";
import type { MCPRegistrationReceipt } from "@/hooks/domains/workspace/use-mcp-registration-completion";

const ACTION = controlSizingClassName("standard", "cursor-pointer");
const SCOPE_KEYS = {
  repository: "settings:mcpAssignmentRepository",
  profile: "settings:mcpAssignmentProfile",
  task_session: "settings:mcpAssignmentSession",
} as const;
const REVIEW_KEYS = {
  repository: "settings:mcpAssignmentRepositoryScope",
  profile: "settings:mcpAssignmentProfileScope",
  task_session: "settings:mcpAssignmentSessionScope",
} as const;

function AssignmentShell({
  name,
  children,
  footer,
  onClose,
}: {
  name: string;
  children: ReactNode;
  footer: ReactNode;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [trigger] = useState(() => document.activeElement as HTMLElement | null);
  const restoreFocus = (event: Event) => {
    event.preventDefault();
    trigger?.focus();
  };
  const body = (
    <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4">{children}</div>
  );
  const actions = (
    <div className="flex shrink-0 flex-wrap justify-between gap-2 border-t p-4">{footer}</div>
  );
  if (isMobile)
    return (
      <Drawer open onOpenChange={(open) => !open && onClose()}>
        <DrawerContent
          data-testid="mcp-assignment-chooser"
          onCloseAutoFocus={restoreFocus}
          className="h-[min(85dvh,42rem)] overflow-hidden pb-[max(0.5rem,env(safe-area-inset-bottom))]"
        >
          <DrawerHeader className="shrink-0 border-b text-left">
            <DrawerTitle className="wrap-anywhere">
              {t("settings:mcpAssignmentTitle", { name })}
            </DrawerTitle>
            <DrawerDescription>{t("settings:mcpAssignmentDescription")}</DrawerDescription>
          </DrawerHeader>
          {body}
          {actions}
        </DrawerContent>
      </Drawer>
    );
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        data-testid="mcp-assignment-chooser"
        onCloseAutoFocus={restoreFocus}
        className="flex max-h-[80dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-lg"
      >
        <DialogHeader className="shrink-0 border-b p-4 pr-12">
          <DialogTitle className="wrap-anywhere">
            {t("settings:mcpAssignmentTitle", { name })}
          </DialogTitle>
          <DialogDescription>{t("settings:mcpAssignmentDescription")}</DialogDescription>
        </DialogHeader>
        {body}
        {actions}
      </DialogContent>
    </Dialog>
  );
}

function OwnerResults({
  items,
  loading,
  failed,
  reload,
  onChoose,
}: {
  items: { id: string; label: string; detail?: string }[];
  loading: boolean;
  failed: boolean;
  reload: () => void;
  onChoose: (id: string) => void;
}) {
  const { t } = useTranslation();
  if (loading)
    return (
      <p role="status" className="text-sm text-muted-foreground">
        {t("common:loading")}
      </p>
    );
  if (failed)
    return (
      <div className="space-y-3">
        <p role="alert" className="text-sm">
          {t("settings:mcpAssignmentReadFailed")}
        </p>
        <Button variant="outline" className={ACTION} onClick={reload}>
          {t("settings:retryDestinations")}
        </Button>
      </div>
    );
  if (!items.length)
    return <p className="text-sm text-muted-foreground">{t("settings:mcpAssignmentEmpty")}</p>;
  return (
    <div className="space-y-2">
      {items.map((item) => (
        <Button
          key={item.id}
          variant="outline"
          aria-label={item.label}
          aria-describedby={
            item.detail ? `mcp-owner-detail-${encodeURIComponent(item.id)}` : undefined
          }
          className={`${ACTION} h-auto w-full justify-start whitespace-normal py-2 text-left`}
          onClick={() => onChoose(item.id)}
        >
          <span className="min-w-0">
            <span className="block wrap-anywhere">{item.label}</span>
            {item.detail && (
              <span
                id={`mcp-owner-detail-${encodeURIComponent(item.id)}`}
                className="block wrap-anywhere text-xs font-normal text-muted-foreground"
              >
                {item.detail}
              </span>
            )}
          </span>
        </Button>
      ))}
    </div>
  );
}

function AssignmentReview({ target, status }: { target: MCPAssignmentTarget; status: string }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <div className="rounded-lg border bg-muted/30 p-3">
        <p className="text-xs text-muted-foreground">{t(SCOPE_KEYS[target.scope])}</p>
        <p className="wrap-anywhere font-medium">{target.label}</p>
        {target.detail && (
          <p className="wrap-anywhere text-sm text-muted-foreground">{target.detail}</p>
        )}
      </div>
      <p className="text-sm">{t(REVIEW_KEYS[target.scope])}</p>
      {status === "failed" && (
        <p role="alert" className="text-sm text-destructive">
          {t("settings:mcpAssignmentReadFailed")}
        </p>
      )}
      {status === "unavailable" && (
        <p role="alert" className="text-sm text-destructive">
          {t("settings:mcpAssignmentUnavailable")}
        </p>
      )}
    </div>
  );
}

function TaskResults({
  tasks,
  page,
  setPage,
  onChoose,
}: {
  tasks: ReturnType<typeof useMCPAssignmentTasks>;
  page: number;
  setPage: (page: number) => void;
  onChoose: (task: { id: string; title: string }) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <OwnerResults
        items={tasks.tasks.map((item) => ({ id: item.id, label: item.title }))}
        loading={tasks.loading}
        failed={tasks.failed}
        reload={tasks.reload}
        onChoose={(id) => {
          const chosen = tasks.tasks.find((item) => item.id === id);
          if (chosen) onChoose(chosen);
        }}
      />
      <div className="flex justify-between gap-2">
        <Button
          variant="outline"
          className={ACTION}
          disabled={page <= 1 || tasks.loading}
          onClick={() => setPage(page - 1)}
        >
          {t("settings:mcpAssignmentPrevious")}
        </Button>
        <Button
          variant="outline"
          className={ACTION}
          disabled={page * 25 >= tasks.total || tasks.loading}
          onClick={() => setPage(page + 1)}
        >
          {t("common:next")}
        </Button>
      </div>
    </>
  );
}

function ScopeChoices({ onSelect }: { onSelect: (scope: MCPAssignmentScope) => void }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      {(Object.keys(SCOPE_KEYS) as MCPAssignmentScope[]).map((value) => (
        <Button
          key={value}
          variant="outline"
          className={`${ACTION} w-full justify-start`}
          onClick={() => onSelect(value)}
        >
          {t(SCOPE_KEYS[value])}
        </Button>
      ))}
    </div>
  );
}

export function MCPAssignmentChooser({
  receipt,
  onClose,
}: {
  receipt: MCPRegistrationReceipt;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const router = useRouter();
  const [scope, setScope] = useState<MCPAssignmentScope | null>(null);
  const [task, setTask] = useState<{ id: string; title: string } | null>(null);
  const [target, setTarget] = useState<MCPAssignmentTarget | null>(null);
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const choosingTask = scope === "task_session" && !task;
  const tasks = useMCPAssignmentTasks(receipt.workspaceId, { active: choosingTask, page, query });
  const owners = useMCPAssignmentTargets(receipt.workspaceId, scope, task?.id);
  const busy = owners.openStatus === "checking";
  const back = () => {
    setQuery("");
    setPage(1);
    if (target) setTarget(null);
    else if (task) setTask(null);
    else setScope(null);
  };
  const open = async () => {
    if (!target) return;
    const href = await owners.openHref(target, receipt.definitionId);
    if (href) router.push(href);
  };
  const footer = (
    <>
      <Button variant="outline" className={ACTION} disabled={busy} onClick={scope ? back : onClose}>
        {t(scope ? "common:back" : "common:close")}
      </Button>
      {target && (
        <Button className={`${ACTION} max-md:min-h-12`} disabled={busy} onClick={() => void open()}>
          {t("settings:mcpAssignmentOpen")}
        </Button>
      )}
    </>
  );
  const normalized = query.trim().toLowerCase();
  const visible = owners.targets.filter((item) =>
    `${item.label} ${item.detail ?? ""}`.toLowerCase().includes(normalized),
  );
  return (
    <AssignmentShell name={receipt.displayName} footer={footer} onClose={onClose}>
      <div className="space-y-4">
        {busy && (
          <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
            {t("common:loading")}
          </p>
        )}
        {!scope && <ScopeChoices onSelect={setScope} />}
        {target ? (
          <AssignmentReview target={target} status={owners.openStatus} />
        ) : (
          scope && (
            <>
              <p className="wrap-anywhere text-sm font-medium">
                {task?.title ??
                  t(choosingTask ? "settings:mcpAssignmentChooseTask" : SCOPE_KEYS[scope])}
              </p>
              <Input
                value={query}
                onChange={(event) => {
                  setQuery(event.target.value);
                  setPage(1);
                }}
                aria-label={t("settings:mcpAssignmentSearch")}
                placeholder={t("settings:mcpAssignmentSearch")}
              />
              {choosingTask ? (
                <TaskResults
                  tasks={tasks}
                  page={page}
                  setPage={setPage}
                  onChoose={(chosen) => {
                    setTask(chosen);
                    setQuery("");
                  }}
                />
              ) : (
                <OwnerResults
                  items={visible.map((item) => ({ ...item, id: item.ownerId }))}
                  loading={owners.loading}
                  failed={owners.failed}
                  reload={owners.reload}
                  onChoose={(id) => setTarget(visible.find((item) => item.ownerId === id) ?? null)}
                />
              )}
            </>
          )
        )}
      </div>
    </AssignmentShell>
  );
}
