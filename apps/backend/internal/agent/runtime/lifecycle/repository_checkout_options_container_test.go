package lifecycle

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestRepositoryCheckoutOptionsContainer exercises the shipped shell/Git sink
// inside the same image used by executor E2E tests. No host checkout is mounted.
func TestRepositoryCheckoutOptionsContainer(t *testing.T) {
	if os.Getenv("KANDEV_E2E_CONTAINERS") != "1" {
		t.Skip("requires explicit container test run")
	}
	options := &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadOnDemand, SparseDirectories: []string{"app"}}
	prepare, err := checkoutOptionsPrepareScript(DefaultPrepareScript("local_docker"), options)
	if err != nil {
		t.Fatal(err)
	}
	prepare = strings.NewReplacer("{{repository.clone_url}}", "http://127.0.0.1:18473/origin.git", "{{workspace.path}}", "/tmp/checkout", "{{repository.branch}}", "main", "{{repository.setup_script}}", ":", "{{git.identity_setup}}", ":", "{{github.auth_setup}}", ":").Replace(prepare)
	setup := `set -eu
export GIT_CONFIG_GLOBAL=/tmp/test-gitconfig
mkdir -p /tmp/source/app /tmp/source/other
cd /tmp/source
git init -b main
git config user.name Fixture
git config user.email fixture@example.test
git config uploadpack.allowFilter true
printf app > app/file.txt
printf other > other/file.txt
printf historical > old.txt
printf revoked > revoked.txt
git add .
git -c commit.gpgsign=false commit -m history
historical_blob=$(git rev-parse HEAD:old.txt)
revoked_blob=$(git rev-parse HEAD:revoked.txt)
git rm old.txt revoked.txt
git -c commit.gpgsign=false commit -m remove
git clone --bare /tmp/source /tmp/origin.git
git -C /tmp/origin.git config uploadpack.allowFilter true
git -C /tmp/origin.git config uploadpack.allowAnySHA1InWant true
cat > /tmp/git-server.cjs <<'JS'
const http = require('node:http');
const fs = require('node:fs');
const { spawn } = require('node:child_process');
const auth = 'Basic ' + Buffer.from('fixture:fixture-secret').toString('base64');
http.createServer((req,res) => {
 if (fs.existsSync('/tmp/revoked') || req.headers.authorization !== auth) { res.writeHead(401, {'WWW-Authenticate':'Basic realm="fixture"'}); res.end(); return; }
 const url = new URL(req.url,'http://localhost');
 const child=spawn('git',['http-backend'],{env:{...process.env,GIT_PROJECT_ROOT:'/tmp',GIT_HTTP_EXPORT_ALL:'1',PATH_INFO:url.pathname,QUERY_STRING:url.search.slice(1),REQUEST_METHOD:req.method,CONTENT_TYPE:req.headers['content-type'] || '',SERVER_PROTOCOL:'HTTP/1.1'}});
 req.pipe(child.stdin);const chunks=[];child.stdout.on('data',chunk=>chunks.push(chunk));
 child.on('close',()=>{const out=Buffer.concat(chunks);const split=out.indexOf('\r\n\r\n');if(split<0){res.writeHead(500);res.end();return;}for(const line of out.subarray(0,split).toString().split('\r\n')){const i=line.indexOf(':');if(i>0){const key=line.slice(0,i);const value=line.slice(i+1).trim();if(key.toLowerCase()==='status')res.statusCode=parseInt(value);else res.setHeader(key,value);}}res.end(out.subarray(split+4));});
}).listen(18473,'127.0.0.1',()=>fs.writeFileSync('/tmp/git-ready','ready'));
JS
node /tmp/git-server.cjs >/tmp/git-server.log 2>&1 &
server_pid=$!
for i in $(seq 1 50); do [ -f /tmp/git-ready ] && break; sleep 0.1; done
test -f /tmp/git-ready
cat > /tmp/initial-helper <<'SH'
#!/bin/sh
printf 'username=fixture\npassword=fixture-secret\n'
SH
chmod +x /tmp/initial-helper
export GIT_TERMINAL_PROMPT=0 GIT_CONFIG_COUNT=3
export GIT_CONFIG_KEY_0=credential.helper GIT_CONFIG_VALUE_0=
export GIT_CONFIG_KEY_1=credential.http://127.0.0.1:18473.helper GIT_CONFIG_VALUE_1='!/tmp/initial-helper'
export GIT_CONFIG_KEY_2=credential.useHttpPath GIT_CONFIG_VALUE_2=true
`
	verify := `
test -f /tmp/checkout/app/file.txt
test ! -e /tmp/checkout/other/file.txt
test "$(git -C /tmp/checkout rev-parse --is-shallow-repository)" = false
! git -C /tmp/checkout cat-file --batch-all-objects --batch-check='%(objectname)' | grep -Fx "$historical_blob"
cp /tmp/initial-helper /tmp/execution-helper
rm /tmp/initial-helper
export GIT_CONFIG_VALUE_1='!/tmp/execution-helper'
test "$(git -C /tmp/checkout cat-file blob "$historical_blob")" = historical
! git -C /tmp/checkout config --local --list | grep -F fixture-secret
touch /tmp/revoked
if git -C /tmp/checkout cat-file blob "$revoked_blob" >/tmp/revoked-output 2>&1; then exit 1; fi
! grep -F fixture-secret /tmp/revoked-output
kill "$server_pid"
printf 'checkout-container-passed\n'
`
	cmd := exec.Command("docker", "run", "--rm", "-i", "--entrypoint", "bash", "kandev-agent:e2e", "-e", "-s")
	cmd.Stdin = strings.NewReader(setup + "\n" + prepare + "\n" + verify)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "checkout-container-passed") {
		t.Fatalf("container checkout: %v\n%s", err, out)
	}
}
