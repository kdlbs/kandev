#!/usr/bin/env bash
set -euo pipefail
mode=${1:---tools}
[[ "$mode" == --tools || "$mode" == --all ]] || { echo 'Usage: smoke.sh --tools|--all' >&2; exit 2; }
[[ $(id -u) == 1000 ]] || { echo 'Run smoke as UID 1000' >&2; exit 1; }
work=$(mktemp -d /workspace/full-worker-smoke.XXXXXX)
project="full-worker-$(basename "$work" | tr '[:upper:].' '[:lower:]-')"
image="$project:smoke"
cleanup() {
  if [[ "$mode" == --all ]]; then
    timeout 30 docker compose -p "$project" -f "$work/compose.yaml" down --volumes --remove-orphans >/dev/null 2>&1 || true
    timeout 15 docker image rm "$image" >/dev/null 2>&1 || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT
cd "$work"
export GOMAXPROCS=2 GOFLAGS=-p=1 CARGO_BUILD_JOBS=2
printf 'module fullworker\n\ngo 1.26.0\n' > go.mod
cat > main.go <<'GO'
package main

import (
 "fmt"
 "io"
 "net/http"
 "os"
)

func validInput(data []byte) bool { return string(data)=="workspace-input\n" }

func main() {
 if len(os.Args)>1 && os.Args[1]=="fetch" {
  response,err:=http.Get("https://registry.npmjs.org/playwright-core/-/playwright-core-1.61.1.tgz")
  if err!=nil {panic(err)}
  defer func(){ _ = response.Body.Close() }()
  if response.StatusCode!=200 {panic(response.Status)}
  n,err:=io.Copy(io.Discard,response.Body)
  if err!=nil || n<100000 {panic("incomplete HTTPS transfer")}
  fmt.Println("https-transfer-ok")
  return
 }
 data,err:=os.ReadFile("/input/source.txt");if err!=nil {panic(err)}
 if !validInput(data) {panic("wrong bind source")}
 if err:=os.WriteFile("/input/forbidden",data,0600);err==nil {panic("read-only bind accepted write")}
 if err:=os.WriteFile("/output/result.txt",data,0600);err!=nil {panic(err)}
 fmt.Println("compose-bind-ok")
}
GO
cat > main_test.go <<'GO'
package main
import "testing"
func TestSource(t *testing.T) {for _,test:=range []struct{input string;valid bool}{{"workspace-input\n",true},{"workspace-input",false},{"",false}} {if validInput([]byte(test.input))!=test.valid {t.Fatalf("input %q",test.input)}}}
GO
timeout 120 go test ./...
printf 'version: "2"\n' > .golangci.yml
timeout 120 golangci-lint run ./...
CGO_ENABLED=0 timeout 120 go build -o bind-smoke .
printf '{"name":"full-worker-smoke","version":"1.0.0","scripts":{"test":"node --test source.test.cjs"}}\n' > package.json
printf "const {test}=require('node:test');const assert=require('node:assert/strict');test('source',()=>assert.equal(6*7,42));\n" > source.test.cjs
timeout 30 pnpm test
python3 -m venv .venv
printf 'import unittest\nclass Source(unittest.TestCase):\n def test_source(self): self.assertEqual(6*7,42)\n' > test_source.py
timeout 30 .venv/bin/python -m unittest test_source.py
printf '#include <stdio.h>\nint main(void){puts("native-ok");return 0;}\n' > native.c
cc -Wall -Werror native.c -o native
test "$(./native)" = native-ok
mkdir rust
printf '[package]\nname="full-worker-smoke"\nversion="0.1.0"\nedition="2021"\n' > rust/Cargo.toml
mkdir rust/src
printf '#[test] fn source(){assert_eq!(6*7,42);}\n' > rust/src/lib.rs
timeout 120 cargo test --offline --manifest-path rust/Cargo.toml
node <<'JS'
const {chromium}=require('/opt/full-worker/playwright-core');
(async()=>{
 const browser=await chromium.launch({headless:true,timeout:30000,args:['--disable-dev-shm-usage']});
 try {const page=await browser.newPage();await page.setContent(`<button onclick="this.textContent='clicked'">run</button>`);await page.getByRole('button').click();if(await page.getByRole('button').textContent()!=='clicked')throw Error('browser interaction failed');await page.screenshot({path:'browser.png'});}
 finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
JS
test -s browser.png
printf 'source-browser-ok\n'
if [[ "$mode" == --all ]]; then
  timeout 10 docker info >/dev/null
  cat > Dockerfile <<'DOCKER'
FROM scratch
COPY bind-smoke /bind-smoke
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/bind-smoke"]
DOCKER
  cp /etc/ssl/certs/ca-certificates.crt .
  mkdir input output
  printf 'workspace-input\n' > input/source.txt
  timeout 120 docker build --network=none -t "$image" .
  timeout 60 docker run --rm --memory=128m --cpus=0.5 "$image" fetch
  cat > compose.yaml <<COMPOSE
services:
  bind:
    image: $image
    user: "1000:1000"
    mem_limit: 128m
    cpus: 0.5
    volumes:
      - type: bind
        source: $work/input
        target: /input
        read_only: true
      - type: bind
        source: $work/output
        target: /output
COMPOSE
  timeout 60 docker compose -p "$project" -f compose.yaml run --rm bind
  cmp input/source.txt output/result.txt
  test ! -e input/forbidden
  timeout 30 docker compose -p "$project" -f compose.yaml down --volumes --remove-orphans
  test -z "$(docker ps -aq --filter "label=com.docker.compose.project=$project")"
  printf 'source-browser-docker-compose-ok\n' > /workspace/full-worker-result.txt
else
  printf 'source-browser-ok\n' > /workspace/full-worker-result.txt
fi
go version
rustc --version
node --version
pnpm --version
python3 --version
golangci-lint version
docker --version
docker buildx version
docker compose version
cat /workspace/full-worker-result.txt
