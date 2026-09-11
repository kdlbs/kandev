import { execFileSync } from "node:child_process";
import { request as httpRequest, type IncomingMessage } from "node:http";
import type { Socket } from "node:net";
import {
  createServer as createTLSServer,
  request as httpsRequest,
  type Server as TLSServer,
} from "node:https";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { Page, Route } from "@playwright/test";

type OriginAlias = {
  origin: string;
  host: string;
};

type ProxyResponse = {
  status: number;
  headers: Record<string, string>;
  body: Buffer;
};

type RuntimeRequestWaiter = {
  runtimePath: string;
  refererOrigin: string;
  resolve: () => void;
};

export type CanvasOriginFixture = {
  aliases: {
    primary: OriginAlias;
    secondary: OriginAlias;
    foreign: OriginAlias;
    nestedForeign: OriginAlias;
  };
  install: (page: Page) => Promise<void>;
  waitForRuntimeRequest: (runtimePath: string, refererOrigin: string) => Promise<void>;
  close: () => Promise<void>;
};

const virtualPagePath = "/__kandev_canvas_origin_test__";

export async function startCanvasOriginFixture(backendURL: string): Promise<CanvasOriginFixture> {
  const certDirectory = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-canvas-origin-cert-"));
  const keyPath = path.join(certDirectory, "key.pem");
  const certPath = path.join(certDirectory, "cert.pem");
  const configPath = path.join(certDirectory, "openssl.cnf");
  fs.writeFileSync(
    configPath,
    [
      "[req]",
      "distinguished_name = req_distinguished_name",
      "x509_extensions = v3_req",
      "prompt = no",
      "[req_distinguished_name]",
      "CN = canvas-a.example.test",
      "[v3_req]",
      "subjectAltName = @alt_names",
      "[alt_names]",
      "DNS.1 = canvas-a.example.test",
      "DNS.2 = canvas-b.example.test",
      "DNS.3 = canvas-foreign.example.test",
      "DNS.4 = canvas-nested.example.test",
      "",
    ].join("\n"),
  );
  execFileSync(
    "openssl",
    [
      "req",
      "-x509",
      "-newkey",
      "rsa:2048",
      "-nodes",
      "-keyout",
      keyPath,
      "-out",
      certPath,
      "-config",
      configPath,
      "-extensions",
      "v3_req",
      "-days",
      "1",
    ],
    { stdio: "ignore" },
  );

  const backend = new URL(backendURL);
  const server = createTLSServer(
    {
      key: fs.readFileSync(keyPath),
      cert: fs.readFileSync(certPath),
    },
    (incoming, outgoing) => {
      const upstream = httpRequest(
        {
          hostname: backend.hostname,
          port: Number(backend.port),
          path: incoming.url,
          method: incoming.method,
          headers: {
            ...incoming.headers,
            host: backend.host,
          },
        },
        (response) => {
          outgoing.writeHead(response.statusCode ?? 502, response.headers);
          response.pipe(outgoing);
        },
      );
      upstream.on("error", (_error) => {
        if (!outgoing.headersSent) outgoing.writeHead(502);
        outgoing.end();
      });
      incoming.pipe(upstream);
    },
  );
  const port = await listen(server);
  const sockets = new Set<Socket>();
  server.on("connection", (socket) => {
    sockets.add(socket);
    socket.once("close", () => sockets.delete(socket));
  });
  const runtimeRequestWaiters: RuntimeRequestWaiter[] = [];
  const aliases = {
    primary: originAlias("canvas-a.example.test", port),
    secondary: originAlias("canvas-b.example.test", port),
    foreign: originAlias("canvas-foreign.example.test", port),
    nestedForeign: originAlias("canvas-nested.example.test", port),
  };

  return {
    aliases,
    install: async (page) => {
      const origins = Object.values(aliases).map((alias) => alias.origin);
      await page.route(
        /https:\/\/canvas-(?:a|b|foreign|nested)\.example\.test:\d+\/.*/,
        async (route) => {
          const requestURL = new URL(route.request().url());
          const requestReferer = route.request().headers().referer ?? "";
          try {
            await handleCanvasOriginRoute(route, origins, port);
          } finally {
            const waiterIndex = runtimeRequestWaiters.findIndex(
              (waiter) =>
                waiter.runtimePath === requestURL.pathname &&
                requestReferer.startsWith(waiter.refererOrigin),
            );
            if (waiterIndex >= 0) {
              runtimeRequestWaiters.splice(waiterIndex, 1)[0]?.resolve();
            }
          }
        },
      );
    },
    waitForRuntimeRequest: (runtimePath, refererOrigin) =>
      new Promise<void>((resolve) => {
        runtimeRequestWaiters.push({ runtimePath, refererOrigin, resolve });
      }),
    close: async () => {
      await closeServer(server, sockets);
      fs.rmSync(certDirectory, { recursive: true, force: true });
    },
  };
}

function originAlias(hostname: string, port: number): OriginAlias {
  return { host: `${hostname}:${port}`, origin: `https://${hostname}:${port}` };
}

async function handleCanvasOriginRoute(
  route: Route,
  origins: string[],
  port: number,
): Promise<void> {
  const requestURL = new URL(route.request().url());
  if (requestURL.pathname === virtualPagePath) {
    const source = requestURL.searchParams.get("src");
    if (!source || !origins.some((origin) => source.startsWith(origin))) {
      await route.fulfill({ status: 400, body: "missing test frame source" });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "text/html",
      body: canvasWrapperHTML(source),
    });
    return;
  }
  // The origin test verifies document policy, not long-lived event delivery.
  // Complete the stream locally so navigation between aliases does not reset
  // an in-flight proxy request while the browser tears down the old iframe.
  if (requestURL.pathname.endsWith("/_kandev/v1/events")) {
    await route.fulfill({
      status: 200,
      headers: {
        "Cache-Control": "no-cache",
        "Content-Type": "text/event-stream",
      },
      body: ": origin fixture stream\n\n",
    });
    return;
  }

  const response = await requestThroughTLSProxy({
    port,
    host: requestURL.host,
    path: `${requestURL.pathname}${requestURL.search}`,
    method: route.request().method(),
    headers: route.request().headers(),
    body: route.request().postDataBuffer() ?? undefined,
  });
  await route.fulfill({
    status: response.status,
    headers: response.headers,
    body: response.body,
  });
}

function canvasWrapperHTML(source: string): string {
  const escapedSource = source
    .replaceAll("&", "&amp;")
    .replaceAll('"', "&quot;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
  return [
    "<!doctype html>",
    '<html lang="en"><head><meta charset="utf-8"><title>Canvas origin test</title></head>',
    '<body style="margin:0"><iframe id="runtime" title="Canvas runtime" style="display:block;width:100vw;height:100vh;border:0" src="',
    escapedSource,
    '"></iframe></body></html>',
  ].join("");
}

function requestThroughTLSProxy(options: {
  port: number;
  host: string;
  path: string;
  method: string;
  headers: Record<string, string>;
  body?: Buffer;
}): Promise<ProxyResponse> {
  return new Promise((resolve, reject) => {
    const request = httpsRequest(
      {
        hostname: "127.0.0.1",
        port: options.port,
        path: options.path,
        method: options.method,
        rejectUnauthorized: false,
        headers: { ...options.headers, host: options.host },
      },
      (response: IncomingMessage) => {
        const chunks: Buffer[] = [];
        response.on("data", (chunk: Buffer) => chunks.push(chunk));
        response.on("end", () => {
          const headers: Record<string, string> = {};
          for (const [key, value] of Object.entries(response.headers)) {
            if (
              value !== undefined &&
              !["connection", "keep-alive", "transfer-encoding"].includes(key)
            ) {
              headers[key] = Array.isArray(value) ? value.join(", ") : value;
            }
          }
          resolve({ status: response.statusCode ?? 502, headers, body: Buffer.concat(chunks) });
        });
      },
    );
    request.on("error", (error) => {
      reject(new Error(`canvas origin proxy ${options.host}${options.path}: ${error.message}`));
    });
    if (options.body) request.write(options.body);
    request.end();
  });
}

function listen(server: TLSServer): Promise<number> {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("canvas origin fixture did not receive a port"));
        return;
      }
      resolve(address.port);
    });
  });
}

function closeServer(server: TLSServer | undefined, sockets: Set<Socket>): Promise<void> {
  if (!server) return Promise.resolve();
  return new Promise((resolve, reject) => {
    for (const socket of sockets) socket.destroy();
    server.close((error) => (error ? reject(error) : resolve()));
  });
}
