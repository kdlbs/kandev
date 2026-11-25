use super::*;
use serde_json::{json, Value};
use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::time::{SystemTime, UNIX_EPOCH};

struct FixtureDir(PathBuf);
impl Drop for FixtureDir {
    fn drop(&mut self) {
        if thread::panicking() {
            eprintln!(
                "Fixture sshd: {}",
                fs::read_to_string(self.0.join("sshd.log")).unwrap_or_default()
            );
        }
        let _ = fs::remove_dir_all(&self.0);
    }
}
struct OwnedProcess(Child);
impl Drop for OwnedProcess {
    fn drop(&mut self) {
        terminate_child(&mut self.0);
    }
}
struct RunningBackend {
    state: BackendState,
    port: u16,
}
impl Drop for RunningBackend {
    fn drop(&mut self) {
        self.state.stop();
    }
}

fn start_native_backend(runtime: &Path, root: &Path) -> RunningBackend {
    let state = BackendState::default();
    let port = pick_loopback_port().expect("disposable backend port");
    let mut inherited: BTreeMap<OsString, OsString> = env::vars_os()
        .filter(|(key, _)| {
            !key.to_string_lossy().starts_with("KANDEV_")
                && key != "SSH_AUTH_SOCK"
                && key != "SSH_AGENT_PID"
        })
        .collect();
    for (key, value) in [
        ("KANDEV_HOME_DIR", root.join("home").into_os_string()),
        (
            "KANDEV_DATABASE_PATH",
            root.join("home/data/kandev.db").into_os_string(),
        ),
        (
            "KANDEV_INTERNAL_CONFIG_FILE",
            root.join("config.yaml").into_os_string(),
        ),
        ("KANDEV_E2E_MOCK", OsString::from("true")),
        (
            "KANDEV_FEATURES_SSH_CONNECTION_MANAGER",
            OsString::from("true"),
        ),
        ("KANDEV_FEATURES_AUTH", OsString::from("false")),
        (
            DESKTOP_HEALTH_TOKEN_ENV,
            OsString::from("disposable-desktop-ssh-fixture"),
        ),
    ] {
        inherited.insert(OsString::from(key), value);
    }
    let spec = build_backend_command(runtime, port, inherited, Some(root)).expect("native command");
    assert!(!spec.env.contains_key(OsStr::new("SSH_AUTH_SOCK")));
    let mut child = spawn_backend_command(&spec).expect("spawn native backend");
    let readers = capture_child_output(&mut child, state.startup_output.clone());
    *state.startup_output_readers.lock().unwrap() = readers;
    *state.child.lock().unwrap() = Some(child);
    let running = RunningBackend { state, port };
    wait_for_backend(
        port,
        &running.state,
        Duration::from_secs(60),
        "disposable-desktop-ssh-fixture",
    )
    .expect("native health");
    let deadline = Instant::now() + Duration::from_secs(60);
    while !ready_ready(port) {
        assert!(
            Instant::now() < deadline,
            "native backend readiness timed out"
        );
        assert!(
            running.state.child_exit_message().unwrap().is_none(),
            "native backend exited"
        );
        thread::sleep(Duration::from_millis(100));
    }
    running
}

fn request(port: u16, path: &str, body: Option<Value>) -> Value {
    let mut command = Command::new("curl");
    command.args([
        "--silent",
        "--show-error",
        "--fail-with-body",
        "--max-time",
        "20",
        "-H",
        "Content-Type: application/json",
    ]);
    if let Some(body) = body {
        command.args(["--data", &body.to_string()]);
    }
    let response = command
        .arg(format!("http://127.0.0.1:{port}/api/v1/ssh/managed{path}"))
        .output()
        .expect("fixture HTTP request");
    assert!(
        response.status.success(),
        "fixture request failed: {path}: {}",
        String::from_utf8_lossy(&response.stdout)
    );
    serde_json::from_slice(&response.stdout).expect("fixture JSON response")
}

fn start_fixture_sshd(root: &Path, port: u16) -> OwnedProcess {
    let host_key = root.join("host_key");
    assert!(Command::new("ssh-keygen")
        .args(["-q", "-t", "ed25519", "-N", "", "-f"])
        .arg(&host_key)
        .status()
        .unwrap()
        .success());
    fs::write(root.join("authorized_keys"), "").unwrap();
    fs::set_permissions(
        root.join("authorized_keys"),
        fs::Permissions::from_mode(0o600),
    )
    .unwrap();
    let config = format!("ListenAddress 127.0.0.1\nPort {port}\nHostKey {}\nPidFile {}\nAuthorizedKeysFile {}\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\nStrictModes yes\nLogLevel VERBOSE\n", host_key.display(), root.join("sshd.pid").display(), root.join("authorized_keys").display());
    fs::write(root.join("sshd_config"), config).unwrap();
    let log = fs::File::create(root.join("sshd.log")).unwrap();
    let child = Command::new(env::var_os("KANDEV_TEST_SSHD").expect("absolute fixture sshd path"))
        .args(["-D", "-e", "-f"])
        .arg(root.join("sshd_config"))
        .stdout(Stdio::null())
        .stderr(log)
        .spawn()
        .expect("isolated sshd");
    let mut owned = OwnedProcess(child);
    let deadline = Instant::now() + Duration::from_secs(10);
    while TcpStream::connect((LOOPBACK_HOST, port)).is_err() {
        assert!(
            owned.0.try_wait().unwrap().is_none(),
            "isolated sshd failed: {}",
            fs::read_to_string(root.join("sshd.log")).unwrap()
        );
        assert!(Instant::now() < deadline, "isolated sshd timed out");
        thread::sleep(Duration::from_millis(50));
    }
    owned
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-001.3, AC-EXECUTORS-SSH-CONNECTIONS-005.5
#[test]
#[ignore = "requires built native runtime, curl, and an absolute KANDEV_TEST_SSHD path"]
fn native_desktop_managed_ssh_survives_restart_without_agent() {
    let runtime = PathBuf::from(
        env::var_os("KANDEV_TEST_DESKTOP_RUNTIME_DIR").expect("prepared native runtime"),
    );
    let parent = PathBuf::from(
        env::var_os("KANDEV_TEST_SSH_FIXTURE_PARENT")
            .expect("private fixture parent with secure ancestors"),
    );
    let root = parent.join(format!(
        "kandev-native-ssh-{}-{}",
        std::process::id(),
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    ));
    fs::create_dir(&root).unwrap();
    fs::set_permissions(&root, fs::Permissions::from_mode(0o700)).unwrap();
    let fixture = FixtureDir(root);
    fs::write(
        fixture.0.join("config.yaml"),
        "features:\n  sshConnectionManager: true\n",
    )
    .unwrap();
    let ssh_port = pick_loopback_port().unwrap();
    let _sshd = start_fixture_sshd(&fixture.0, ssh_port);
    let backend = start_native_backend(&runtime, &fixture.0);
    let key = request(
        backend.port,
        "/identities",
        Some(
            json!({"request_id":"5cc4e16a-14df-47a0-b60e-c5c1086d6d2b", "kind":"generate", "label":"Desktop fixture"}),
        ),
    );
    fs::write(
        fixture.0.join("authorized_keys"),
        key["public_key"].as_str().unwrap(),
    )
    .unwrap();
    let user = String::from_utf8(Command::new("id").arg("-un").output().unwrap().stdout).unwrap();
    let route = request(
        backend.port,
        "/connections",
        Some(
            json!({"request_id":"5cc4e16a-14df-47a0-b60e-c5c1086d6d2c", "expected_version":0,"connection":{"name":"Desktop fixture","host":"127.0.0.1","port":ssh_port,"user":user.trim(),"identity_id":key["id"]}}),
        ),
    );
    let path = format!(
        "/connections/{}",
        route["destination"]["id"].as_str().unwrap()
    );
    let observed = request(
        backend.port,
        &format!("{path}/observe"),
        Some(json!({"expected_stamp":route["stamp"]})),
    );
    let fingerprint = Command::new("ssh-keygen")
        .args(["-lf"])
        .arg(fixture.0.join("host_key.pub"))
        .output()
        .unwrap();
    let expected = String::from_utf8(fingerprint.stdout).unwrap();
    assert_eq!(
        observed["fingerprint"].as_str().unwrap(),
        expected.split_whitespace().nth(1).unwrap()
    );
    let trusted = request(
        backend.port,
        &format!("{path}/trust"),
        Some(
            json!({"expected_stamp":observed["stamp"],"receipt":observed["receipt"],"fingerprint":observed["fingerprint"]}),
        ),
    );
    request(
        backend.port,
        &format!("{path}/test"),
        Some(json!({"expected_stamp":trusted["stamp"]})),
    );
    drop(backend);
    let restarted = start_native_backend(&runtime, &fixture.0);
    let saved = request(restarted.port, &path, None);
    assert_eq!(saved["stamp"], trusted["stamp"]);
    let tested = request(
        restarted.port,
        &format!("{path}/test"),
        Some(json!({"expected_stamp":saved["stamp"]})),
    );
    assert_eq!(tested["destination"]["verified_route"], saved["stamp"]);
    let restored = request(
        restarted.port,
        &format!("/identities/{}", key["id"].as_str().unwrap()),
        None,
    );
    assert_eq!(restored["public_key"], key["public_key"]);
    assert_eq!(restored["generation"], key["generation"]);
}
