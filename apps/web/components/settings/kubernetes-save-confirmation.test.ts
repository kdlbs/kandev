import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsSaveCancelledError } from "./settings-save-provider";
import {
  requireKubernetesSessionSaveConfirmation,
  saveWithKubernetesSessionConfirmation,
} from "./kubernetes-save-confirmation";

afterEach(() => vi.unstubAllGlobals());

describe("Kubernetes active-session save confirmation", () => {
  it("does not prompt when no active sessions exist", () => {
    const confirm = vi.fn();
    vi.stubGlobal("confirm", confirm);

    requireKubernetesSessionSaveConfirmation("connection", 0);

    expect(confirm).not.toHaveBeenCalled();
  });

  it("explains that current connection settings apply on reconnect", () => {
    const confirm = vi.fn(() => true);
    vi.stubGlobal("confirm", confirm);

    requireKubernetesSessionSaveConfirmation("connection", 2);

    expect(confirm).toHaveBeenCalledWith(expect.stringMatching(/2 active sessions.*reconnect/i));
  });

  it("explains that existing workload snapshots do not adopt profile edits", () => {
    const confirm = vi.fn(() => true);
    vi.stubGlobal("confirm", confirm);

    requireKubernetesSessionSaveConfirmation("workload", 1);

    expect(confirm).toHaveBeenCalledWith(expect.stringMatching(/recorded workload snapshot/i));
  });

  it("explains both reconnect credentials and recorded workload snapshots once", () => {
    const confirm = vi.fn(() => true);
    vi.stubGlobal("confirm", confirm);

    requireKubernetesSessionSaveConfirmation("connection_and_workload", 2);

    expect(confirm).toHaveBeenCalledOnce();
    expect(confirm).toHaveBeenCalledWith(
      expect.stringMatching(/connection.*reconnect.*workload snapshot/i),
    );
  });

  it("signals a silent coordinated-save cancellation when declined", () => {
    vi.stubGlobal(
      "confirm",
      vi.fn(() => false),
    );

    expect(() => requireKubernetesSessionSaveConfirmation("connection", 1)).toThrow(
      SettingsSaveCancelledError,
    );
  });

  it("refreshes session inventory before confirming and saving", async () => {
    const events: string[] = [];
    vi.stubGlobal(
      "confirm",
      vi.fn(() => {
        events.push("confirm");
        return true;
      }),
    );

    await saveWithKubernetesSessionConfirmation({
      kind: "connection",
      loadActiveSessionCount: async () => {
        events.push("refresh");
        return 1;
      },
      save: async () => {
        events.push("save");
      },
    });

    expect(events).toEqual(["refresh", "confirm", "save"]);
  });

  it("does not save when the active-session confirmation is declined", async () => {
    vi.stubGlobal(
      "confirm",
      vi.fn(() => false),
    );
    const save = vi.fn();

    await expect(
      saveWithKubernetesSessionConfirmation({
        kind: "workload",
        loadActiveSessionCount: async () => 2,
        save,
      }),
    ).rejects.toBeInstanceOf(SettingsSaveCancelledError);
    expect(save).not.toHaveBeenCalled();
  });
});
