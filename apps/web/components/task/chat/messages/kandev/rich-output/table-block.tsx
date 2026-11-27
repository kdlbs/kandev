import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { flexRender, getCoreRowModel, useReactTable, type ColumnDef } from "@tanstack/react-table";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { TableProps } from "@/lib/rich-output/catalog";

function CellValue({ value }: { value: string }) {
  const { t } = useTranslation();
  return value === "" ? (
    <span className="text-muted-foreground">{t("task:richOutputEmptyCell")}</span>
  ) : (
    <>{value}</>
  );
}

export function EvidenceTable({ title, columns, rows }: TableProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const definitions = useMemo<ColumnDef<string[]>[]>(
    () =>
      columns.map((column, index) => ({
        id: column.key,
        header: column.label,
        accessorFn: (row) => row[index],
        cell: (cell) => <CellValue value={cell.getValue<string>()} />,
      })),
    [columns],
  );
  const table = useReactTable({
    data: rows,
    columns: definitions,
    getCoreRowModel: getCoreRowModel(),
  });
  return (
    <section
      className="min-w-0 space-y-3"
      data-testid="rich-output-table"
      aria-label={title || t("task:richOutputTableName")}
    >
      {title && <h4 className="text-xs font-medium">{title}</h4>}
      {rows.length === 0 && (
        <p className="text-xs text-muted-foreground">{t("task:richOutputNoRows")}</p>
      )}
      {rows.length > 0 && isMobile && (
        <div className="space-y-3" data-testid="rich-output-table-phone">
          {rows.map((row, index) => (
            <dl className="min-w-0 space-y-2 rounded-lg border p-3 text-xs" key={index}>
              {columns.map((column, cell) => (
                <div className="min-w-0" key={column.key}>
                  <dt className="break-words font-medium text-muted-foreground">{column.label}</dt>
                  <dd className="whitespace-pre-wrap wrap-anywhere">
                    <CellValue value={row[cell]} />
                  </dd>
                </div>
              ))}
            </dl>
          ))}
        </div>
      )}
      {rows.length > 0 && !isMobile && (
        <Table aria-label={title || t("task:richOutputTableName")}>
          <TableHeader>
            {table.getHeaderGroups().map((group) => (
              <TableRow key={group.id}>
                {group.headers.map((header) => (
                  <TableHead
                    className="whitespace-normal wrap-anywhere"
                    key={header.id}
                    scope="col"
                  >
                    {flexRender(header.column.columnDef.header, header.getContext())}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows.map((row) => (
              <TableRow key={row.id}>
                {row.getVisibleCells().map((cell) => (
                  <TableCell className="whitespace-pre-wrap wrap-anywhere" key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  );
}
