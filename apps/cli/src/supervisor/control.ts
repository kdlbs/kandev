import net from "node:net";
import fs from "node:fs";

export type ControlRequest = {
  action: "restart";
};

export type ControlResponse = {
  accepted: boolean;
  message: string;
};

export type ControlServer = {
  close: () => Promise<void>;
};

export function startControlServer(
  socket: string,
  onRestart: () => Promise<void>,
): Promise<ControlServer> {
  let restartInProgress = false;
  const scheduleRestart = (): boolean => {
    if (restartInProgress) return false;
    restartInProgress = true;
    setTimeout(() => {
      void onRestart().finally(() => {
        restartInProgress = false;
      });
    }, 0);
    return true;
  };

  if (process.platform !== "win32" && fs.existsSync(socket)) {
    fs.unlinkSync(socket);
  }
  const server = net.createServer((conn) => {
    let buf = "";
    conn.setEncoding("utf8");
    conn.on("data", (chunk) => {
      buf += chunk;
      if (!buf.includes("\n")) return;
      const line = buf.slice(0, buf.indexOf("\n"));
      void handleControlLine(line, scheduleRestart)
        .then((resp) => {
          conn.end(`${JSON.stringify(resp)}\n`);
        })
        .catch((err) => {
          conn.end(
            `${JSON.stringify({
              accepted: false,
              message: err instanceof Error ? err.message : String(err),
            })}\n`,
          );
        });
    });
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(socket, () => {
      server.off("error", reject);
      resolve({
        close: () =>
          new Promise<void>((closeResolve, closeReject) => {
            server.close((err) => {
              if (process.platform !== "win32" && fs.existsSync(socket)) {
                fs.unlinkSync(socket);
              }
              if (err) closeReject(err);
              else closeResolve();
            });
          }),
      });
    });
  });
}

export function requestRestart(socket: string): Promise<ControlResponse> {
  return new Promise((resolve, reject) => {
    const conn = net.createConnection(socket);
    let buf = "";
    conn.setEncoding("utf8");
    conn.on("connect", () => {
      conn.write(`${JSON.stringify({ action: "restart" satisfies ControlRequest["action"] })}\n`);
    });
    conn.on("data", (chunk) => {
      buf += chunk;
    });
    conn.on("error", reject);
    conn.on("end", () => {
      try {
        resolve(JSON.parse(buf.trim()) as ControlResponse);
      } catch (err) {
        reject(err);
      }
    });
  });
}

async function handleControlLine(
  line: string,
  scheduleRestart: () => boolean,
): Promise<ControlResponse> {
  const req = JSON.parse(line) as ControlRequest;
  if (req.action !== "restart") {
    return { accepted: false, message: `unsupported action ${String(req.action)}` };
  }
  if (!scheduleRestart()) {
    return { accepted: false, message: "Restart already in progress" };
  }
  return { accepted: true, message: "Restart accepted" };
}
