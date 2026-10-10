"""Compare setup configurations serially without changing permanent selection.

Run from the repository root with an output directory and optional selection JSON.
The selection contains explicit `node` and `english-browser` file lists.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

root = Path.cwd()
web = root / "apps/web"
out = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(tempfile.mkdtemp(prefix="kandev-ci-setup-"))
out = out.resolve()
out.mkdir(parents=True, exist_ok=True)
selection_path = (Path(sys.argv[2]) if len(sys.argv) > 2
                  else Path(__file__).parent / "setup-selection.json")
selection = json.loads(selection_path.read_text())
config = web / ".ci-investigation-temporary.config.ts"
if config.exists():
    raise SystemExit("Another setup experiment owns the temporary config")
config.write_text('''import base from "./vitest.config";
const nodeFiles = ''' + json.dumps(selection["node"]) + ';\nconst browserFiles = ' + json.dumps(selection["english-browser"]) + ';\n' + '''
const candidate = process.env.CI_SETUP_EXPERIMENT === "candidate";
const projects = (base.test!.projects! as any[]).map(p => ({
  ...p,
  test: {
    ...p.test,
    include: p.test.name === "node" ? (candidate ? nodeFiles : [])
      : p.test.name === "browser" ? (candidate ? browserFiles : [])
      : (candidate ? [] : [...nodeFiles, ...browserFiles]),
    exclude: [],
  },
})).filter(p => p.test.include.length);
export default { ...base, test: { ...base.test, maxWorkers: 2, projects } };
''')
results = []
try:
    for sample in range(1, 4):
        modes = ["baseline", "candidate"] if sample % 2 else ["candidate", "baseline"]
        for mode in modes:
            env = dict(os.environ, NODE_ENV="production", CI_SETUP_EXPERIMENT=mode)
            env.pop("CI", None)
            log = out / f"bench-{mode}-{sample}.log"
            report = out / f"bench-{mode}-{sample}.json"
            start = time.monotonic()
            with log.open("w") as handle:
                result = subprocess.run(
                    ["pnpm", "exec", "vitest", "run", "--config", str(config),
                     "--reporter=default", "--reporter=json", "--outputFile", str(report)],
                    cwd=web, env=env, stdout=handle, stderr=subprocess.STDOUT,
                )
            record = {"mode": mode, "sample": sample,
                      "seconds": round(time.monotonic() - start, 3), "exit": result.returncode}
            results.append(record)
            print(json.dumps(record), flush=True)
            if result.returncode:
                print(log.read_text()[-5000:], flush=True)
                raise SystemExit(result.returncode)
finally:
    config.unlink(missing_ok=True)
    (out / "benchmark-results.json").write_text(json.dumps(results, indent=2) + "\n")
