import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { IdentityForm } from "./identity-form";
import type { SSHManager } from "@/hooks/domains/settings/use-ssh-connections";
import type { SSHIdentity } from "@/lib/types/http-ssh-connections";
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
afterEach(cleanup);
it("clears credential inputs after a failed import and gives a different key a new request ID", async () => {
  const createIdentity = vi.fn().mockRejectedValue(new Error("import rejected"));
  render(
    <IdentityForm
      manager={{ busy: false, createIdentity } as unknown as SSHManager}
      onCreated={vi.fn()}
      onCancel={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByLabelText("settings:sshManager.label"), {
    target: { value: "label" },
  });
  fireEvent.change(screen.getByLabelText("settings:sshManager.method"), {
    target: { value: "import" },
  });
  fireEvent.change(screen.getByLabelText("settings:sshManager.privateKey"), {
    target: { value: "first-key" },
  });
  fireEvent.change(screen.getByLabelText("settings:sshManager.passphrase"), {
    target: { value: "secret" },
  });
  fireEvent.click(screen.getByRole("button", { name: "settings:sshManager.createIdentity" }));
  await waitFor(() =>
    expect(
      (screen.getByLabelText("settings:sshManager.privateKey") as HTMLTextAreaElement).value,
    ).toBe(""),
  );
  expect((screen.getByLabelText("settings:sshManager.passphrase") as HTMLInputElement).value).toBe(
    "",
  );
  fireEvent.change(screen.getByLabelText("settings:sshManager.privateKey"), {
    target: { value: "different-key" },
  });
  fireEvent.click(screen.getByRole("button", { name: "settings:sshManager.createIdentity" }));
  await waitFor(() => expect(createIdentity).toHaveBeenCalledTimes(2));
  expect(createIdentity.mock.calls[0][0].request_id).not.toBe(
    createIdentity.mock.calls[1][0].request_id,
  );
});

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.3
it("does not navigate back to an identity after its form was dismissed", async () => {
  let finish!: (identity: SSHIdentity) => void;
  const created = new Promise<SSHIdentity>((resolve) => {
    finish = resolve;
  });
  const onCreated = vi.fn();
  const { unmount } = render(
    <IdentityForm
      manager={{ busy: false, createIdentity: () => created } as unknown as SSHManager}
      onCreated={onCreated}
      onCancel={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByLabelText("settings:sshManager.label"), {
    target: { value: "New key" },
  });
  fireEvent.click(screen.getByRole("button", { name: "settings:sshManager.createIdentity" }));
  unmount();
  await act(async () => {
    finish({
      id: "new",
      label: "New key",
      kind: "managed",
      public_key: "ssh-ed25519 public",
      fingerprint: "SHA256:key",
      generation: "1",
      version: 1,
    });
    await created;
  });
  expect(onCreated).not.toHaveBeenCalled();
});
