import { describe, expect, it } from "vitest";
import type { SSHExecutorConfig } from "@/components/settings/ssh-connection-card";
import { remoteDockerConnectionConfig } from "./remote-docker-connection-config";

const form: SSHExecutorConfig = {
  name: "Docker box",
  host: " box.lan ",
  host_alias: "",
  port: 2222,
  user: " kandev ",
  identity_source: "file",
  identity_file: " ~/.ssh/id_ed25519 ",
  proxy_jump: "",
  host_fingerprint: "SHA256:new",
};

describe("remoteDockerConnectionConfig", () => {
  it("serializes the connection the way the create flow does", () => {
    expect(remoteDockerConnectionConfig({}, form)).toEqual({
      ssh_identity_source: "file",
      ssh_host: "box.lan",
      ssh_port: "2222",
      ssh_user: "kandev",
      ssh_identity_file: "~/.ssh/id_ed25519",
      ssh_host_fingerprint: "SHA256:new",
    });
  });

  it("drops connection fields the form cleared instead of saving empty strings", () => {
    const existing = {
      ssh_host: "old.lan",
      ssh_proxy_jump: "bastion",
      ssh_host_fingerprint: "SHA256:old",
    };
    const saved = remoteDockerConnectionConfig(existing, form);
    expect(saved).not.toHaveProperty("ssh_proxy_jump");
    expect(saved).not.toHaveProperty("ssh_host_alias");
    expect(saved.ssh_host_fingerprint).toBe("SHA256:new");
  });

  it("keeps executor config that is not part of the connection", () => {
    expect(remoteDockerConnectionConfig({ unrelated_key: "kept" }, form).unrelated_key).toBe(
      "kept",
    );
  });
});
