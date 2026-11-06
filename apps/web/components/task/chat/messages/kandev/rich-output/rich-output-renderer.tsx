"use client";

import { IconChartBar } from "@tabler/icons-react";
import { useMemo } from "react";
import { createStateStore } from "@json-render/core";
import { useOptionalAppStoreApi, useOptionalAppStore } from "@/components/state-provider";
import { getRichOutputViewStore, withHostElementIds } from "@/lib/rich-output/view-state";
import { RichOutputViewContext } from "./view-state";
import { useTranslation } from "react-i18next";
import { KandevRow } from "../shared";
import type { KandevRenderer } from "../types";
import { ActionProvider, Renderer, StateProvider, VisibilityProvider } from "@json-render/react";
import { normalizeRichOutput } from "@/lib/rich-output/normalize";
import { RichOutputHostContext, registry } from "./registry";
import { RichOutputErrorDisclosure } from "./rich-output-error-disclosure";
import { getRichOutputErrorText } from "./rich-output-error";

export const RichOutputRenderer: KandevRenderer = ({
  args,
  result,
  status,
  sessionId,
  messageId,
  onOpenFile,
}) => {
  const { t } = useTranslation();
  const owner = useOptionalAppStoreApi();
  const authIdentity = useOptionalAppStore(
    (state) => JSON.stringify([state.auth.mode, state.auth.authenticated, state.auth.user?.id]),
    undefined,
  );
  const viewStore = useMemo(
    () =>
      owner && messageId
        ? getRichOutputViewStore(owner, JSON.stringify([authIdentity, sessionId, messageId]))
        : createStateStore(),
    [owner, authIdentity, sessionId, messageId],
  );
  const viewIdentity = JSON.stringify([authIdentity, sessionId, messageId]);
  const host = useMemo(
    () => ({ sessionId, onOpenFile, viewIdentity }),
    [sessionId, onOpenFile, viewIdentity],
  );
  const output = useMemo(() => normalizeRichOutput(args, result), [args, result]);

  const spec = useMemo(() => output && withHostElementIds(output.spec), [output]);

  if (status !== "complete") {
    const errorText =
      status === "error"
        ? getRichOutputErrorText(result, t("task:richOutputErrorUnavailable"))
        : undefined;
    return (
      <KandevRow
        Icon={IconChartBar}
        title={t("task:richOutputTool")}
        status={status}
        hasExpandableContent={false}
        statusControl={errorText ? <RichOutputErrorDisclosure diagnostic={errorText} /> : undefined}
      />
    );
  }

  return (
    <section
      className="my-2 max-w-full overflow-hidden rounded-xl border border-border/60 bg-background/80 px-4 py-3.5"
      data-testid="rich-output"
      aria-label={output?.title || t("task:richOutputTool")}
    >
      {output ? (
        <>
          <header className="space-y-1">
            <h3 className="text-sm font-medium tracking-tight text-foreground">{output.title}</h3>
            {output.description && (
              <p className="text-xs leading-relaxed text-muted-foreground">{output.description}</p>
            )}
          </header>
          <div className="mt-4 min-w-0">
            <RichOutputHostContext.Provider value={host}>
              <RichOutputViewContext.Provider value={viewStore}>
                <StateProvider store={viewStore}>
                  <VisibilityProvider>
                    <ActionProvider>
                      <Renderer spec={spec!} registry={registry} />
                    </ActionProvider>
                  </VisibilityProvider>
                </StateProvider>
              </RichOutputViewContext.Provider>
            </RichOutputHostContext.Provider>
          </div>
        </>
      ) : (
        <p className="text-xs text-muted-foreground">{t("task:richOutputUnavailable")}</p>
      )}
    </section>
  );
};
