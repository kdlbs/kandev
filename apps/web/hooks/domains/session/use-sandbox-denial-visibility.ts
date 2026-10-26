import { useState } from "react";
import type { SandboxDenial } from "@/lib/api/domains/sandbox-reports-api";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";

export function useSandboxDenialVisibility(report: SandboxReport) {
  const scope = JSON.stringify([
    report.session_id,
    report.attempt_id,
    report.policy_digest,
    report.ceiling_digest,
    report.provider,
  ]);
  const [hidden, setHidden] = useState({ scope, keys: new Set<string>() });
  const all = report.data.records ?? [];
  const records =
    hidden.scope === scope ? all.filter((record) => !hidden.keys.has(noticeKey(record))) : all;
  return {
    records,
    hiddenCount: all.length - records.length,
    hide: (ids: string[]) =>
      setHidden((current) => {
        const keys = new Set(current.scope === scope ? current.keys : []);
        for (const record of all) if (ids.includes(record.id)) keys.add(noticeKey(record));
        return { scope, keys };
      }),
    showAll: () => setHidden({ scope, keys: new Set() }),
  };
}

function noticeKey(record: SandboxDenial) {
  return JSON.stringify([record.kind, record.operation, record.target]);
}

export type SandboxDenialVisibility = ReturnType<typeof useSandboxDenialVisibility>;
