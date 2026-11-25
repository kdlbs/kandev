import { expect, it } from "vitest";
import { diagnosticIssue } from "./diagnostic-presentation";
it.each([
  ["ssh_target_required", "edit"],
  ["ssh_identity_required", "edit"],
  ["ssh_trust_required", "access"],
  ["ssh_host_key_changed", "access"],
  ["ssh_bastion_not_ready", "access"],
  ["ssh_authentication_rejected", "access"],
  ["ssh_agent_empty", "identity"],
  ["ssh_agent_unavailable", "identity"],
  ["ssh_key_file_unavailable", "identity"],
  ["ssh_managed_key_unavailable", "identity"],
])("maps %s to an existing non-mutating destination", (code, remedy) => {
  expect(diagnosticIssue(code, "destination")).toMatchObject({ code, remedy, hop: "destination" });
});
it("keeps unknown codes safe and never invents a hop or repair action", () => {
  expect(diagnosticIssue("ssh_future_problem", "unexpected")).toEqual({
    code: "ssh_operation_failed",
    remedy: undefined,
    hop: undefined,
    technicalCode: "ssh_future_problem",
  });
  expect(diagnosticIssue("private\nraw failure").technicalCode).toBeUndefined();
});
