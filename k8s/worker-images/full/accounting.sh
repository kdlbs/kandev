#!/bin/sh
# Test-only bounded nested process. The caller must delete its owned Pod even
# when a cgroup assertion fails; the process also exits after 90 seconds.
set -eu
work=$(mktemp -d /workspace/full-accounting.XXXXXX)
trap 'rm -rf "$work"' EXIT
cd "$work"
cat > main.go <<'GO'
package main
import ("runtime";"time")
func main(){
 data:=make([]byte,32*1024*1024)
 for i:=range data {data[i]=1}
 until:=time.Now().Add(90*time.Second)
 for time.Now().Before(until) {for i:=range data {data[i]++};runtime.KeepAlive(data)}
}
GO
CGO_ENABLED=0 GOMAXPROCS=2 timeout 120 go build -p 1 -o accounting main.go
printf 'FROM scratch\nCOPY accounting /accounting\nENTRYPOINT ["/accounting"]\n' > Dockerfile
timeout 120 docker build --network=none -t full-worker-accounting . >&2
# Its private daemon/image is discarded with the test Pod.
timeout 10 docker run -d --name full-worker-accounting --memory=128m --cpus=0.25 full-worker-accounting
