import { cn } from "@/lib/utils";

type BudgetGaugeProps = {
  budgetCents: number;
  spentCents?: number;
  className?: string;
};

export function BudgetGauge({ budgetCents, spentCents = 0, className }: BudgetGaugeProps) {
  if (budgetCents <= 0) {
    return <span className={cn("text-xs text-muted-foreground", className)}>No budget</span>;
  }

  const pct = Math.min(100, Math.round((spentCents / budgetCents) * 100));
  let barColor = "bg-green-500";
  if (pct > 90) barColor = "bg-red-500";
  else if (pct > 70) barColor = "bg-yellow-500";

  return (
    <div className={cn("flex items-center gap-2", className)}>
      <div className="h-1.5 w-16 bg-muted rounded-full overflow-hidden">
        <div className={cn("h-full rounded-full", barColor)} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs text-muted-foreground">${(budgetCents / 100).toFixed(0)}/mo</span>
    </div>
  );
}
