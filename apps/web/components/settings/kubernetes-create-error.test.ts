import { describe, expect, it } from "vitest";
import { KubernetesExecutorRollbackRejectedError } from "@/hooks/domains/settings/use-kubernetes-settings";
import { kubernetesCreateErrorMessage } from "./kubernetes-create-error";

function translate(key: string, values?: Record<string, unknown>): string {
  if (key === "executors:kubernetesCreateRollbackFailed") {
    return `${String(values?.profileError)} | ${String(values?.rollbackError)}`;
  }
  return key;
}

describe("Kubernetes create error presentation", () => {
  it("localizes a rejected compensating delete while retaining the profile error", () => {
    const failure = new AggregateError([
      new Error("profile validation failed"),
      new KubernetesExecutorRollbackRejectedError(),
    ]);

    expect(kubernetesCreateErrorMessage(failure, translate)).toBe(
      "profile validation failed | executors:kubernetesCreateRollbackRejected",
    );
  });

  it("uses a useful localized cause when rollback has no error detail", () => {
    const failure = new AggregateError([new Error(), new Error()]);

    expect(kubernetesCreateErrorMessage(failure, translate)).toBe(
      "executors:kubernetesCreateFailed | executors:kubernetesCreateRollbackUnknown",
    );
  });
});
