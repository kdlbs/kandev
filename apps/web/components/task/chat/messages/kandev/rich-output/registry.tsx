import { createContext, useContext, type ReactNode } from "react";
import { defineRegistry, type Components } from "@json-render/react";
import { catalog } from "@/lib/rich-output/catalog";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { Chart } from "./chart-block";
import { Callout } from "./callout-block";
import { StatusList } from "./status-list-block";
import { EvidenceTable } from "./table-block";
import { Metrics } from "./metrics-block";
import { FilePreview } from "./file-preview-block";
import { withElementViewState } from "./view-state";
import { RichOutputContentBoundary } from "./error-boundary";

export const RichOutputHostContext = createContext<{
  sessionId?: string;
  viewIdentity?: string;
  onOpenFile?: (path: string, repo?: string) => void;
}>({});

function Stack({
  direction,
  children,
}: {
  direction: "vertical" | "horizontal";
  children?: ReactNode;
}) {
  const { isMobile } = useResponsiveBreakpoint();
  const horizontal = direction === "horizontal" && !isMobile;
  return (
    <div
      data-testid="rich-output-stack"
      className={
        horizontal
          ? "flex min-w-0 flex-row gap-4 [&>*]:min-w-0 [&>*]:flex-1"
          : "flex min-w-0 flex-col gap-4"
      }
    >
      {children}
    </div>
  );
}

const components: Components<typeof catalog> = {
  Stack: ({ props, children }) => <Stack {...props}>{children}</Stack>,
  Group: ({ props, children }) => (
    <section className="min-w-0 space-y-3" data-testid="rich-output-group">
      {props.title && <h4 className="text-xs font-medium text-foreground">{props.title}</h4>}
      {children}
    </section>
  ),
  Text: ({ props }) => (
    <p className="min-w-0 break-words text-xs leading-relaxed text-muted-foreground">
      {props.text}
    </p>
  ),
  Chart: ({ props }) =>
    "csv" in props ? null : (
      <RichOutputContentBoundary resetKey={JSON.stringify(props)}>
        <Chart {...props} />
      </RichOutputContentBoundary>
    ),
  Table: ({ props }) => ("csv" in props ? null : <EvidenceTable {...props} />),
  Callout: ({ props }) => <Callout {...props} />,
  StatusList: ({ props }) => <StatusList {...props} />,
  Metrics: ({ props }) => <Metrics {...props} />,
  FilePreview: ({ props }) => {
    const host = useContext(RichOutputHostContext);
    return (
      <RichOutputContentBoundary resetKey={JSON.stringify([host.sessionId, props])}>
        <FilePreview
          key={JSON.stringify([host.viewIdentity, host.sessionId, props.repo, props.path])}
          {...props}
          {...host}
        />
      </RichOutputContentBoundary>
    );
  },
};
const defined = defineRegistry(catalog, { components });
export const registry = {
  ...defined.registry,
  Chart: withElementViewState(defined.registry.Chart),
  FilePreview: withElementViewState(defined.registry.FilePreview),
};
