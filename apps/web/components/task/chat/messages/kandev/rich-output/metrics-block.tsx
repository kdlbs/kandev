import type { MetricsProps } from "@/lib/rich-output/catalog";

export function Metrics({ items }: MetricsProps) {
  return (
    <dl
      className="grid min-w-0 grid-cols-1 gap-x-5 gap-y-3 min-[420px]:grid-cols-2 sm:grid-cols-3"
      data-testid="rich-output-metrics"
    >
      {items.map((item, index) => (
        <div className="min-w-0 space-y-0.5" key={`${item.label}-${index}`}>
          <dt className="truncate text-[11px] font-medium text-muted-foreground">{item.label}</dt>
          <dd className="break-words font-mono text-xl font-medium leading-tight tabular-nums text-foreground">
            {item.value}
          </dd>
          {item.detail && (
            <dd className="text-[11px] leading-relaxed text-muted-foreground">{item.detail}</dd>
          )}
        </div>
      ))}
    </dl>
  );
}
