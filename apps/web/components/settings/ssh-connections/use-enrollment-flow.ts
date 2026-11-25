import { useEffect, useRef, useState } from "react";
import { useConfirmationBoundary } from "@/components/confirmation/mobile-action-confirmation";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHEnrollment, SSHIdentity, SSHRoute } from "@/lib/types/http-ssh-connections";
import { generateUUID } from "@/lib/uuid";
import type { EnrollmentMethod } from "./enrollment-methods";
export function useEnrollmentFlow(route: SSHRoute, identity: SSHIdentity, manager: SSHManager) {
  const [step, setStep] = useState<"choose" | "review" | "pending" | "result">("choose");
  const [method, setMethod] = useState<EnrollmentMethod>("");
  const [password, setPassword] = useState("");
  const [bootstrap, setBootstrap] = useState("");
  const [result, setResult] = useState<SSHEnrollment>();
  const [failed, setFailed] = useState(false);
  const submitted = useRef(false);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const back = () => {
    setPassword("");
    setStep("choose");
    setFailed(false);
    setResult(undefined);
  };
  const boundary = useConfirmationBoundary(
    step === "review",
    `${route.stamp}:${identity.fingerprint}`,
    back,
  );
  const valid = Boolean(
    route.destination.fingerprint &&
    (method === "password" ? password : method === "identity" && bootstrap),
  );
  const started =
    route.latest_enrollment?.stamp === route.stamp && route.latest_enrollment.state === "started";
  const review = () => {
    if (valid && !manager.busy && !started) setStep("review");
  };
  const install = async () => {
    if (
      submitted.current ||
      boundary.changed ||
      step !== "review" ||
      !valid ||
      manager.busy ||
      started
    )
      return;
    submitted.current = true;
    const request = {
      request_id: generateUUID(),
      expected_stamp: route.stamp,
      key_fingerprint: identity.fingerprint,
      mode: method === "password" ? ("password" as const) : ("identity" as const),
      ...(method === "password" ? { password } : { bootstrap_identity_id: bootstrap }),
    };
    setPassword("");
    setStep("pending");
    try {
      const completed = await manager.enroll(route, request);
      if (!active.current) return;
      setResult(completed);
      setStep("result");
    } catch {
      if (active.current) {
        setFailed(true);
        setStep("result");
      }
    } finally {
      submitted.current = false;
    }
  };
  return {
    step,
    method,
    password,
    bootstrap,
    result,
    failed,
    valid,
    started,
    boundary,
    setPassword,
    setBootstrap,
    back,
    review,
    install,
    chooseMethod: (next: EnrollmentMethod) => {
      setPassword("");
      setBootstrap("");
      setMethod(next);
    },
  };
}
