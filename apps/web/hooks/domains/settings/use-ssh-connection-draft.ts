"use client";
import { useEffect, useRef, useState } from "react";
import { flushSync } from "react-dom";
import type { ManagedSSHConnection, SSHRoute } from "@/lib/types/http-ssh-connections";
import type { SSHManager } from "./use-ssh-connections";
import { generateUUID } from "@/lib/uuid";
import { useSettingsSaveCoordinator } from "@/components/settings/settings-save-provider";
const empty: ManagedSSHConnection = {
  id: "",
  name: "",
  host: "",
  port: 22,
  user: "",
  identity_id: "",
  fingerprint: "",
  public_key: "",
  version: 0,
  revision: 0,
};
function editable(c: ManagedSSHConnection) {
  return JSON.stringify([c.name, c.host, c.port, c.user, c.identity_id, c.bastion_id ?? ""]);
}
export function useConnectionDraft(route: SSHRoute | undefined, manager: SSHManager) {
  const [baseline, setBaseline] = useState(route?.destination ?? empty);
  const [form, setForm] = useState(baseline);
  const [requestId, setRequestId] = useState(generateUUID);
  const dirty = editable(form) !== editable(baseline);
  useEffect(() => {
    if (route && !dirty) {
      setBaseline(route.destination);
      setForm(route.destination);
    }
  }, [route, dirty]);
  const update = (key: keyof ManagedSSHConnection, value: string | number) => {
    setForm((previous) => ({ ...previous, [key]: value }));
    if (!baseline.id) setRequestId(generateUUID());
  };
  const saveDraft = async () => {
    const saved = await manager.saveConnection({
      request_id: requestId,
      connection: form,
      expected_version: baseline.version,
    });
    flushSync(() => {
      setBaseline(saved.destination);
      setForm(saved.destination);
    });
    return saved;
  };
  return { form, requestId, dirty, update, saveDraft, discard: () => setForm(baseline) };
}

export function useCreateRelated(saveDraft: () => Promise<SSHRoute>) {
  const coordinator = useSettingsSaveCoordinator();
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  return (navigate: (route: SSHRoute) => void) => {
    void coordinator
      .runExclusive(async () => {
        const saved = await saveDraft();
        if (active.current) navigate(saved);
      })
      .catch(() => undefined);
  };
}
