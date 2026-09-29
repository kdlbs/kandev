"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import type { StandingOrder } from "@/lib/api/domains/coordinator-api";

type Label = { id: string; order: StandingOrder };

/** The proposal's orders in `standing_order_ids` order; an id no order matches yields nothing. */
export function shapedByLabels(ids: string[] | undefined, orders: StandingOrder[]): Label[] {
  const byId = new Map(orders.map((order) => [order.id, order]));
  const labels: Label[] = [];
  for (const id of ids ?? []) {
    const order = byId.get(id);
    if (order) labels.push({ id, order });
  }
  return labels;
}

function ShapedByLabel({ order }: { order: StandingOrder }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [pinned, setPinned] = useState(false);
  const active = order.retired_at === null && order.number !== null;
  const text = active
    ? t("coordinator:shapedByOrder", { number: order.number })
    : t("coordinator:shapedByRetired");
  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) setPinned(false);
      }}
    >
      <PopoverTrigger asChild>
        <button
          type="button"
          className="cursor-pointer text-xs/relaxed underline decoration-dotted"
          onMouseEnter={() => setOpen(true)}
          onMouseLeave={() => {
            if (!pinned) setOpen(false);
          }}
          onClick={(event) => {
            event.preventDefault();
            if (open && !pinned) {
              setPinned(true);
              return;
            }
            setOpen(!open);
            setPinned(!open);
          }}
        >
          {text}
        </button>
      </PopoverTrigger>
      <PopoverContent className="text-xs/relaxed whitespace-pre-wrap">{order.text}</PopoverContent>
    </Popover>
  );
}

/** "Shaped by: Standing order N" labels; nothing while orders are unloaded or none match. */
export function ShapedBy({
  ids,
  orders,
}: {
  ids: string[] | undefined;
  orders: StandingOrder[] | undefined;
}) {
  const labels = orders ? shapedByLabels(ids, orders) : [];
  if (labels.length === 0) return null;
  return (
    <p className="text-muted-foreground flex flex-wrap gap-x-2 text-xs/relaxed">
      {labels.map(({ id, order }) => (
        <ShapedByLabel key={id} order={order} />
      ))}
    </p>
  );
}
