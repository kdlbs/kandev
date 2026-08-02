import { toast as upstreamToast } from "sonner";

import { scheduleFrontendErrorReport } from "@/lib/api/domains/frontend-error-log-api";

const reportingError = (
  ...args: Parameters<typeof upstreamToast.error>
): ReturnType<typeof upstreamToast.error> => {
  const result = upstreamToast.error(...args);
  const options = args[1];
  scheduleFrontendErrorReport({
    source: "sonner",
    title: args[0],
    description: ownDataValue(options, "description"),
    error: args[0] instanceof Error ? args[0] : ownDataValue(options, "error"),
  });
  return result;
};

export const toast = new Proxy(upstreamToast, {
  get(target, property, receiver) {
    if (property === "error") return reportingError;
    return Reflect.get(target, property, receiver);
  },
}) as typeof upstreamToast;

function ownDataValue(value: unknown, property: string): unknown {
  if (!value || typeof value !== "object") return undefined;
  try {
    return Object.getOwnPropertyDescriptor(value, property)?.value;
  } catch {
    return undefined;
  }
}
