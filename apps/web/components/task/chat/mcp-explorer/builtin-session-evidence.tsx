import {
  builtinPackageState,
  builtinSelectionKey,
  isReservedBuiltinName,
} from "./builtin-mcp-view-model";
import { createContext, useContext } from "react";
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
        <dt>{t("settings:builtinMcp.desired")}</dt>
        <dd>{t(builtinSelectionKey(desired))}</dd>
        <dt>{t("settings:builtinMcp.bound")}</dt>
        <dd>{t(builtinSelectionKey(bound))}</dd>
      </dl>
      {pending && <p className="text-xs">{t("settings:builtinMcp.pending")}</p>}
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
