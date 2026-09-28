import { execFileSync } from "node:child_process";
import { request as httpRequest, type ClientRequest, type IncomingMessage } from "node:http";
import type { Socket } from "node:net";
import { createServer as createTLSServer, type Server as TLSServer } from "node:https";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import type { BrowserContext } from "@playwright/test";

export const CANVAS_PROXY_COOKIE_NAME = "kandev_e2e_access";

type RuntimeRequestKind =
  | "runtime-entry"
  | "runtime-host"
  | "runtime-asset"
  | "context"
  | "data"
  | "write"
  | "events";

export type CanvasProxyObservation = {
  authenticated: boolean;
  host: string;
  kind: RuntimeRequestKind;
  method: string;
};

export type CanvasAuthenticatedProxy = {
  origin: string;
  host: string;
  cookieName: string;
  setAuthentication: (context: BrowserContext) => Promise<void>;
  clearAuthentication: (context: BrowserContext) => Promise<void>;
  count: (kind: RuntimeRequestKind, authenticated?: boolean) => number;
  observations: () => CanvasProxyObservation[];
  activeUpstreamRequests: () => number;
  injectedRuntimeDocuments: () => number;
  close: () => Promise<void>;
};

export type CanvasAuthenticatedProxyOptions = {
  /** Ask the proxy to try an HTML transformation when the origin permits it. */
  injectRuntimeHtml?: boolean;
  /** Simulate a proxy that removes the origin's no-transform directive. */
  stripNoTransform?: boolean;
};

const runtimePrefix = "/api/v1/plugins/web-apps/runtime/";

export async function startCanvasAuthenticatedProxy(
  backendURL: string,
  options: CanvasAuthenticatedProxyOptions = {},
): Promise<CanvasAuthenticatedProxy> {
  const certificate = createCertificate();
  const backend = new URL(backendURL);
  const observations: CanvasProxyObservation[] = [];
  const upstreamRequests = new Set<ClientRequest>();
  const upstreamResponses = new Set<IncomingMessage>();
  let injectedRuntimeDocumentCount = 0;
  const server = createTLSServer(
    {
      key: fs.readFileSync(certificate.keyPath),
      cert: fs.readFileSync(certificate.certPath),
    },
    (incoming, outgoing) => {
      const requestURL = new URL(incoming.url ?? "/", "https://127.0.0.1");
      const kind = runtimeRequestKind(requestURL.pathname, incoming.method ?? "GET");
      const host = incoming.headers.host ?? "";
      const authenticated = hasAuthenticationCookie(incoming.headers.cookie);
      if (kind) {
        observations.push({
          authenticated,
          host,
          kind,
          method: incoming.method ?? "GET",
        });
      }

      if (requestURL.pathname.startsWith(runtimePrefix) && !authenticated) {
        outgoing.writeHead(401, {
          "Cache-Control": "no-store",
          "Content-Type": "application/json; charset=utf-8",
        });
        outgoing.end(JSON.stringify({ error: "proxy_auth_required" }));
        return;
      }

      const publicHost = host || `${certificate.hostname}:${certificate.port}`;
      const headers = {
        ...incoming.headers,
        host: publicHost,
        "x-forwarded-host": publicHost,
        "x-forwarded-proto": "https",
      };
      let upstreamResponse: IncomingMessage | undefined;
      const upstream = httpRequest(
        {
          hostname: backend.hostname,
          port: Number(backend.port),
          path: incoming.url,
          method: incoming.method,
          headers,
        },
        (response) => {
          upstreamResponse = response;
          upstreamResponses.add(response);
          response.once("close", () => upstreamResponses.delete(response));
          const headers = proxyResponseHeaders(response.headers, options.stripNoTransform === true);
          const shouldTransform =
            options.injectRuntimeHtml === true &&
            kind === "runtime-entry" &&
            isHtmlContentType(headers["content-type"]) &&
            !hasNoTransformDirective(headers["cache-control"]) &&
            !headers["content-encoding"];
          if (shouldTransform) {
            const chunks: Buffer[] = [];
            response.on("data", (chunk: Buffer | string) => chunks.push(Buffer.from(chunk)));
            response.once("end", () => {
              if (outgoing.destroyed) return;
              const original = Buffer.concat(chunks);
              const transformed = injectRuntimeHtml(original);
              if (transformed.equals(original)) {
                outgoing.writeHead(response.statusCode ?? 502, headers);
                outgoing.end(original);
                return;
              }
              injectedRuntimeDocumentCount += 1;
              const transformedHeaders = {
                ...headers,
                "content-length": String(transformed.length),
              };
              delete transformedHeaders["transfer-encoding"];
              outgoing.writeHead(response.statusCode ?? 502, transformedHeaders);
              outgoing.end(transformed);
            });
          } else {
            outgoing.writeHead(response.statusCode ?? 502, headers);
            outgoing.flushHeaders();
            response.pipe(outgoing);
          }
          response.once("error", (error) => {
            if (!outgoing.destroyed) outgoing.destroy(error);
            destroyUpstream(error);
          });
          response.once("aborted", () => destroyUpstream());
        },
      );
      upstreamRequests.add(upstream);
      const destroyUpstream = (error?: Error) => {
        if (!upstream.destroyed) upstream.destroy(error);
        if (upstreamResponse && !upstreamResponse.destroyed) upstreamResponse.destroy(error);
      };
      const destroyIncompleteUpstream = () => {
        if (!incoming.complete) destroyUpstream();
      };
      const removeUpstream = () => {
        upstreamRequests.delete(upstream);
        incoming.off("aborted", destroyUpstream);
        incoming.off("error", destroyUpstream);
        incoming.off("close", destroyIncompleteUpstream);
        outgoing.off("close", destroyUpstream);
        outgoing.off("error", destroyUpstream);
      };
      incoming.once("aborted", destroyUpstream);
      incoming.once("error", destroyUpstream);
      incoming.once("close", destroyIncompleteUpstream);
      outgoing.once("close", destroyUpstream);
      outgoing.once("error", destroyUpstream);
      upstream.once("close", removeUpstream);
      upstream.on("error", (error) => {
        if (outgoing.destroyed) return;
        if (!outgoing.headersSent) {
          outgoing.writeHead(502);
          outgoing.end();
        } else {
          outgoing.destroy(error);
        }
      });
      incoming.pipe(upstream);
    },
  );
  const port = await listen(server);
  certificate.port = port;
  const sockets = new Set<Socket>();
  server.on("connection", (socket) => {
    sockets.add(socket);
    socket.once("close", () => sockets.delete(socket));
  });

  const origin = `https://${certificate.hostname}:${port}`;
  const host = `${certificate.hostname}:${port}`;
  return {
    origin,
    host,
    cookieName: CANVAS_PROXY_COOKIE_NAME,
    setAuthentication: async (context) => {
      await context.addCookies([
        {
          name: CANVAS_PROXY_COOKIE_NAME,
          value: "valid",
          url: origin,
          httpOnly: true,
          secure: true,
          sameSite: "Lax",
        },
      ]);
    },
    clearAuthentication: async (context) => {
      await context.clearCookies({ name: CANVAS_PROXY_COOKIE_NAME });
    },
    count: (kind, authenticated) =>
      observations.filter(
        (observation) =>
          observation.kind === kind &&
          (authenticated === undefined || observation.authenticated === authenticated),
      ).length,
    observations: () => observations.map((observation) => ({ ...observation })),
    activeUpstreamRequests: () => upstreamRequests.size,
    injectedRuntimeDocuments: () => injectedRuntimeDocumentCount,
    close: async () => {
      await closeServer(server, sockets, upstreamRequests, upstreamResponses);
      fs.rmSync(certificate.directory, { recursive: true, force: true });
    },
  };
}

function proxyResponseHeaders(
  source: IncomingMessage["headers"],
  stripNoTransform: boolean,
): IncomingMessage["headers"] {
  const headers = { ...source };
  if (!stripNoTransform) return headers;
  const cacheControl = headerString(headers["cache-control"]);
  if (!cacheControl) return headers;
  const directives = cacheControl
    .split(",")
    .map((directive) => directive.trim())
    .filter((directive) => directive.toLowerCase() !== "no-transform");
  if (directives.length === 0) delete headers["cache-control"];
  else headers["cache-control"] = directives.join(", ");
  return headers;
}

function headerString(value: string | string[] | undefined): string {
  return Array.isArray(value) ? value.join(", ") : (value ?? "");
}

function hasNoTransformDirective(value: string | string[] | undefined): boolean {
  return headerString(value)
    .split(",")
    .some((directive) => directive.trim().toLowerCase() === "no-transform");
}

function isHtmlContentType(value: string | string[] | undefined): boolean {
  return headerString(value).toLowerCase().startsWith("text/html");
}

function injectRuntimeHtml(document: Buffer): Buffer {
  const source = document.toString("utf8");
  const script = '<script src="https://static.cloudflareinsights.com/beacon.min.js"></script>';
  const closingHead = /<\/head\s*>/i.exec(source);
  const transformed = closingHead
    ? `${source.slice(0, closingHead.index)}${script}${source.slice(closingHead.index)}`
    : `${source}${script}`;
  return Buffer.from(transformed, "utf8");
}

function runtimeRequestKind(pathname: string, method: string): RuntimeRequestKind | null {
  if (!pathname.startsWith(runtimePrefix)) return null;
  const tokenAndPath = pathname.slice(runtimePrefix.length);
  const separator = tokenAndPath.indexOf("/");
  const requestPath = separator < 0 ? "" : tokenAndPath.slice(separator + 1);
  if (requestPath === "") return "runtime-entry";
  if (requestPath === "_kandev/host-runtime.js") return "runtime-host";
  if (requestPath === "_kandev/v1/context") return "context";
  if (requestPath === "_kandev/v1/events") return "events";
  if (requestPath.startsWith("_kandev/v1/data/") || requestPath.startsWith("_kandev/v1/state")) {
    return method === "GET" ? "data" : "write";
  }
  return "runtime-asset";
}

function hasAuthenticationCookie(cookieHeader: string | undefined): boolean {
  return new RegExp(`(?:^|;\\s*)${CANVAS_PROXY_COOKIE_NAME}=valid(?:;|$)`).test(cookieHeader ?? "");
}

function createCertificate(): {
  directory: string;
  keyPath: string;
  certPath: string;
  hostname: string;
  port: number;
} {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-canvas-auth-proxy-cert-"));
  const keyPath = path.join(directory, "key.pem");
  const certPath = path.join(directory, "cert.pem");
  const configPath = path.join(directory, "openssl.cnf");
  fs.writeFileSync(
    configPath,
    [
      "[req]",
      "distinguished_name = req_distinguished_name",
      "x509_extensions = v3_req",
      "prompt = no",
      "[req_distinguished_name]",
      "CN = 127.0.0.1",
      "[v3_req]",
      "subjectAltName = @alt_names",
      "[alt_names]",
      "IP.1 = 127.0.0.1",
      "DNS.1 = localhost",
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
  return { directory, keyPath, certPath, hostname: "127.0.0.1", port: 0 };
}

function listen(server: TLSServer): Promise<number> {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("canvas authenticated proxy did not receive a port"));
        return;
      }
      resolve(address.port);
    });
  });
}

function closeServer(
  server: TLSServer,
  sockets: Set<Socket>,
  upstreamRequests: Set<ClientRequest>,
  upstreamResponses: Set<IncomingMessage>,
): Promise<void> {
  return new Promise((resolve, reject) => {
    for (const request of upstreamRequests) request.destroy();
    for (const response of upstreamResponses) response.destroy();
    for (const socket of sockets) socket.destroy();
    server.close((error) => (error ? reject(error) : resolve()));
  });
}
