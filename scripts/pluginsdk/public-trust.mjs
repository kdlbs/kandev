import { execFile } from "node:child_process";
import { promisify } from "node:util";
const execute = promisify(execFile);
export async function validatePublicTrust({
  run = execute,
  env = process.env,
  cwd = process.cwd(),
} = {}) {
  const options = { env, cwd, timeout: 30_000 };
  const keys = [
    "GOPROXY",
    "GOSUMDB",
    "GOPRIVATE",
    "GONOPROXY",
    "GONOSUMDB",
    "GOINSECURE",
    "GOFLAGS",
  ];
  const go = JSON.parse(
    (await run("go", ["env", "-json", ...keys], options)).stdout,
  );
  for (const key of keys.slice(2))
    if (go[key]) throw new Error(`public trust rejects effective ${key}`);
  if (
    !["https://proxy.golang.org", "https://proxy.golang.org,direct"].includes(
      go.GOPROXY,
    ) ||
    go.GOSUMDB !== "sum.golang.org"
  )
    throw new Error(
      "public trust requires the public Go proxy and checksum database",
    );
  const npm = JSON.parse(
    (await run("npm", ["config", "list", "--json"], options)).stdout,
  );
  for (const [key, value] of Object.entries(npm)) {
    if (
      (key === "registry" || key.endsWith(":registry")) &&
      value?.replace(/\/$/, "") !== "https://registry.npmjs.org"
    )
      throw new Error(
        "public trust rejects an effective npm registry override",
      );
  }
  if (npm["strict-ssl"] === false || env.NODE_TLS_REJECT_UNAUTHORIZED === "0")
    throw new Error("public trust requires TLS verification");
}
