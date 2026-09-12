import {
  createContext,
  useCallback,
  useContext,
  useId,
  useLayoutEffect,
  type ReactNode,
  type RefObject,
} from "react";
import { flushSync } from "react-dom";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import type { TipTapInputHandle } from "./tiptap-input";
import type { ComposerActivity, useComposerDisclosure } from "./use-composer-disclosure";
import "./composer-disclosure.css";

export const ComposerDisclosureContext = createContext<ReturnType<
  typeof useComposerDisclosure
> | null>(null);
export const useComposerDisclosureContext = () => useContext(ComposerDisclosureContext);

export function useComposerActivity({ draft, busy, required, overlay }: ComposerActivity) {
  const report = useComposerDisclosureContext()?.reportActivity;
  const owner = useId();
  useLayoutEffect(() => {
    report?.(owner, { draft, busy, required, overlay });
    return () => report?.(owner, null);
  }, [report, owner, draft, busy, required, overlay]);
}

export function useComposerFocus(inputRef: RefObject<TipTapInputHandle | null>) {
  const disclosure = useComposerDisclosureContext();
  const reveal = disclosure?.reveal;
  const expanded = disclosure?.expanded ?? true;
  return useCallback(() => {
    // An explicit native/plugin focus must remove inert before touching TipTap.
    if (!expanded) flushSync(() => reveal?.());
    inputRef.current?.focus();
  }, [expanded, reveal, inputRef]);
}

export function ComposerDisclosureRegion({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  const disclosure = useComposerDisclosureContext();
  const animated = disclosure?.enabled;
  const collapsed = disclosure?.expanded === false;
  return (
    <div
      className={animated ? "thread-composer-disclosure" : "contents"}
      data-state={collapsed ? "closed" : "open"}
      aria-hidden={collapsed || undefined}
      inert={collapsed || undefined}
    >
      <div className={animated ? "thread-composer-clip" : "contents"}>
        <div className={cn(animated ? "thread-composer-content" : "contents", className)}>
          {children}
        </div>
      </div>
    </div>
  );
}

/** Bounds Threads actions and input together, including when auto-hide is off. */
export function ComposerFooterAllocation({ children }: { children: ReactNode }) {
  const disclosure = useComposerDisclosureContext();
  return (
    <div
      data-testid={disclosure ? "thread-footer-allocation" : undefined}
      className={disclosure ? "min-h-0 shrink-0 overflow-y-auto overscroll-contain" : "contents"}
      style={disclosure ? { maxHeight: "calc(100% - 80px)" } : undefined}
    >
      {children}
    </div>
  );
}

export function ComposerCollapseButton() {
  const { t } = useTranslation("threads");
  const disclosure = useComposerDisclosureContext();
  if (!disclosure?.enabled) return null;
  return (
    <div className="flex justify-end pt-1">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-7 cursor-pointer gap-1 px-2 text-xs text-muted-foreground"
        data-testid="collapse-composer"
        disabled={!disclosure.canCollapse}
        onClick={disclosure.collapse}
      >
        <IconChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
        {t("collapseComposer")}
      </Button>
    </div>
  );
}
