import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from "@tanstack/react-table";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import Link from "@/components/routing/app-link";
import { useRouter } from "@/lib/routing/client-router";
import { controlHref } from "@/lib/navigation/integrated-control";
import type { NativeDashboard } from "@/lib/types/control-native";
import type { ControlWorkspace } from "@/hooks/domains/coordinator/use-integrated-control-data";
import { useNativeDashboards } from "@/hooks/domains/coordinator/use-control-native";
import type {
  ControlRead,
  ControlReadStatus,
} from "@/hooks/domains/coordinator/use-control-resource";
import {
  dashboardStageCounts,
  TASK_STAGES,
  workflowStepCounts,
  type TaskStage,
} from "./control-stages";
import { numberCellClass } from "./control-table";
import { ControlWaitReasons } from "./control-wait-reasons-panel";

type NativeRow = { workspace: ControlWorkspace } & ControlRead<NativeDashboard>;
const UNKNOWN_KEY = "common:unknown";
type CompareKind = "tasks" | "sessions";
const STAGE_LABELS: Record<TaskStage, string> = {
  work: "sidebar:controlSummary.stage.work",
  review: "sidebar:controlSummary.stage.review",
  held: "sidebar:controlSummary.stage.held",
  completed: "sidebar:controlSummary.stage.completed",
  unknown: UNKNOWN_KEY,
};
const READ_LABELS: Record<ControlReadStatus, string> = {
  loading: "common:loading",
  ready: UNKNOWN_KEY,
  denied: "sidebar:integratedControl.readDenied",
  unsupported: "sidebar:controlRestore.contractUnavailable",
  error: "sidebar:integratedControl.loadError",
};
const TASK_FIGURES = [
  { field: "waiting_input_tasks", label: "sidebar:controlSummary.compare.waitingInput" },
  { field: "completed_tasks_24h", label: "sidebar:controlSummary.compare.completed" },
  { field: "transitions_24h", label: "sidebar:controlSummary.compare.moves" },
] as const;

function StageFigure({ data, stage }: { data?: NativeDashboard; stage: TaskStage }) {
  const { t } = useTranslation();
  const unknown = t(UNKNOWN_KEY);
  const value = data ? dashboardStageCounts(data)[stage] : unknown;
  const steps = workflowStepCounts(data, stage);
  if (steps.length === 0) return <span>{value}</span>;
  return (
    <details onClick={(event) => event.stopPropagation()}>
      <summary
        className="inline-flex min-h-7 cursor-pointer items-center underline decoration-dotted underline-offset-4 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11 [&::-webkit-details-marker]:hidden"
        aria-label={t("sidebar:controlSummary.compare.stageDetails", {
          stage: t(STAGE_LABELS[stage]),
          count: value,
        })}
      >
        {value}
      </summary>
      <ul className="mt-2 space-y-1 whitespace-normal text-xs font-normal text-muted-foreground">
        {steps.map((step) => (
          <li key={step.id}>
            {step.name || unknown}: {step.count}
          </li>
        ))}
      </ul>
    </details>
  );
}

function ReadState({ row }: { row: NativeRow }) {
  const { t } = useTranslation();
  if (row.status === "ready" && !row.stale) return null;
  return (
    <p
      role={row.status === "denied" || row.status === "error" ? "alert" : "status"}
      className="px-4 pb-2 text-xs text-muted-foreground"
    >
      {row.workspace.name}:{" "}
      {row.stale && row.receivedAt
        ? t("sidebar:controlRefresh.retainedAt", { time: row.receivedAt })
        : t(READ_LABELS[row.status])}
    </p>
  );
}

function useNativeColumns(kind: CompareKind) {
  const { t } = useTranslation();
  return useMemo<ColumnDef<NativeRow>[]>(() => {
    const workspace: ColumnDef<NativeRow> = {
      id: "workspace",
      size: 176,
      header: t("sidebar:integratedControl.aggregate.workspace"),
      cell: ({ row }) => (
        <Link
          href={controlHref("workspaces", row.original.workspace.id)}
          onClick={(event) => event.stopPropagation()}
          className="inline-flex min-h-7 cursor-pointer items-center max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        >
          {row.original.workspace.name}
        </Link>
      ),
    };
    if (kind === "tasks")
      return [
        workspace,
        ...TASK_STAGES.map(
          (stage): ColumnDef<NativeRow> => ({
            id: stage,
            size: 72,
            header: t(STAGE_LABELS[stage]),
            accessorFn: (row) => (row.data ? dashboardStageCounts(row.data)[stage] : undefined),
            cell: ({ row }) => <StageFigure data={row.original.data} stage={stage} />,
          }),
        ),
        ...TASK_FIGURES.map(
          (figure): ColumnDef<NativeRow> => ({
            id: figure.field,
            size: 96,
            header: t(figure.label),
            accessorFn: (row) => row.data?.[figure.field],
            cell: ({ getValue }) => getValue<number | undefined>() ?? t(UNKNOWN_KEY),
          }),
        ),
      ];
    return [
      workspace,
      ...(["RUNNING", "FAILED"] as const).map(
        (state): ColumnDef<NativeRow> => ({
          id: state,
          size: 112,
          header: t(
            state === "RUNNING"
              ? "sidebar:controlSummary.compare.running"
              : "sidebar:controlSummary.compare.failed",
          ),
          accessorFn: (row) => (row.data ? (row.data.sessions[state] ?? 0) : undefined),
          cell: ({ getValue }) => getValue<number | undefined>() ?? t(UNKNOWN_KEY),
        }),
      ),
      {
        id: "queued_messages",
        size: 112,
        header: t("sidebar:controlSummary.compare.queued"),
        accessorFn: (row) => row.data?.queued_messages,
        cell: ({ getValue }) => getValue<number | undefined>() ?? t(UNKNOWN_KEY),
      },
    ];
  }, [kind, t]);
}

function NativeComparisonTable({ rows, kind }: { rows: NativeRow[]; kind: CompareKind }) {
  const { t } = useTranslation();
  const router = useRouter();
  const columns = useNativeColumns(kind);
  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Table's API is designed this way
  const table = useReactTable({
    data: rows,
    columns,
    getRowId: (row) => row.workspace.id,
    getCoreRowModel: getCoreRowModel(),
  });
  const label = t(
    kind === "tasks"
      ? "sidebar:controlSummary.compare.taskStages"
      : "sidebar:controlSummary.compare.sessions",
  );
  return (
    <section className="space-y-2" data-testid={`control-native-${kind}`}>
      <h3 className="text-sm font-medium">{label}</h3>
      <div className="rounded-lg border">
        <Table
          aria-label={label}
          className={`${kind === "tasks" ? "min-w-[51.5rem]" : "min-w-[32rem]"} table-fixed`}
        >
          <colgroup>
            {table.getAllLeafColumns().map((column) => (
              <col key={column.id} style={{ width: column.getSize() }} />
            ))}
          </colgroup>
          <TableHeader>
            {table.getHeaderGroups().map((group) => (
              <TableRow key={group.id}>
                {group.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    scope="col"
                    className={`whitespace-normal ${header.index > 0 ? "text-right" : ""}`}
                  >
                    {flexRender(header.column.columnDef.header, header.getContext())}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.map((row) => (
              <TableRow
                key={row.id}
                className="cursor-pointer"
                onClick={() => router.push(controlHref("workspaces", row.original.workspace.id))}
                data-testid={`control-native-${kind}-row`}
              >
                {row.getVisibleCells().map((cell, index) => (
                  <TableCell
                    key={cell.id}
                    className={
                      index === 0 ? "truncate font-medium" : numberCellClass(cell.getValue())
                    }
                  >
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  );
}

export function ControlNativeComparison({
  workspaces,
  revision,
}: {
  workspaces: ControlWorkspace[];
  revision: object;
}) {
  const { t } = useTranslation();
  const read = useNativeDashboards(
    workspaces.map((workspace) => workspace.id),
    revision,
  );
  const rows: NativeRow[] = workspaces.map((workspace) => ({
    workspace,
    ...(read.data?.find((row) => row.id === workspace.id) ?? { status: "loading" as const }),
  }));
  return (
    <section className="space-y-4" data-testid="control-native-comparison">
      <h2 className="text-sm font-semibold">{t("sidebar:controlSummary.compare.title")}</h2>
      <NativeComparisonTable rows={rows} kind="tasks" />
      <NativeComparisonTable rows={rows} kind="sessions" />
      <ControlWaitReasons rows={rows} />
      {rows.map((row) => (
        <ReadState key={row.workspace.id} row={row} />
      ))}
    </section>
  );
}
