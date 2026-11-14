import { Fragment, useState } from "react";
import { useTranslation } from "react-i18next";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { NUMBER_CELL, numberCellClass } from "./control-table";
import { useNativeLedger } from "@/hooks/domains/coordinator/use-control-native";
import { formatDateTime, formatNumber } from "@/lib/i18n/formats";
import { formatSubcentsUsd } from "@/lib/coordinator/autonomy";
import { useIntegratedControl } from "./integrated-control-provider";
import { ControlReadState } from "./control-read-state";
import { SectionCard } from "./overview-section-card";
import {
  modelTotals,
  visibleTokenFields,
  type Measured,
  type ModelTotal,
  type TokenField,
} from "./control-model-totals";

const TOKEN_LABELS: Record<TokenField, string> = {
  tokens_in: "sidebar:controlNative.tokensIn",
  tokens_out: "sidebar:controlNative.tokensOut",
  tokens_cached_read: "sidebar:controlNative.tokensCachedRead",
  tokens_cached_write: "sidebar:controlNative.tokensCachedWrite",
  tokens_total: "sidebar:controlNative.tokensTotal",
};

const allWorkspaceReadsReady = (rows: Array<{ status: string; stale?: boolean }> | undefined) =>
  rows?.every((workspace) => workspace.status === "ready" && !workspace.stale) ?? false;

function MeasuredValue({ value, cost = false }: { value: Measured; cost?: boolean }) {
  const { t } = useTranslation();
  if (value.value === null)
    return <span className="text-muted-foreground">{t("common:unknown")}</span>;
  const text = cost
    ? t("sidebar:controlSummary.providers.usd", { amount: formatSubcentsUsd(value.value) })
    : formatNumber(value.value);
  return (
    <span title={value.partial ? t("sidebar:controlSummary.providers.partial") : undefined}>
      {text}
      {value.partial && "+"}
    </span>
  );
}

/** Recorded usage per model across workspaces; the workspace split opens per row. */
export function ControlModelActivity({ workspaceId }: { workspaceId?: string }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const workspaces = (control?.data?.workspaces ?? []).filter(
    (w) => !workspaceId || w.id === workspaceId,
  );
  const names = Object.fromEntries(workspaces.map((w) => [w.id, w.name]));
  const result = useNativeLedger(
    workspaces.map((w) => w.id),
    control?.data,
  );
  const ready = (result.data ?? []).flatMap((w) => (w.data ? [{ id: w.id, data: w.data }] : []));
  const rows = modelTotals(ready.map((w) => ({ workspaceId: w.id, models: w.data.models })));
  const tokenFields = visibleTokenFields(rows);
  const period = ready[0]?.data;
  return (
    <SectionCard
      title={t("sidebar:controlNative.ledgerTitle")}
      action={
        period && (
          <span className="text-xs text-muted-foreground">
            {formatDateTime(period.since)} - {formatDateTime(period.generated_at)}
          </span>
        )
      }
    >
      {(result.status !== "ready" || result.stale) && <ControlReadState {...result} />}
      {result.data
        ?.filter((w) => w.status !== "ready")
        .map((w) => (
          <div key={w.id} className="flex items-center px-3 text-xs text-muted-foreground">
            {names[w.id]}
            <ControlReadState {...w} />
          </div>
        ))}
      {result.status === "ready" && allWorkspaceReadsReady(result.data) && rows.length === 0 && (
        <p className="p-3 text-sm">{t("sidebar:controlRestore.noModelActivity")}</p>
      )}
      {rows.length > 0 && (
        <Table data-testid="control-model-table">
          <TableHeader>
            <TableRow>
              <TableHead>{t("sidebar:controlSummary.providers.model")}</TableHead>
              <TableHead>{t("sidebar:controlNative.recordedProvider")}</TableHead>
              <TableHead className="text-right">{t("sidebar:controlNative.events")}</TableHead>
              {tokenFields.map((field) => (
                <TableHead key={field} className="text-right">
                  {t(TOKEN_LABELS[field])}
                </TableHead>
              ))}
              <TableHead className="text-right">
                {t("sidebar:controlSummary.providers.cost")}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <ModelRow key={row.model} row={row} tokenFields={tokenFields} names={names} />
            ))}
          </TableBody>
        </Table>
      )}
    </SectionCard>
  );
}

function ModelRow({
  row,
  tokenFields,
  names,
}: {
  row: ModelTotal;
  tokenFields: TokenField[];
  names: Record<string, string>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const numeric = NUMBER_CELL;
  return (
    <Fragment>
      <TableRow data-testid="control-ledger-model">
        <TableCell className="max-w-72 whitespace-normal break-all">
          <button
            type="button"
            className="cursor-pointer text-left"
            aria-expanded={open}
            onClick={() => setOpen((value) => !value)}
          >
            <span aria-hidden className="mr-1 inline-block w-3 text-muted-foreground">
              {open ? "▾" : "▸"}
            </span>
            {row.model || t("common:unknown")}
          </button>
        </TableCell>
        <TableCell className="text-muted-foreground">
          {row.providers.join(", ") || t("common:unknown")}
        </TableCell>
        <TableCell className={numberCellClass(row.events)}>{formatNumber(row.events)}</TableCell>
        {tokenFields.map((field) => (
          <TableCell
            key={field}
            className={numberCellClass(row.tokens[field].value)}
            data-testid={`control-ledger-${field}`}
          >
            <MeasuredValue value={row.tokens[field]} />
          </TableCell>
        ))}
        <TableCell className={numeric} data-testid="control-ledger-cost">
          <MeasuredValue value={row.cost} cost />
        </TableCell>
      </TableRow>
      {open &&
        row.workspaces.map((share) => (
          <TableRow key={share.workspaceId} className="bg-muted/30 text-xs">
            <TableCell className="pl-8 text-muted-foreground">
              {names[share.workspaceId] ?? share.workspaceId}
            </TableCell>
            <TableCell />
            <TableCell className={numeric}>{formatNumber(share.events)}</TableCell>
            {tokenFields.map((field) => (
              <TableCell key={field} />
            ))}
            <TableCell className={numeric}>
              <MeasuredValue value={share.cost} cost />
            </TableCell>
          </TableRow>
        ))}
    </Fragment>
  );
}
