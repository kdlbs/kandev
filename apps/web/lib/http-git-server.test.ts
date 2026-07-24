import { expect, test } from "vitest";
import { startHTTPGitFixture } from "../e2e/helpers/http-git-server";
import { execFileSync } from "node:child_process";
import { type Server } from "node:http";
import fs from "node:fs";
import net from "node:net";
import os from "node:os";
import path from "node:path";

test("uses a trusted GitLab URL with isolated executor and backend Git rewrites", async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-http-git-test-"));
  try {
    const fixture = await startHTTPGitFixture(root, "rewrite-source");
    try {
      expect(fixture.remoteURL).toBe("https://gitlab.com/fixture/rewrite-source.git");
      expect(fixture.gitConfigEnvVars).toEqual([
        { key: "GIT_CONFIG_COUNT", value: "1" },
        {
          key: "GIT_CONFIG_KEY_0",
          value: expect.stringMatching(/^url\.http:\/\/[^/]+\/.insteadOf$/),
        },
        { key: "GIT_CONFIG_VALUE_0", value: "https://gitlab.com/" },
      ]);
      expect(fixture.backendEnv).toEqual({
        GIT_CONFIG_GLOBAL: expect.stringMatching(/rewrite-source\.gitconfig$/),
      });
      expect(fixture.backendEnv.GIT_CONFIG_GLOBAL.startsWith(root)).toBe(true);
      const configuredRewrite = execFileSync(
        "git",
        ["config", "--global", "--get-regexp", "^url\\..*\\.insteadOf$"],
        { env: { ...process.env, ...fixture.backendEnv }, encoding: "utf8" },
      ).trim();
      expect(configuredRewrite).toMatch(
        /^url\.http:\/\/[^/]+\/fixture\/rewrite-source\.git\.insteadof https:\/\/gitlab\.com\/fixture\/rewrite-source\.git$/,
      );
    } finally {
      await fixture.close();
    }
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("closes its listener when backend config setup fails after listening", async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-http-git-test-"));
  let port: number | undefined;
  try {
    await expect(
      startHTTPGitFixture(root, "config-write-failure", {
        onListening: (listeningPort) => {
          port = listeningPort;
        },
        writeBackendGitConfig: () => {
          throw new Error("config write failed");
        },
      }),
    ).rejects.toThrow("config write failed");

    expect(port).toBeDefined();
    expect(await canConnect(port!)).toBe(false);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

test("aggregates setup and listener-close failures", async () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-http-git-test-"));
  const setupError = new Error("config write failed");
  const closeError = new Error("listener close failed");
  try {
    await expect(
      startHTTPGitFixture(root, "close-failure", {
        writeBackendGitConfig: () => {
          throw setupError;
        },
        closeServer: async (server) => {
          await closeTestServer(server);
          throw closeError;
        },
      }),
    ).rejects.toMatchObject({
      errors: [setupError, closeError],
      message: "HTTP Git fixture setup failed and its server did not close",
    });
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

function canConnect(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const socket = net.connect(port, "127.0.0.1");
    socket.once("connect", () => {
      socket.destroy();
      resolve(true);
    });
    socket.once("error", () => resolve(false));
  });
}

function closeTestServer(server: Server): Promise<void> {
  return new Promise((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
}
