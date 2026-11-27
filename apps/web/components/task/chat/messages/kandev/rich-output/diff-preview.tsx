import { useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { PreviewDiff } from "./diff-preview-model";

const pageSize = 100;
const control = "max-md:min-h-11 [@media(pointer:coarse)]:min-h-11";
const tones = {
  header: "bg-muted text-muted-foreground",
  add: "bg-green-500/15",
  remove: "bg-red-500/15",
  context: "",
};

export function DiffPreview({ diff }: { diff: PreviewDiff }) {
  const { t } = useTranslation();
  const [split, setSplit] = useState(true);
  const [page, setPage] = useState(0);
  const viewport = useRef<HTMLDivElement>(null);
  const target = useRef(0);
  const total = split ? diff.split.length : diff.lines.length;
  const start = page * pageSize;
  const end = Math.min(start + pageSize, total);

  useLayoutEffect(() => {
    const box = viewport.current;
    const row = box?.querySelector<HTMLElement>(`[data-row="${target.current}"]`);
    if (box && target.current === page * pageSize) box.scrollTop = 0;
    else if (box && row)
      box.scrollTop += row.getBoundingClientRect().top - box.getBoundingClientRect().top;
  }, [split, page]);

  function switchLayout(next: boolean) {
    if (next === split) return;
    const box = viewport.current!;
    const top = box.getBoundingClientRect().top;
    const visible = Array.from(box.querySelectorAll<HTMLElement>("[data-row]")).find(
      (row) => row.getBoundingClientRect().bottom > top,
    );
    const current = Number(visible?.dataset.row ?? start);
    const source = split ? (diff.split[current][0] ?? diff.split[current][1]!) : current;
    target.current = next
      ? diff.split.findIndex((row) => row.some((index) => index === source))
      : source;
    setPage(Math.floor(target.current / pageSize));
    setSplit(next);
  }

  function movePage(next: number) {
    target.current = next * pageSize;
    setPage(next);
  }

  return (
    <div className="min-w-0 space-y-2">
      <div className="flex gap-1">
        {[true, false].map((side) => (
          <Button
            key={String(side)}
            type="button"
            size="sm"
            className={control}
            variant={split === side ? "secondary" : "ghost"}
            aria-pressed={split === side}
            onClick={() => switchLayout(side)}
          >
            {side ? t("task:questionReviewModeSide") : t("task:unified")}
          </Button>
        ))}
      </div>
      <div ref={viewport} tabIndex={0} className="max-h-80 overflow-auto rounded-md border">
        <table className="w-full table-fixed border-collapse whitespace-pre font-mono text-xs leading-5">
          {split && (
            <thead>
              <tr>
                {[t("diff:before"), t("diff:after")].map((label) => (
                  <th key={label} scope="col" className="w-1/2 px-2 text-start capitalize">
                    {label}
                  </th>
                ))}
              </tr>
            </thead>
          )}
          <DiffRows diff={diff} split={split} start={start} end={end} />
        </table>
      </div>
      {total > pageSize && (
        <div className="flex flex-wrap items-center justify-between gap-1 text-xs">
          <Button
            className={control}
            size="sm"
            variant="ghost"
            disabled={!page}
            onClick={() => movePage(page - 1)}
          >
            {t("diff:previous")}
          </Button>
          <span aria-live="polite">
            {t("diff:lineRangeLabel", { start: start + 1, end })} / {total}
          </span>
          <Button
            className={control}
            size="sm"
            variant="ghost"
            disabled={end === total}
            onClick={() => movePage(page + 1)}
          >
            {t("diff:next")}
          </Button>
        </div>
      )}
    </div>
  );
}

function DiffRows({
  diff,
  split,
  start,
  end,
}: {
  diff: PreviewDiff;
  split: boolean;
  start: number;
  end: number;
}) {
  return (
    <tbody>
      {Array.from({ length: end - start }, (_, offset) => {
        const index = start + offset;
        const cells = split ? diff.split[index] : [index];
        return (
          <tr key={index} data-row={index}>
            {cells.map((source, column) => (
              <td
                key={column}
                colSpan={split && cells.length === 1 ? 2 : 1}
                className={`px-2 align-top border-r last:border-r-0 ${source === null ? "bg-muted/40" : tones[diff.kinds[source]]}`}
              >
                <div className="overflow-x-auto">{source === null ? "" : diff.lines[source]}</div>
              </td>
            ))}
          </tr>
        );
      })}
    </tbody>
  );
}
