import { expect, it } from "vitest";
import {
  buildInlineSSHExecutorConfig,
  buildSSHExecutorConfig,
  parseInlineSSHExecutorConfig,
  parseManagedSSHExecutorConfig,
} from "./ssh-config";

it("preserves legacy inline fields and leaves inherited ports unspecified", () => {
  expect(
    buildInlineSSHExecutorConfig({
      name: "legacy",
      identity_source: "file",
      host_alias: " build ",
      host: " ",
      user: " deploy ",
      identity_file: " /fixture/key ",
      proxy_jump: " gateway ",
      host_fingerprint: " SHA256:fixture ",
    }),
  ).toEqual({
    ssh_identity_source: "file",
    ssh_host_alias: "build",
    ssh_user: "deploy",
    ssh_identity_file: "/fixture/key",
    ssh_proxy_jump: "gateway",
    ssh_host_fingerprint: "SHA256:fixture",
  });
  expect(parseInlineSSHExecutorConfig("legacy", { ssh_host_alias: "build" }).port).toBeUndefined();
});

it("managed bindings exclude inline routing and preserve the checked route stamp", () => {
  const config = buildSSHExecutorConfig({
    name: "shared",
    identity_source: "agent",
    connection_id: "managed",
    connection_stamp: "checked",
    host: "ignored",
    host_fingerprint: "ignored",
  });
  expect(config).toEqual({ ssh_connection_id: "managed", ssh_connection_stamp: "checked" });
  expect(parseManagedSSHExecutorConfig("shared", config)).toMatchObject({
    connection_id: "managed",
    connection_stamp: "checked",
  });
});
