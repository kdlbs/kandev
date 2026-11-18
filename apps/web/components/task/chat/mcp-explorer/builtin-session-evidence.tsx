import {
  builtinApplicationStatus,
  builtinPackageState,
  builtinPendingPackages,
  builtinSelectionKey,
  isReservedBuiltinName,
} from "./builtin-mcp-view-model";
import { createContext, useContext, useRef, useState } from "react";
import { IconAlertCircle, IconCheck, IconLoader2, IconClock } from "@tabler/icons-react";
import { retryBuiltinMCPApplication } from "@/lib/api/domains/builtin-mcp-api";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import type { BuiltinMCPPackage, BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
import type { MCPAttachmentServer } from "@/lib/state/slices/session-runtime/types";
import { isKandevMcpServer } from "./mcp-explorer-view-model";

type EvidenceContext = {
  view?: BuiltinMCPSessionView;
  manage: (id: BuiltinMCPPackage) => void;
  refresh: () => void;
};
const Context = createContext<EvidenceContext | null>(null);
export const BuiltinSessionEvidenceProvider = Context.Provider;

export function BuiltinPendingPackages() {
  const { t } = useTranslation();
  const context = useContext(Context);
  const changes = builtinPendingPackages(context?.view);
  if (!context || changes.length === 0) return null;
  const changeLabels = {
    selected: "settings:builtinMcp.selected",
    add: "settings:mcpPendingAdd",
    remove: "settings:mcpPendingRemove",
  };
  return (
    <section className="mb-4 space-y-2" data-testid="builtin-mcp-pending-changes">
      <h3 className="px-2 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
        {t(
          context?.view?.application
            ? "settings:builtinMcp.savedSelection"
            : "settings:builtinMcp.desired",
        )}
      </h3>
      {!context.view?.application && (
        <p className="px-2 text-xs text-muted-foreground">
          {t(
            changes.some(({ action }) => action === "selected")
              ? "settings:builtinMcp.currentUnknown"
              : "settings:builtinMcp.pending",
          )}
        </p>
      )}
      {changes.map(({ id, action }) => (
        <Button
          key={id}
          type="button"
          variant="outline"
          data-testid={`builtin-mcp-pending-${id}`}
          className={controlSizingClassName(
            "standard",
            "h-auto min-h-7 w-full justify-between gap-2 whitespace-normal px-2 py-2 text-left max-md:h-auto [@media(pointer:coarse)]:h-auto",
          )}
          onClick={() => context.manage(id)}
        >
          <span className="min-w-0 break-words">
            {t(`settings:builtinMcp.packages.${id}.label`)}
          </span>
          <span className="shrink-0 text-xs text-muted-foreground">{t(changeLabels[action])}</span>
        </Button>
      ))}
    </section>
  );
}

export function BuiltinSessionEvidence({ server }: { server: MCPAttachmentServer }) {
  const { t } = useTranslation();
  const context = useContext(Context);
  if (!isKandevMcpServer(server)) {
    if (server.source === "profile" && isReservedBuiltinName(server.name))
      return <p className="text-sm text-destructive">{t("settings:builtinMcp.collision")}</p>;
    return null;
  }
  const id = server.builtin_package;
  const view = context?.view;
  if (!id)
    return (
      <p className="text-sm text-muted-foreground">
        {t(
          view?.binding_state === "legacy"
            ? "settings:builtinMcp.legacy"
            : "settings:builtinMcp.bindingUnknown",
        )}
      </p>
    );
  return <AcceptedPackageEvidence id={id} context={context} />;
}

function AcceptedPackageEvidence({
  id,
  context,
}: {
  id: BuiltinMCPPackage;
  context: EvidenceContext | null;
}) {
  const { t } = useTranslation();
  const { desired, bound, pending } = builtinPackageState(context?.view, id);
  return (
    <div className="space-y-2 rounded-md bg-muted/40 p-3" data-testid="builtin-mcp-evidence">
      <p className="font-medium">
        {t("settings:builtinMcp.title")}: {t(`settings:builtinMcp.packages.${id}.label`)}
      </p>
      <p className="text-xs text-muted-foreground">{t("settings:builtinMcp.owner")}</p>
      <dl className="grid grid-cols-2 gap-2 text-xs">
        <dt>
          {t(
            context?.view?.application
              ? "settings:builtinMcp.savedSelection"
              : "settings:builtinMcp.desired",
          )}
        </dt>
        <dd>{t(builtinSelectionKey(desired))}</dd>
        <dt>{t("settings:builtinMcp.bound")}</dt>
        <dd>
          {t(
            context?.view?.provider_available === false
              ? "settings:builtinMcp.applicationToolsUnavailable"
              : builtinSelectionKey(bound),
          )}
        </dd>
      </dl>
      {pending && !context?.view?.application && (
        <p className="text-xs">{t("settings:builtinMcp.pending")}</p>
      )}
      <p className="text-xs text-muted-foreground">{t("settings:builtinMcp.evidenceOnly")}</p>
      {context && (
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            className={controlSizingClassName("standard")}
            onClick={() => context.manage(id)}
          >
            {t("settings:builtinMcp.manage")}
          </Button>
          <Button
            variant="ghost"
            className={controlSizingClassName("standard")}
            onClick={context.refresh}
          >
            {t("settings:builtinMcp.refresh")}
          </Button>
        </div>
      )}
    </div>
  );
}

export function BuiltinApplicationFeedback() {
  const { t } = useTranslation();
  const context = useContext(Context);
  const view = context?.view;
  const application = view?.application;
  const status = builtinApplicationStatus(view);
  const action = useApplicationRetry(context);
  if (!context || !view || !application || status.state === "unknown") return null;
  const { pending, failed, retry } = action;
  const spinning = pending || status.state === "applying";
  const Icon = applicationIcon(status.state, spinning);
  const messageKey = applicationMessage(status.messageKey, pending, failed);
  return (
    <section
      className="shrink-0 space-y-3 border-b border-border/70 pb-3"
      data-testid="builtin-mcp-application"
      data-state={status.state}
    >
      <div className="flex items-start gap-2" role="status" aria-live="polite" aria-atomic="true">
        <Icon
          aria-hidden
          className={`mt-0.5 size-4 shrink-0 ${spinning ? "motion-safe:animate-spin" : ""}`}
        />
        <p className="min-w-0 text-xs leading-5">{t(messageKey)}</p>
      </div>
      {status.retry && (
        <Button
          type="button"
          variant="outline"
          disabled={pending}
          onClick={() => void retry()}
          className={controlSizingClassName("standard", "w-full md:w-auto")}
          data-testid="builtin-mcp-application-retry"
        >
          {t("settings:builtinMcp.applicationRetry")}
        </Button>
      )}
    </section>
  );
}

function applicationIcon(state: string, spinning: boolean) {
  if (spinning) return IconLoader2;
  if (state === "failed") return IconAlertCircle;
  if (state === "applied") return IconCheck;
  return IconClock;
}
function applicationMessage(key: string, pending: boolean, failed: boolean) {
  if (failed) return "settings:builtinMcp.applicationRetryFailed";
  if (pending) return "settings:builtinMcp.applicationRetrying";
  return key;
}
function useApplicationRetry(context: EvidenceContext | null) {
  const view = context?.view;
  const application = view?.application;
  const owner = JSON.stringify([
    view?.workspace_id,
    view?.task_id,
    view?.session_id,
    view?.execution_id,
    application?.request_id,
  ]);
  const currentOwner = useRef(owner);
  currentOwner.current = owner;
  const [action, setAction] = useState({ owner: "", pending: false, error: false });
  const pending = action.owner === owner && action.pending;
  const failed = action.owner === owner && action.error;
  const retry = async () => {
    if (!context || !view || !application) return;
    setAction({ owner, pending: true, error: false });
    try {
      await retryBuiltinMCPApplication(
        view.session_id,
        application.request_id,
        application.row_version,
      );
      if (currentOwner.current === owner) context.refresh();
    } catch {
      if (currentOwner.current === owner) {
        setAction({ owner, pending: false, error: true });
        context.refresh();
      }
      return;
    }
    if (currentOwner.current === owner) setAction({ owner, pending: false, error: false });
  };
  return { pending, failed, retry };
}
