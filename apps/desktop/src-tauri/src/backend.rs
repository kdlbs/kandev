use serde::{Deserialize, Serialize};
use std::{
    collections::BTreeMap,
    env,
    ffi::{OsStr, OsString},
    fs::{self, OpenOptions},
    io::{ErrorKind, Read, Write},
    net::{TcpListener, TcpStream, ToSocketAddrs},
    path::{Path, PathBuf},
    process::{Child, Command, Stdio},
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc, Mutex,
    },
    thread,
    time::{Duration, Instant},
};
use url::Url;

#[cfg(feature = "desktop-runtime")]
use tauri::{AppHandle, Manager, State, WebviewWindow};

const HEALTH_TIMEOUT: Duration = Duration::from_secs(60);
// Keep this above the Go launcher's graceful stop and forced-exit bounds.
const DESKTOP_LAUNCHER_SHUTDOWN_TIMEOUT: Duration = Duration::from_secs(80);
const LOOPBACK_HOST: &str = "127.0.0.1";
const DEFAULT_DESKTOP_PORT: u16 = 38430;
const DESKTOP_PORT_ENV: &str = "KANDEV_DESKTOP_PORT";
const DESKTOP_HEALTH_TOKEN_ENV: &str = "KANDEV_DESKTOP_HEALTH_TOKEN";
const DESKTOP_NATIVE_NOTIFICATIONS_ENV: &str = "KANDEV_DESKTOP_NATIVE_NOTIFICATIONS";
const DESKTOP_RUNTIME_ENV: &str = "KANDEV_DESKTOP_RUNTIME";
const LAUNCHER_PARENT_PID_ENV: &str = "KANDEV_LAUNCHER_PARENT_PID";
const TEMPORARY_TEST_ARGUMENT: &str = "--kandev-temporary-test";
const INTERNAL_CONFIG_FILE_ENV: &str = "KANDEV_INTERNAL_CONFIG_FILE";
const DESKTOP_HEALTH_TOKEN_HEADER: &str = "x-kandev-desktop-health-token";
const STARTUP_OUTPUT_LIMIT: usize = 12 * 1024;
const STARTUP_CONFLICT_MARKER_PREFIX: &[u8] = b"KANDEV_DESKTOP_CONFLICT_V1 ";
const STARTUP_CONFLICT_LINE_LIMIT: usize = 16 * 1024;
const HEALTH_READY_SETTLE: Duration = Duration::from_millis(100);
const REMOTE_HELPER_MANIFEST_SCHEMA_VERSION: u32 = 1;
const REMOTE_HELPER_MANIFEST_MAX_BYTES: u64 = 64 * 1024;
const REMOTE_HELPER_MAX_BYTES: u64 = 256 * 1024 * 1024;
const REMOTE_AGENTCTL_HELPERS: [(&str, &str); 4] = [
    ("agentctl-linux-amd64", "agentctl linux/amd64 helper"),
    ("agentctl-linux-arm64", "agentctl linux/arm64 helper"),
    ("agentctl-darwin-arm64", "agentctl darwin/arm64 helper"),
    ("agentctl-darwin-amd64", "agentctl darwin/amd64 helper"),
];
const REMOTE_HELPER_MANIFEST_RECORDS: [(&str, &str); 4] = [
    ("linux/amd64", "agentctl-linux-amd64.gz"),
    ("linux/arm64", "agentctl-linux-arm64.gz"),
    ("darwin/amd64", "agentctl-darwin-amd64.gz"),
    ("darwin/arm64", "agentctl-darwin-arm64.gz"),
];

#[derive(Clone)]
pub struct BackendState {
    child: Arc<Mutex<Option<Child>>>,
    startup_output: Arc<Mutex<StartupOutput>>,
    startup_output_readers: Arc<Mutex<Vec<std::sync::mpsc::Receiver<()>>>>,
    startup_conflict: Arc<Mutex<Option<StartupConflict>>>,
    shutdown_started: Arc<AtomicBool>,
    owned_origin: Arc<Mutex<Option<String>>>,
    temporary_test: bool,
    temporary_home: Arc<Mutex<Option<TemporaryHome>>>,
    temporary_home_error: Arc<Mutex<Option<String>>>,
    retain_temporary_home: Arc<AtomicBool>,
    backend_ready: Arc<AtomicBool>,
}

impl Default for BackendState {
    fn default() -> Self {
        Self {
            child: Arc::new(Mutex::new(None)),
            startup_output: Arc::new(Mutex::new(StartupOutput::default())),
            startup_output_readers: Arc::new(Mutex::new(Vec::new())),
            startup_conflict: Arc::new(Mutex::new(None)),
            shutdown_started: Arc::new(AtomicBool::new(false)),
            owned_origin: Arc::new(Mutex::new(None)),
            temporary_test: false,
            temporary_home: Arc::new(Mutex::new(None)),
            temporary_home_error: Arc::new(Mutex::new(None)),
            retain_temporary_home: Arc::new(AtomicBool::new(false)),
            backend_ready: Arc::new(AtomicBool::new(false)),
        }
    }
}

impl BackendState {
    pub fn temporary_test_instance() -> Self {
        let mut state = Self::default();
        state.temporary_test = true;
        match TemporaryHome::create() {
            Ok(home) => {
                *state
                    .temporary_home
                    .lock()
                    .expect("temporary home mutex poisoned") = Some(home);
            }
            Err(err) => {
                *state
                    .temporary_home_error
                    .lock()
                    .expect("temporary home error mutex poisoned") = Some(err);
            }
        }
        state
    }

    pub fn begin_shutdown(&self) -> bool {
        !self.shutdown_started.swap(true, Ordering::SeqCst)
    }

    pub fn stop(&self) {
        self.shutdown_started.store(true, Ordering::SeqCst);
        self.clear_owned_origin();
        let child = self
            .child
            .lock()
            .expect("backend child mutex poisoned")
            .take();
        let clean_stop = child
            .map(|mut child| terminate_child(&mut child))
            .unwrap_or(false);
        self.cleanup_temporary_home_after_stop(clean_stop);
    }

    fn is_shutdown_started(&self) -> bool {
        self.shutdown_started.load(Ordering::SeqCst)
    }

    pub fn set_owned_origin(&self, backend_url: &str) -> Result<(), String> {
        let origin = loopback_origin(backend_url)?;
        *self
            .owned_origin
            .lock()
            .expect("desktop origin mutex poisoned") = Some(origin);
        Ok(())
    }

    pub fn accepts_url(&self, url: &str) -> bool {
        let expected = self
            .owned_origin
            .lock()
            .expect("desktop origin mutex poisoned")
            .clone();
        expected.is_some_and(|origin| same_origin(&origin, url))
    }

    #[cfg(feature = "desktop-runtime")]
    pub fn require_owned_origin(&self, webview: &WebviewWindow) -> Result<(), String> {
        if !self.has_live_child() {
            self.clear_owned_origin();
            return Err("desktop backend is no longer running".to_string());
        }
        let url = webview
            .url()
            .map_err(|err| format!("could not read desktop WebView URL: {err}"))?;
        if self.accepts_url(url.as_str()) {
            Ok(())
        } else {
            Err("desktop command is only available from the Kandev backend".to_string())
        }
    }

    fn clear_owned_origin(&self) {
        *self
            .owned_origin
            .lock()
            .expect("desktop origin mutex poisoned") = None;
    }

    fn is_temporary_test(&self) -> bool {
        self.temporary_test
    }

    fn can_start_temporary_test(&self, url: &str) -> bool {
        !self.temporary_test
            && self.startup_conflict().is_some()
            && !self.has_live_child()
            && is_local_startup_url(url)
    }

    #[cfg(feature = "desktop-runtime")]
    fn require_conflict_startup(&self, webview: &WebviewWindow) -> Result<(), String> {
        let url = webview
            .url()
            .map_err(|err| format!("Could not read desktop startup URL: {err}"))?;
        if self.can_start_temporary_test(url.as_str()) {
            Ok(())
        } else {
            Err(
                "Temporary test instances can only start from a detected startup conflict"
                    .to_string(),
            )
        }
    }

    pub fn temporary_home_path(&self) -> Option<PathBuf> {
        self.temporary_home
            .lock()
            .expect("temporary home mutex poisoned")
            .as_ref()
            .map(|home| home.path.clone())
    }

    fn temporary_home_for_launch(&self) -> Result<Option<TemporaryHome>, String> {
        if !self.temporary_test {
            return Ok(None);
        }
        if let Some(err) = self
            .temporary_home_error
            .lock()
            .expect("temporary home error mutex poisoned")
            .as_ref()
        {
            return Err(err.clone());
        }
        self.temporary_home
            .lock()
            .expect("temporary home mutex poisoned")
            .clone()
            .map(Some)
            .ok_or_else(|| "Temporary Kandev home is unavailable".to_string())
    }

    fn mark_backend_ready(&self) {
        self.backend_ready.store(true, Ordering::SeqCst);
    }

    fn cleanup_temporary_home_after_stop(&self, clean_stop: bool) -> bool {
        if !self.temporary_test
            || !clean_stop
            || !self.backend_ready.load(Ordering::SeqCst)
            || self.retain_temporary_home.load(Ordering::SeqCst)
        {
            return false;
        }
        let mut home = self
            .temporary_home
            .lock()
            .expect("temporary home mutex poisoned");
        let Some(owned_home) = home.as_ref() else {
            return false;
        };
        if !owned_home.remove_if_owned() {
            return false;
        }
        home.take();
        true
    }

    fn retain_temporary_home(&self) {
        self.retain_temporary_home.store(true, Ordering::SeqCst);
    }

    fn has_live_child(&self) -> bool {
        self.child
            .lock()
            .expect("backend child mutex poisoned")
            .as_mut()
            .is_some_and(|child| matches!(child.try_wait(), Ok(None)))
    }

    fn set_child(&self, mut child: Child) -> bool {
        let mut guard = self.child.lock().expect("backend child mutex poisoned");
        if self.is_shutdown_started() {
            drop(guard);
            terminate_child(&mut child);
            return false;
        }
        *guard = Some(child);
        true
    }

    fn reset_startup_output(&self) {
        self.startup_output
            .lock()
            .expect("startup output mutex poisoned")
            .clear();
        self.startup_output_readers
            .lock()
            .expect("startup output reader mutex poisoned")
            .clear();
        *self
            .startup_conflict
            .lock()
            .expect("startup conflict mutex poisoned") = None;
    }

    fn recent_startup_output(&self) -> Option<String> {
        self.startup_output
            .lock()
            .expect("startup output mutex poisoned")
            .text()
    }

    fn child_exit_message(&self) -> Result<Option<String>, String> {
        let mut guard = self.child.lock().expect("backend child mutex poisoned");
        if let Some(child) = guard.as_mut() {
            if let Some(status) = child.try_wait().map_err(|err| err.to_string())? {
                let output_drained = wait_for_startup_output_readers(
                    std::mem::take(
                        &mut *self
                            .startup_output_readers
                            .lock()
                            .expect("startup output reader mutex poisoned"),
                    ),
                    Duration::from_secs(2),
                );
                let (output, conflict) = {
                    let output = self
                        .startup_output
                        .lock()
                        .expect("startup output mutex poisoned");
                    (
                        output.text(),
                        output_drained.then(|| output.startup_conflict()).flatten(),
                    )
                };
                *self
                    .startup_conflict
                    .lock()
                    .expect("startup conflict mutex poisoned") = conflict;
                return Ok(Some(launcher_exit_message(&status.to_string(), output)));
            }
        }
        Ok(None)
    }

    pub fn startup_conflict(&self) -> Option<StartupConflict> {
        self.startup_conflict
            .lock()
            .expect("startup conflict mutex poisoned")
            .clone()
    }
}

pub fn is_temporary_test_process() -> bool {
    is_temporary_test_args(env::args_os())
}

fn is_temporary_test_args(args: impl IntoIterator<Item = OsString>) -> bool {
    args.into_iter()
        .any(|arg| arg == OsStr::new(TEMPORARY_TEST_ARGUMENT))
}

fn is_local_startup_url(input: &str) -> bool {
    let Ok(url) = Url::parse(input) else {
        return false;
    };
    if url.scheme() == "tauri" {
        return url.host_str() == Some("localhost");
    }
    if url.scheme() != "http" {
        return false;
    }
    if url.host_str() == Some("tauri.localhost") {
        return true;
    }
    cfg!(debug_assertions)
        && url.port() == Some(1420)
        && matches!(url.host_str(), Some("localhost" | "127.0.0.1"))
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TemporaryHome {
    pub path: PathBuf,
    temp_root: PathBuf,
}

impl TemporaryHome {
    fn create() -> Result<Self, String> {
        let temp_root = env::temp_dir().canonicalize().map_err(|err| {
            format!("Could not resolve the operating-system temporary directory: {err}")
        })?;
        let mut created_path = None;
        for _ in 0..8 {
            let mut random = [0_u8; 16];
            getrandom::fill(&mut random)
                .map_err(|err| format!("Could not create a private temporary home name: {err}"))?;
            let path = temp_root.join(format!(
                "kandev-test-{}-{}",
                std::process::id(),
                hex_encode(&random)
            ));
            let mut builder = fs::DirBuilder::new();
            #[cfg(unix)]
            {
                use std::os::unix::fs::DirBuilderExt;
                builder.mode(0o700);
            }
            match builder.create(&path) {
                Ok(()) => {
                    created_path = Some(path);
                    break;
                }
                Err(err) if err.kind() == ErrorKind::AlreadyExists => continue,
                Err(err) => {
                    return Err(format!("Could not create a private temporary home: {err}"))
                }
            }
        }
        let path = created_path.ok_or_else(|| {
            "Could not allocate a unique private temporary Kandev home".to_string()
        })?;
        let canonical_path = match path.canonicalize() {
            Ok(path) if path.parent() == Some(temp_root.as_path()) => path,
            Ok(_) => {
                let _ = fs::remove_dir_all(&path);
                return Err(
                    "The temporary Kandev home is outside the operating-system temporary directory"
                        .to_string(),
                );
            }
            Err(err) => {
                let _ = fs::remove_dir_all(&path);
                return Err(format!("Could not verify the temporary Kandev home: {err}"));
            }
        };
        let config_path = canonical_path.join("config.yaml");
        let mut options = OpenOptions::new();
        options.write(true).create_new(true);
        #[cfg(unix)]
        {
            use std::os::unix::fs::OpenOptionsExt;
            options.mode(0o600);
        }
        let config_file = match options.open(&config_path) {
            Ok(file) => file,
            Err(err) => {
                let _ = fs::remove_dir_all(&canonical_path);
                return Err(format!(
                    "Could not create the temporary Kandev config: {err}"
                ));
            }
        };
        drop(config_file);
        Ok(Self {
            path: canonical_path,
            temp_root,
        })
    }

    fn remove_if_owned(&self) -> bool {
        if self
            .path
            .file_name()
            .is_none_or(|name| !name.to_string_lossy().starts_with("kandev-test-"))
        {
            return false;
        }
        let Ok(metadata) = fs::symlink_metadata(&self.path) else {
            return false;
        };
        if metadata.file_type().is_symlink() || !metadata.is_dir() {
            return false;
        }
        let Ok(canonical_path) = self.path.canonicalize() else {
            return false;
        };
        if canonical_path != self.path || canonical_path.parent() != Some(self.temp_root.as_path())
        {
            return false;
        }
        fs::remove_dir_all(&canonical_path).is_ok()
    }
}

#[derive(Default)]
struct StartupOutput {
    bytes: Vec<u8>,
    conflict_line: Vec<u8>,
    discarding_conflict_line: bool,
    conflict_marker_count: usize,
    conflict: Option<StartupConflict>,
}

impl StartupOutput {
    fn clear(&mut self) {
        self.bytes.clear();
        self.conflict_line.clear();
        self.discarding_conflict_line = false;
        self.conflict_marker_count = 0;
        self.conflict = None;
    }

    fn push(&mut self, stream: &str, chunk: &[u8]) {
        if chunk.is_empty() {
            return;
        }

        if stream == "stderr" {
            self.push_conflict_bytes(chunk);
        }

        self.bytes
            .extend_from_slice(format!("\n[{stream}] ").as_bytes());
        self.bytes.extend_from_slice(chunk);
        if self.bytes.len() > STARTUP_OUTPUT_LIMIT {
            let overflow = self.bytes.len() - STARTUP_OUTPUT_LIMIT;
            self.bytes.drain(0..overflow);
        }
    }

    fn push_conflict_bytes(&mut self, chunk: &[u8]) {
        for byte in chunk {
            if *byte == b'\n' {
                if !self.discarding_conflict_line {
                    self.parse_conflict_line();
                }
                self.conflict_line.clear();
                self.discarding_conflict_line = false;
                continue;
            }
            if self.discarding_conflict_line {
                continue;
            }
            if self.conflict_line.len() >= STARTUP_CONFLICT_LINE_LIMIT {
                self.conflict_line.clear();
                self.discarding_conflict_line = true;
                continue;
            }
            self.conflict_line.push(*byte);
        }
    }

    fn parse_conflict_line(&mut self) {
        let line = self
            .conflict_line
            .strip_suffix(b"\r")
            .unwrap_or(&self.conflict_line);
        let Some(json) = line.strip_prefix(STARTUP_CONFLICT_MARKER_PREFIX) else {
            return;
        };
        let Ok(json) = std::str::from_utf8(json) else {
            return;
        };
        let Ok(conflict) = serde_json::from_str::<StartupConflict>(json) else {
            return;
        };
        if !conflict.is_valid() {
            return;
        }
        self.conflict_marker_count += 1;
        if self.conflict_marker_count == 1 {
            self.conflict = Some(conflict);
        } else {
            self.conflict = None;
        }
    }

    fn startup_conflict(&self) -> Option<StartupConflict> {
        if self.conflict_marker_count == 1 {
            self.conflict.clone()
        } else {
            None
        }
    }

    fn text(&self) -> Option<String> {
        let text = String::from_utf8_lossy(&self.bytes).trim().to_string();
        if text.is_empty() {
            None
        } else {
            Some(text)
        }
    }
}

#[derive(Debug, Clone, Deserialize, Serialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
pub struct StartupConflict {
    version: u8,
    pub target_kind: ConflictTargetKind,
    pub target_path: String,
    pub storage_kind: ConflictStorageKind,
    pub database_path: Option<String>,
    pub owner: Option<StartupConflictOwner>,
}

impl StartupConflict {
    fn is_valid(&self) -> bool {
        if self.version != 1
            || self.target_path.len() > 4096
            || !Path::new(&self.target_path).is_absolute()
            || self.owner.as_ref().is_some_and(|owner| {
                owner.pid.is_some_and(|pid| pid <= 0)
                    || owner.executable.as_ref().is_some_and(|value| {
                        value.len() > 256 || value.chars().any(char::is_control)
                    })
                    || owner.started_at.as_ref().is_some_and(|value| {
                        value.len() > 256 || value.chars().any(char::is_control)
                    })
            })
        {
            return false;
        }
        let storage_valid = match self.storage_kind {
            ConflictStorageKind::SqliteInHome | ConflictStorageKind::SqliteExternal => self
                .database_path
                .as_ref()
                .is_some_and(|path| path.len() <= 4096 && Path::new(path).is_absolute()),
            ConflictStorageKind::Postgres => self.database_path.is_none(),
        };
        storage_valid
            && (self.target_kind != ConflictTargetKind::Database
                || self.storage_kind == ConflictStorageKind::SqliteExternal)
    }
}

#[derive(Debug, Clone, Copy, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ConflictTargetKind {
    Home,
    Database,
}

#[derive(Debug, Clone, Copy, Deserialize, Serialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum ConflictStorageKind {
    SqliteInHome,
    SqliteExternal,
    Postgres,
}

#[derive(Debug, Clone, Deserialize, Serialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
pub struct StartupConflictOwner {
    pub pid: Option<i64>,
    pub executable: Option<String>,
    pub started_at: Option<String>,
}

#[derive(Debug, PartialEq, Eq)]
pub struct BackendCommandSpec {
    pub program: PathBuf,
    pub args: Vec<OsString>,
    pub cwd: PathBuf,
    pub env: BTreeMap<OsString, OsString>,
}

#[cfg(feature = "desktop-runtime")]
pub fn start_desktop_backend(app: AppHandle, window: WebviewWindow) {
    let state = app.state::<BackendState>().inner().clone();
    thread::spawn(move || {
        if let Err(err) = state.temporary_home_for_launch() {
            let home = state
                .temporary_home_path()
                .map(|path| path.to_string_lossy().into_owned());
            state.stop();
            set_status(&window, "failure", Some(&err), None, home);
            return;
        }
        let startup_kind = if state.is_temporary_test() {
            "temporary"
        } else {
            "loading"
        };
        let home = state
            .temporary_home_path()
            .map(|path| path.to_string_lossy().into_owned());
        set_status(&window, startup_kind, None, None, home.clone());
        match launch_and_wait(&app, &state) {
            Ok(url) => {
                if let Err(err) = state
                    .set_owned_origin(&url)
                    .and_then(|_| navigate_to_backend(&window, &url))
                {
                    state.clear_owned_origin();
                    state.retain_temporary_home();
                    state.stop();
                    let detail =
                        format!("Backend started, but the window could not navigate: {err}");
                    set_status(&window, "failure", Some(&detail), None, home.clone());
                } else {
                    if !state.is_temporary_test() {
                        crate::updater::start_automatic_checks(app);
                    }
                }
            }
            Err(err) => {
                state.stop();
                if let Some(conflict) = state.startup_conflict() {
                    set_status(&window, "conflict", None, Some(conflict), None);
                } else {
                    set_status(&window, "failure", Some(&err), None, home.clone());
                }
            }
        }
    });
}

fn loopback_origin(input: &str) -> Result<String, String> {
    let url = Url::parse(input).map_err(|err| format!("invalid desktop backend URL: {err}"))?;
    if url.scheme() != "http"
        || url.host_str() != Some(LOOPBACK_HOST)
        || url.port_or_known_default().is_none()
    {
        return Err("desktop backend URL must use a loopback HTTP origin".to_string());
    }
    Ok(url.origin().ascii_serialization())
}

fn same_origin(expected: &str, input: &str) -> bool {
    Url::parse(input)
        .map(|url| url.origin().ascii_serialization() == expected)
        .unwrap_or(false)
}

#[cfg(feature = "desktop-runtime")]
fn launch_and_wait(app: &AppHandle, state: &BackendState) -> Result<String, String> {
    let runtime_dir = resolve_runtime_dir(app)?;
    let temporary_home = state.temporary_home_for_launch()?;
    let port = if state.is_temporary_test() {
        pick_loopback_port()?
    } else {
        pick_desktop_port()?
    };
    let health_token = desktop_health_token()?;
    let mut inherited_env: BTreeMap<OsString, OsString> = env::vars_os().collect();
    if let Some(home) = temporary_home.as_ref() {
        inherited_env = temporary_test_backend_environment(inherited_env, home);
    }
    inherited_env.insert(
        OsString::from(DESKTOP_HEALTH_TOKEN_ENV),
        OsString::from(&health_token),
    );
    add_launcher_parent_pid(&mut inherited_env);
    let spec = build_backend_command(
        &runtime_dir,
        port,
        inherited_env,
        current_home_dir().as_deref(),
    )?;
    state.reset_startup_output();
    if state.is_shutdown_started() {
        return Err("Desktop startup cancelled".to_string());
    }
    let mut child = spawn_backend_command(&spec)?;
    let output_readers = capture_child_output(&mut child, state.startup_output.clone());
    *state
        .startup_output_readers
        .lock()
        .expect("startup output reader mutex poisoned") = output_readers;
    if !state.set_child(child) {
        return Err("Desktop startup cancelled".to_string());
    }
    wait_for_backend(port, state, HEALTH_TIMEOUT, &health_token)?;
    wait_for_ready(port, state)?;
    state.mark_backend_ready();
    Ok(format!("http://{LOOPBACK_HOST}:{port}/"))
}

fn temporary_test_backend_environment(
    mut inherited: BTreeMap<OsString, OsString>,
    home: &TemporaryHome,
) -> BTreeMap<OsString, OsString> {
    inherited.retain(|key, _| {
        let key = key.to_string_lossy();
        !key.to_ascii_uppercase().starts_with("KANDEV_")
            || [
                "KANDEV_DEBUG_DEV_MODE",
                "KANDEV_E2E_MOCK",
                "KANDEV_DESKTOP_RUNTIME_DIR",
            ]
            .iter()
            .any(|preserved| key.eq_ignore_ascii_case(preserved))
    });
    inherited.insert(
        OsString::from("KANDEV_HOME_DIR"),
        home.path.as_os_str().to_os_string(),
    );
    inherited.insert(
        OsString::from("KANDEV_DATABASE_DRIVER"),
        OsString::from("sqlite"),
    );
    inherited.insert(
        OsString::from("KANDEV_DATABASE_PATH"),
        home.path.join("data/kandev.db").into_os_string(),
    );
    inherited.insert(
        OsString::from(INTERNAL_CONFIG_FILE_ENV),
        home.path.join("config.yaml").into_os_string(),
    );
    inherited
}

fn add_launcher_parent_pid(env: &mut BTreeMap<OsString, OsString>) {
    env.insert(
        OsString::from(LAUNCHER_PARENT_PID_ENV),
        OsString::from(std::process::id().to_string()),
    );
}

#[cfg(feature = "desktop-runtime")]
fn resolve_runtime_dir(app: &AppHandle) -> Result<PathBuf, String> {
    if let Some(dir) = env::var_os("KANDEV_DESKTOP_RUNTIME_DIR") {
        return Ok(PathBuf::from(dir));
    }
    app.path()
        .resource_dir()
        .map(|dir| dir.join("kandev"))
        .map_err(|err| err.to_string())
}

pub fn build_backend_command(
    runtime_dir: &Path,
    port: u16,
    inherited_env: BTreeMap<OsString, OsString>,
    home_dir: Option<&Path>,
) -> Result<BackendCommandSpec, String> {
    validate_runtime_dir(runtime_dir)?;
    let program = runtime_dir.join("bin").join(executable_name("kandev"));
    let cwd = program
        .parent()
        .ok_or_else(|| {
            format!(
                "Kandev launcher has no parent directory: {}",
                program.display()
            )
        })?
        .to_path_buf();
    Ok(BackendCommandSpec {
        program,
        args: vec![
            OsString::from("--headless"),
            OsString::from("--port"),
            OsString::from(port.to_string()),
        ],
        cwd,
        env: desktop_environment(runtime_dir, inherited_env, home_dir),
    })
}

pub fn validate_runtime_dir(runtime_dir: &Path) -> Result<(), String> {
    let bin_dir = runtime_dir.join("bin");
    require_runtime_file(
        &bin_dir.join(executable_name("kandev")),
        "Kandev launcher binary",
    )?;
    require_runtime_file(
        &bin_dir.join(executable_name("agentctl")),
        "agentctl binary",
    )?;
    match read_remote_helper_manifest_variant(runtime_dir)? {
        Some(RemoteHelperManifestVariant::Standard) => Ok(()),
        Some(RemoteHelperManifestVariant::Full) | None => {
            for &(name, label) in REMOTE_AGENTCTL_HELPERS.iter() {
                require_runtime_file(&bin_dir.join(name), label)?;
            }
            Ok(())
        }
    }
}

#[derive(serde::Deserialize)]
struct DesktopRemoteHelperManifest {
    schema_version: u32,
    version: String,
    commit: String,
    variant: RemoteHelperManifestVariant,
    helpers: Vec<DesktopRemoteHelperRecord>,
}

#[derive(serde::Deserialize)]
struct DesktopRemoteHelperRecord {
    platform: String,
    asset: String,
    sha256: String,
    size_bytes: u64,
}

#[derive(serde::Deserialize)]
#[serde(rename_all = "lowercase")]
enum RemoteHelperManifestVariant {
    Standard,
    Full,
}

fn read_remote_helper_manifest_variant(
    runtime_dir: &Path,
) -> Result<Option<RemoteHelperManifestVariant>, String> {
    let manifest_path = runtime_dir.join("remote-helpers.json");
    let metadata = match std::fs::metadata(&manifest_path) {
        Ok(metadata) => metadata,
        Err(err) if err.kind() == ErrorKind::NotFound => return Ok(None),
        Err(err) => {
            return Err(format!(
                "could not inspect remote helper manifest at {}: {err}",
                manifest_path.display()
            ));
        }
    };
    if !metadata.is_file() || metadata.len() > REMOTE_HELPER_MANIFEST_MAX_BYTES {
        return Err(format!(
            "remote helper manifest at {} is not a regular file within the size limit",
            manifest_path.display()
        ));
    }
    let bytes = std::fs::read(&manifest_path).map_err(|err| {
        format!(
            "could not read remote helper manifest at {}: {err}",
            manifest_path.display()
        )
    })?;
    let manifest: DesktopRemoteHelperManifest = serde_json::from_slice(&bytes).map_err(|err| {
        format!(
            "remote helper manifest at {} is invalid: {err}",
            manifest_path.display()
        )
    })?;
    if manifest.schema_version != REMOTE_HELPER_MANIFEST_SCHEMA_VERSION {
        return Err(format!(
            "remote helper manifest at {} uses unsupported schema version {}",
            manifest_path.display(),
            manifest.schema_version
        ));
    }
    validate_remote_helper_manifest(&manifest, &manifest_path)?;
    Ok(Some(manifest.variant))
}

fn validate_remote_helper_manifest(
    manifest: &DesktopRemoteHelperManifest,
    path: &Path,
) -> Result<(), String> {
    if manifest.version.trim().is_empty() || !is_full_git_sha(&manifest.commit) {
        return Err(format!(
            "remote helper manifest at {} has an invalid release identity",
            path.display()
        ));
    }
    if manifest.helpers.len() != REMOTE_HELPER_MANIFEST_RECORDS.len() {
        return Err(format!(
            "remote helper manifest at {} must contain exactly {} platform records",
            path.display(),
            REMOTE_HELPER_MANIFEST_RECORDS.len()
        ));
    }
    for (platform, asset) in REMOTE_HELPER_MANIFEST_RECORDS {
        let Some(record) = manifest
            .helpers
            .iter()
            .find(|record| record.platform == platform)
        else {
            return Err(format!(
                "remote helper manifest at {} is missing platform {platform}",
                path.display()
            ));
        };
        if record.asset != asset
            || !is_lowercase_sha256(&record.sha256)
            || record.size_bytes == 0
            || record.size_bytes > REMOTE_HELPER_MAX_BYTES
        {
            return Err(format!(
                "remote helper manifest at {} has an invalid record for {platform}",
                path.display()
            ));
        }
    }
    Ok(())
}

fn is_full_git_sha(value: &str) -> bool {
    matches!(value.len(), 40 | 64)
        && value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
}

fn is_lowercase_sha256(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
}

fn require_runtime_file(path: &Path, label: &str) -> Result<(), String> {
    if path.is_file() {
        Ok(())
    } else {
        Err(format!("{label} is missing at {}", path.display()))
    }
}

pub fn desktop_environment(
    runtime_dir: &Path,
    mut env: BTreeMap<OsString, OsString>,
    home_dir: Option<&Path>,
) -> BTreeMap<OsString, OsString> {
    let path = normalized_path(env.get(OsStr::new("PATH")), home_dir);
    env.retain(|key, _| {
        let key = key.to_string_lossy();
        !key.eq_ignore_ascii_case(DESKTOP_NATIVE_NOTIFICATIONS_ENV)
            && !key.eq_ignore_ascii_case(DESKTOP_RUNTIME_ENV)
    });
    env.insert(
        OsString::from("KANDEV_SERVER_HOST"),
        OsString::from(LOOPBACK_HOST),
    );
    env.insert(
        OsString::from("KANDEV_BUNDLE_DIR"),
        runtime_dir.as_os_str().to_os_string(),
    );
    env.insert(
        OsString::from(DESKTOP_NATIVE_NOTIFICATIONS_ENV),
        OsString::from("true"),
    );
    env.insert(OsString::from(DESKTOP_RUNTIME_ENV), OsString::from("true"));
    env.insert(OsString::from("PATH"), path);
    env
}

pub fn pick_loopback_port() -> Result<u16, String> {
    let listener = TcpListener::bind((LOOPBACK_HOST, 0)).map_err(|err| err.to_string())?;
    listener
        .local_addr()
        .map(|addr| addr.port())
        .map_err(|err| err.to_string())
}

pub fn pick_desktop_port() -> Result<u16, String> {
    let preferred = preferred_desktop_port(env::var_os(DESKTOP_PORT_ENV))?;
    pick_available_loopback_port(preferred)
}

fn desktop_health_token() -> Result<String, String> {
    let mut bytes = [0_u8; 32];
    getrandom::fill(&mut bytes)
        .map_err(|err| format!("Could not generate desktop health token: {err}"))?;
    Ok(hex_encode(&bytes))
}

fn hex_encode(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}

fn preferred_desktop_port(value: Option<OsString>) -> Result<u16, String> {
    let Some(value) = value else {
        return Ok(DEFAULT_DESKTOP_PORT);
    };
    let value = value
        .into_string()
        .map_err(|_| format!("{DESKTOP_PORT_ENV} must be valid UTF-8"))?;
    let port = value
        .parse::<u16>()
        .map_err(|_| format!("{DESKTOP_PORT_ENV} must be a TCP port between 1 and 65535"))?;
    if port == 0 {
        Err(format!(
            "{DESKTOP_PORT_ENV} must be a TCP port between 1 and 65535"
        ))
    } else {
        Ok(port)
    }
}

fn pick_available_loopback_port(preferred: u16) -> Result<u16, String> {
    match TcpListener::bind((LOOPBACK_HOST, preferred)) {
        Ok(listener) => listener
            .local_addr()
            .map(|addr| addr.port())
            .map_err(|err| err.to_string()),
        Err(err) if err.kind() == ErrorKind::AddrInUse => pick_loopback_port(),
        Err(err) => Err(format!(
            "Could not reserve {LOOPBACK_HOST}:{preferred}: {err}"
        )),
    }
}

fn spawn_backend_command(spec: &BackendCommandSpec) -> Result<Child, String> {
    let mut command = Command::new(&spec.program);
    command
        .args(&spec.args)
        .current_dir(&spec.cwd)
        .env_clear()
        .envs(&spec.env)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    command.spawn().map_err(|err| {
        format!(
            "Failed to start Kandev launcher at {}: {err}",
            spec.program.display()
        )
    })
}

fn capture_child_output(
    child: &mut Child,
    output: Arc<Mutex<StartupOutput>>,
) -> Vec<std::sync::mpsc::Receiver<()>> {
    let mut readers = Vec::new();
    if let Some(stdout) = child.stdout.take() {
        readers.push(capture_stream("stdout", stdout, output.clone()));
    }
    if let Some(stderr) = child.stderr.take() {
        readers.push(capture_stream("stderr", stderr, output));
    }
    readers
}

fn capture_stream<R>(
    stream: &'static str,
    mut reader: R,
    output: Arc<Mutex<StartupOutput>>,
) -> std::sync::mpsc::Receiver<()>
where
    R: Read + Send + 'static,
{
    let (finished_tx, finished_rx) = std::sync::mpsc::channel();
    thread::spawn(move || {
        let mut buffer = [0_u8; 1024];
        loop {
            match reader.read(&mut buffer) {
                Ok(0) => break,
                Err(err) if err.kind() == ErrorKind::Interrupted => continue,
                Err(_) => break,
                Ok(n) => output
                    .lock()
                    .expect("startup output mutex poisoned")
                    .push(stream, &buffer[..n]),
            }
        }
        drop(finished_tx);
    });
    finished_rx
}

fn wait_for_startup_output_readers(
    readers: Vec<std::sync::mpsc::Receiver<()>>,
    timeout: Duration,
) -> bool {
    let deadline = Instant::now() + timeout;
    for reader in readers {
        let remaining = deadline.saturating_duration_since(Instant::now());
        if matches!(
            reader.recv_timeout(remaining),
            Err(std::sync::mpsc::RecvTimeoutError::Timeout)
        ) {
            return false;
        }
    }
    true
}

fn wait_for_backend(
    port: u16,
    state: &BackendState,
    timeout: Duration,
    expected_health_token: &str,
) -> Result<(), String> {
    let deadline = Instant::now() + timeout;
    loop {
        if state.is_shutdown_started() {
            return Err("Desktop startup cancelled".to_string());
        }
        if let Some(message) = state.child_exit_message()? {
            return Err(format!(
                "Kandev launcher exited before /health became ready ({message})"
            ));
        }
        if health_ready(port, expected_health_token) {
            thread::sleep(HEALTH_READY_SETTLE);
            if let Some(message) = state.child_exit_message()? {
                return Err(format!(
                    "Kandev launcher exited before /health became ready ({message})"
                ));
            }
            return Ok(());
        }
        if Instant::now() >= deadline {
            let mut message = format!("Timed out waiting for http://{LOOPBACK_HOST}:{port}/health");
            append_recent_output(&mut message, state.recent_startup_output());
            return Err(message);
        }
        thread::sleep(Duration::from_millis(250));
    }
}

// wait_for_ready polls GET /ready after wait_for_backend's /health check
// already passed. /health flips to 200 as soon as the listener binds, before
// startup recovery finishes, so treating a healthy response as "safe to
// navigate the webview" reopens the crash-loop-adjacent bug this backend
// change fixed: the window would load the bootstrap handler's raw 503 body.
// /ready is the readiness signal instead. This wait has no timeout and never
// aborts the launch on its own: HEALTH_TIMEOUT above already made the
// keep-or-kill decision, and readiness can legitimately take longer while
// startup recovery sweeps run. It only returns early if the backend process
// exits or shutdown starts. Mirrors wait_for_backend's settle-then-recheck
// after a successful probe: a bare "GET /ready succeeded" is not proof the
// backend that answered is still the one we launched, since another process
// could rebind the same loopback port in the gap between exit and probe.
fn wait_for_ready(port: u16, state: &BackendState) -> Result<(), String> {
    loop {
        if state.is_shutdown_started() {
            return Err("Desktop startup cancelled".to_string());
        }
        if let Some(message) = state.child_exit_message()? {
            return Err(format!(
                "Kandev launcher exited before /ready reported ready ({message})"
            ));
        }
        if ready_ready(port) {
            thread::sleep(HEALTH_READY_SETTLE);
            if let Some(message) = state.child_exit_message()? {
                return Err(format!(
                    "Kandev launcher exited before /ready reported ready ({message})"
                ));
            }
            return Ok(());
        }
        thread::sleep(Duration::from_millis(250));
    }
}

fn launcher_exit_message(status: &str, output: Option<String>) -> String {
    let mut message = status.to_string();
    append_recent_output(&mut message, output);
    message
}

fn append_recent_output(message: &mut String, output: Option<String>) {
    if let Some(output) = output {
        message.push_str("\n\nRecent backend output:\n");
        message.push_str(&output);
    }
}

fn health_ready(port: u16, expected_health_token: &str) -> bool {
    request_health(port, expected_health_token).unwrap_or(false)
}

fn request_health(port: u16, expected_health_token: &str) -> Result<bool, String> {
    let addr = (LOOPBACK_HOST, port)
        .to_socket_addrs()
        .map_err(|err| err.to_string())?
        .next()
        .ok_or_else(|| format!("Could not resolve {LOOPBACK_HOST}:{port}"))?;
    let mut stream = TcpStream::connect_timeout(&addr, Duration::from_millis(250))
        .map_err(|err| err.to_string())?;
    stream
        .set_read_timeout(Some(Duration::from_millis(250)))
        .map_err(|err| err.to_string())?;
    stream
        .write_all(
            format!(
                "GET /health HTTP/1.1\r\nHost: {LOOPBACK_HOST}:{port}\r\nConnection: close\r\n\r\n"
            )
            .as_bytes(),
        )
        .map_err(|err| err.to_string())?;
    let response = read_http_response_head(&mut stream)?;
    if !(response.starts_with(b"HTTP/1.1 200") || response.starts_with(b"HTTP/1.0 200")) {
        return Ok(false);
    }
    Ok(response_has_header(
        &response,
        DESKTOP_HEALTH_TOKEN_HEADER,
        expected_health_token,
    ))
}

fn ready_ready(port: u16) -> bool {
    request_ready(port).unwrap_or(false)
}

// request_ready hits /ready rather than /health. /ready never sets
// DESKTOP_HEALTH_TOKEN_HEADER (that header is /health-only — see
// docs/specs/health-endpoint-version/spec.md AC-21), so unlike
// request_health this does not check for one.
fn request_ready(port: u16) -> Result<bool, String> {
    let addr = (LOOPBACK_HOST, port)
        .to_socket_addrs()
        .map_err(|err| err.to_string())?
        .next()
        .ok_or_else(|| format!("Could not resolve {LOOPBACK_HOST}:{port}"))?;
    let mut stream = TcpStream::connect_timeout(&addr, Duration::from_millis(250))
        .map_err(|err| err.to_string())?;
    stream
        .set_read_timeout(Some(Duration::from_millis(250)))
        .map_err(|err| err.to_string())?;
    stream
        .write_all(
            format!(
                "GET /ready HTTP/1.1\r\nHost: {LOOPBACK_HOST}:{port}\r\nConnection: close\r\n\r\n"
            )
            .as_bytes(),
        )
        .map_err(|err| err.to_string())?;
    let response = read_http_response_head(&mut stream)?;
    Ok(response.starts_with(b"HTTP/1.1 200") || response.starts_with(b"HTTP/1.0 200"))
}

fn read_http_response_head<R: Read>(reader: &mut R) -> Result<Vec<u8>, String> {
    const MAX_HEADER_BYTES: usize = 8 * 1024;
    let mut response = Vec::with_capacity(512);
    let mut buffer = [0_u8; 16];
    while response.len() < MAX_HEADER_BYTES {
        let read_len = buffer.len().min(MAX_HEADER_BYTES - response.len());
        let n = reader
            .read(&mut buffer[..read_len])
            .map_err(|err| err.to_string())?;
        if n == 0 {
            break;
        }
        response.extend_from_slice(&buffer[..n]);
        if response.windows(4).any(|window| window == b"\r\n\r\n") {
            break;
        }
    }
    Ok(response)
}

fn response_has_header(response: &[u8], expected_name: &str, expected_value: &str) -> bool {
    let response = String::from_utf8_lossy(response);
    for line in response.lines().skip(1) {
        let Some((name, value)) = line.split_once(':') else {
            continue;
        };
        if name.eq_ignore_ascii_case(expected_name) && value.trim() == expected_value {
            return true;
        }
    }
    false
}

fn normalized_path(existing: Option<&OsString>, home_dir: Option<&Path>) -> OsString {
    let mut entries: Vec<PathBuf> = existing
        .map(env::split_paths)
        .into_iter()
        .flatten()
        .collect();
    for entry in common_path_entries(home_dir) {
        if !entries.iter().any(|existing| existing == &entry) {
            entries.push(entry);
        }
    }
    env::join_paths(entries).unwrap_or_else(|_| existing.cloned().unwrap_or_default())
}

fn common_path_entries(home_dir: Option<&Path>) -> Vec<PathBuf> {
    let mut entries = if cfg!(windows) {
        Vec::new()
    } else if cfg!(target_os = "macos") {
        vec![
            PathBuf::from("/opt/homebrew/bin"),
            PathBuf::from("/usr/local/bin"),
            PathBuf::from("/usr/bin"),
            PathBuf::from("/bin"),
        ]
    } else {
        vec![
            PathBuf::from("/usr/local/bin"),
            PathBuf::from("/usr/bin"),
            PathBuf::from("/bin"),
            PathBuf::from("/opt/homebrew/bin"),
            PathBuf::from("/home/linuxbrew/.linuxbrew/bin"),
        ]
    };
    if let Some(home) = home_dir {
        entries.push(home.join(".local/bin"));
        if cfg!(windows) {
            entries.push(home.join("AppData/Roaming/npm"));
            entries.push(home.join("scoop/shims"));
        } else {
            entries.push(home.join(".bun/bin"));
            entries.push(home.join(".opencode/bin"));
        }
    }
    entries
}

fn current_home_dir() -> Option<PathBuf> {
    env::var_os("HOME")
        .or_else(|| env::var_os("USERPROFILE"))
        .map(PathBuf::from)
}

pub(crate) fn picker_home_dir() -> Option<PathBuf> {
    current_home_dir()
}

fn executable_name(name: &str) -> OsString {
    if cfg!(windows) {
        OsString::from(format!("{name}.exe"))
    } else {
        OsString::from(name)
    }
}

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub fn start_temporary_test_instance(
    state: State<'_, BackendState>,
    webview: WebviewWindow,
) -> Result<(), String> {
    state.require_conflict_startup(&webview)?;
    let executable = env::current_exe()
        .map_err(|err| format!("Could not locate the Kandev desktop application: {err}"))?;
    Command::new(&executable)
        .arg(TEMPORARY_TEST_ARGUMENT)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .map(|_| ())
        .map_err(|err| format!("Could not open a temporary Kandev test window: {err}"))
}

#[cfg(feature = "desktop-runtime")]
fn set_status(
    window: &WebviewWindow,
    kind: &str,
    detail: Option<&str>,
    conflict: Option<StartupConflict>,
    home: Option<String>,
) {
    let payload = serde_json::json!({
        "kind": kind,
        "detail": detail,
        "conflict": conflict,
        "home": home,
    });
    let script = format!(
        "window.__KANDEV_DESKTOP_PENDING_STATUS={payload};window.__KANDEV_DESKTOP_SET_STATUS?.({payload});"
    );
    let _ = window.eval(&script);
}

#[cfg(feature = "desktop-runtime")]
fn navigate_to_backend(window: &WebviewWindow, url: &str) -> Result<(), String> {
    let url = serde_json::to_string(url).map_err(|err| err.to_string())?;
    window
        .eval(&format!("window.location.replace({url});"))
        .map_err(|err| err.to_string())
}

#[cfg(unix)]
fn terminate_child(child: &mut Child) -> bool {
    terminate_child_with_timeout(child, DESKTOP_LAUNCHER_SHUTDOWN_TIMEOUT)
}

#[cfg(windows)]
fn terminate_child(child: &mut Child) -> bool {
    terminate_child_with_timeout(child, DESKTOP_LAUNCHER_SHUTDOWN_TIMEOUT)
}

#[cfg(not(any(unix, windows)))]
fn terminate_child(child: &mut Child) -> bool {
    terminate_child_with_timeout(child, Duration::from_secs(0))
}

fn terminate_child_with_timeout(child: &mut Child, graceful_timeout: Duration) -> bool {
    match child.try_wait() {
        Ok(Some(_)) => return false,
        Err(_) => {
            force_kill_child(child);
            let _ = child.wait();
            return false;
        }
        Ok(None) => {}
    }
    if !request_graceful_child_shutdown(child) {
        force_kill_child(child);
        let _ = child.wait();
        return false;
    }
    wait_or_kill(child, graceful_timeout)
}

#[cfg(unix)]
fn request_graceful_child_shutdown(child: &Child) -> bool {
    unsafe { libc::kill(child.id() as i32, libc::SIGTERM) == 0 }
}

#[cfg(windows)]
fn request_graceful_child_shutdown(child: &Child) -> bool {
    let pid = child.id().to_string();
    Command::new("taskkill")
        .args(["/T", "/PID", &pid])
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status()
        .is_ok_and(|status| status.success())
}

#[cfg(not(any(unix, windows)))]
fn request_graceful_child_shutdown(_child: &Child) -> bool {
    false
}

fn wait_or_kill(child: &mut Child, graceful_timeout: Duration) -> bool {
    let deadline = Instant::now() + graceful_timeout;
    loop {
        match child.try_wait() {
            Ok(Some(status)) => return status.success(),
            Ok(None) if Instant::now() < deadline => thread::sleep(Duration::from_millis(100)),
            Ok(None) | Err(_) => break,
        }
    }
    force_kill_child(child);
    let _ = child.wait();
    false
}

#[cfg(windows)]
fn force_kill_child(child: &mut Child) {
    let pid = child.id().to_string();
    let _ = Command::new("taskkill")
        .args(["/PID", &pid, "/T", "/F"])
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
    let _ = child.kill();
}

#[cfg(not(windows))]
fn force_kill_child(child: &mut Child) {
    let _ = child.kill();
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    #[test]
    fn command_spec_uses_headless_launcher_and_loopback_env() {
        let runtime_dir = temp_runtime_dir("command-spec");
        let mut inherited = BTreeMap::new();
        inherited.insert(OsString::from("CUSTOM_ENV"), OsString::from("kept"));
        inherited.insert(
            OsString::from(DESKTOP_HEALTH_TOKEN_ENV),
            OsString::from("health-token"),
        );
        inherited.insert(
            OsString::from(LAUNCHER_PARENT_PID_ENV),
            OsString::from("12345"),
        );
        inherited.insert(OsString::from("PATH"), OsString::from("/existing/bin"));

        let spec = build_backend_command(
            &runtime_dir,
            48123,
            inherited,
            Some(Path::new("/home/example")),
        )
        .expect("command spec");

        assert_eq!(
            spec.program,
            runtime_dir.join("bin").join(executable_name("kandev"))
        );
        assert_eq!(
            spec.args,
            vec![
                OsString::from("--headless"),
                OsString::from("--port"),
                OsString::from("48123"),
            ]
        );
        assert_eq!(
            spec.env.get(OsStr::new("CUSTOM_ENV")),
            Some(&OsString::from("kept"))
        );
        assert_eq!(
            spec.env.get(OsStr::new("KANDEV_SERVER_HOST")),
            Some(&OsString::from(LOOPBACK_HOST))
        );
        assert_eq!(
            spec.env.get(OsStr::new("KANDEV_BUNDLE_DIR")),
            Some(&runtime_dir.as_os_str().to_os_string())
        );
        assert_eq!(
            spec.env.get(OsStr::new(DESKTOP_HEALTH_TOKEN_ENV)),
            Some(&OsString::from("health-token"))
        );
        assert_eq!(
            spec.env.get(OsStr::new(LAUNCHER_PARENT_PID_ENV)),
            Some(&OsString::from("12345"))
        );
        assert_eq!(
            spec.env
                .get(OsStr::new("KANDEV_DESKTOP_NATIVE_NOTIFICATIONS")),
            Some(&OsString::from("true"))
        );
        assert_eq!(
            spec.env.get(OsStr::new("KANDEV_DESKTOP_RUNTIME")),
            Some(&OsString::from("true"))
        );
    }

    #[test]
    fn launcher_parent_environment_uses_shell_pid() {
        let mut inherited = BTreeMap::new();

        add_launcher_parent_pid(&mut inherited);

        assert_eq!(
            inherited.get(OsStr::new(LAUNCHER_PARENT_PID_ENV)),
            Some(&OsString::from(std::process::id().to_string()))
        );
    }

    #[test]
    fn desktop_environment_preserves_path_and_adds_gui_paths() {
        let runtime_dir = Path::new("/opt/kandev");
        let home_dir = Path::new("/home/example");
        let mut inherited = BTreeMap::new();
        inherited.insert(OsString::from("PATH"), OsString::from("/existing/bin"));

        let env = desktop_environment(runtime_dir, inherited, Some(home_dir));
        let path = env.get(OsStr::new("PATH")).expect("PATH");
        let entries: Vec<PathBuf> = env::split_paths(path).collect();

        assert_eq!(entries.first(), Some(&PathBuf::from("/existing/bin")));
        assert!(entries.contains(&PathBuf::from("/usr/local/bin")) || cfg!(windows));
        assert!(entries.contains(&home_dir.join(".local/bin")));
    }

    #[test]
    fn desktop_environment_replaces_notification_flag_case_insensitively() {
        let mut inherited = BTreeMap::new();
        inherited.insert(
            OsString::from("kandev_desktop_native_notifications"),
            OsString::from("false"),
        );

        let env = desktop_environment(Path::new("/opt/kandev"), inherited, None);

        assert_eq!(
            env.get(OsStr::new(DESKTOP_NATIVE_NOTIFICATIONS_ENV)),
            Some(&OsString::from("true"))
        );
        assert!(!env.contains_key(OsStr::new("kandev_desktop_native_notifications")));
    }

    #[test]
    fn desktop_environment_overrides_inherited_server_host_with_loopback() {
        // Desktop launches must keep the embedded backend on the loopback host.
        let mut inherited = BTreeMap::new();
        inherited.insert(
            OsString::from("KANDEV_SERVER_HOST"),
            OsString::from("10.0.0.42"),
        );

        let env = desktop_environment(Path::new("/opt/kandev"), inherited, None);

        assert_eq!(
            env.get(OsStr::new("KANDEV_SERVER_HOST")),
            Some(&OsString::from(LOOPBACK_HOST)),
            "desktop_environment must force the loopback server host",
        );
    }

    #[test]
    fn desktop_environment_falls_back_to_loopback_when_server_host_unset() {
        // The default production path: no inherited KANDEV_SERVER_HOST,
        // desktop_environment must inject the loopback default.
        let env = desktop_environment(Path::new("/opt/kandev"), BTreeMap::new(), None);

        assert_eq!(
            env.get(OsStr::new("KANDEV_SERVER_HOST")),
            Some(&OsString::from(LOOPBACK_HOST))
        );
    }

    #[test]
    fn missing_launcher_returns_readable_error() {
        let dir = temp_root("missing-launcher");
        let err = build_backend_command(&dir, 48123, BTreeMap::new(), None)
            .expect_err("missing launcher");
        assert!(err.contains("Kandev launcher binary is missing"), "{err}");
    }

    #[test]
    fn missing_agentctl_returns_readable_error() {
        let dir = temp_root("missing-agentctl");
        let bin = dir.join("bin");
        fs::create_dir_all(&bin).expect("create bin");
        fs::write(bin.join(executable_name("kandev")), b"stub").expect("write launcher");

        let err = build_backend_command(&dir, 48123, BTreeMap::new(), None)
            .expect_err("missing agentctl");

        assert!(err.contains("agentctl binary is missing"), "{err}");
    }

    #[test]
    fn missing_linux_helper_returns_readable_error() {
        let dir = temp_root("missing-linux-helper");
        let bin = dir.join("bin");
        fs::create_dir_all(&bin).expect("create bin");
        fs::write(bin.join(executable_name("kandev")), b"stub").expect("write launcher");
        fs::write(bin.join(executable_name("agentctl")), b"stub").expect("write agentctl");

        let err =
            build_backend_command(&dir, 48123, BTreeMap::new(), None).expect_err("missing helper");

        assert!(
            err.contains("agentctl linux/amd64 helper is missing"),
            "{err}"
        );
    }

    #[test]
    fn manifest_bearing_standard_runtime_needs_only_host_binaries() {
        let dir = temp_root("standard-runtime");
        let bin = dir.join("bin");
        fs::create_dir_all(&bin).expect("create bin");
        fs::write(bin.join(executable_name("kandev")), b"stub").expect("write launcher");
        fs::write(bin.join(executable_name("agentctl")), b"stub").expect("write agentctl");
        fs::write(
            dir.join("remote-helpers.json"),
            desktop_helper_manifest(1, "standard"),
        )
        .expect("write manifest");

        validate_runtime_dir(&dir).expect("validate standard runtime");
    }

    #[test]
    fn manifest_bearing_full_runtime_requires_all_helpers() {
        let dir = temp_root("full-runtime-missing-helpers");
        let bin = dir.join("bin");
        fs::create_dir_all(&bin).expect("create bin");
        fs::write(bin.join(executable_name("kandev")), b"stub").expect("write launcher");
        fs::write(bin.join(executable_name("agentctl")), b"stub").expect("write agentctl");
        fs::write(
            dir.join("remote-helpers.json"),
            desktop_helper_manifest(1, "full"),
        )
        .expect("write manifest");

        let err = validate_runtime_dir(&dir).expect_err("full runtime needs helpers");

        assert!(
            err.contains("agentctl linux/amd64 helper is missing"),
            "{err}"
        );
    }

    #[test]
    fn invalid_remote_helper_manifests_are_rejected() {
        let mut unsupported_schema: serde_json::Value =
            serde_json::from_slice(&desktop_helper_manifest(1, "standard")).unwrap();
        unsupported_schema["schema_version"] = serde_json::json!(2);
        let mut unsupported_variant: serde_json::Value =
            serde_json::from_slice(&desktop_helper_manifest(1, "standard")).unwrap();
        unsupported_variant["variant"] = serde_json::json!("compact");
        for (name, contents) in [
            ("empty", b"{}".to_vec()),
            ("corrupt", b"not json".to_vec()),
            (
                "missing-contract-fields",
                br#"{"schema_version":1,"variant":"standard"}"#.to_vec(),
            ),
            (
                "unsupported-variant",
                serde_json::to_vec(&unsupported_variant).unwrap(),
            ),
            (
                "unsupported-schema",
                serde_json::to_vec(&unsupported_schema).unwrap(),
            ),
        ] {
            let dir = temp_root(&format!("invalid-manifest-{name}"));
            let bin = dir.join("bin");
            fs::create_dir_all(&bin).expect("create bin");
            fs::write(bin.join(executable_name("kandev")), b"stub").expect("write launcher");
            fs::write(bin.join(executable_name("agentctl")), b"stub").expect("write agentctl");
            fs::write(dir.join("remote-helpers.json"), contents).expect("write manifest");

            let err = validate_runtime_dir(&dir)
                .expect_err(&format!("{name} manifest should be rejected"));

            assert!(err.contains("remote helper manifest"), "{name}: {err}");
        }
    }

    #[test]
    fn legacy_complete_runtime_still_validates_without_manifest() {
        let dir = temp_runtime_dir("legacy-complete-runtime");

        validate_runtime_dir(&dir).expect("validate legacy complete runtime");
    }

    #[test]
    fn missing_darwin_helper_returns_readable_error() {
        let dir = temp_root("missing-darwin-helper");
        let bin = dir.join("bin");
        fs::create_dir_all(&bin).expect("create bin");
        fs::write(bin.join(executable_name("kandev")), b"stub").expect("write launcher");
        fs::write(bin.join(executable_name("agentctl")), b"stub").expect("write agentctl");
        fs::write(bin.join("agentctl-linux-amd64"), b"stub").expect("write linux helper");
        fs::write(bin.join("agentctl-linux-arm64"), b"stub").expect("write linux arm64 helper");
        fs::write(bin.join("agentctl-darwin-amd64"), b"stub").expect("write darwin amd64 helper");

        let err =
            build_backend_command(&dir, 48123, BTreeMap::new(), None).expect_err("missing helper");

        assert!(
            err.contains("agentctl darwin/arm64 helper is missing"),
            "{err}"
        );
    }

    #[test]
    fn pick_loopback_port_returns_valid_port() {
        let port = pick_loopback_port().expect("loopback port");
        assert_ne!(port, 0);
    }

    #[test]
    fn preferred_desktop_port_defaults_to_stable_origin() {
        assert_eq!(
            preferred_desktop_port(None).expect("default desktop port"),
            DEFAULT_DESKTOP_PORT
        );
    }

    #[test]
    fn preferred_desktop_port_accepts_env_override() {
        assert_eq!(
            preferred_desktop_port(Some(OsString::from("49152"))).expect("env desktop port"),
            49152
        );
    }

    #[test]
    fn preferred_desktop_port_rejects_invalid_env_override() {
        let err = preferred_desktop_port(Some(OsString::from("0"))).expect_err("zero port");

        assert!(err.contains(DESKTOP_PORT_ENV), "{err}");
    }

    #[test]
    fn desktop_health_token_is_hex_encoded() {
        let token = desktop_health_token().expect("desktop health token");

        assert_eq!(token.len(), 64);
        assert!(token.chars().all(|ch| ch.is_ascii_hexdigit()), "{token}");
    }

    #[test]
    fn pick_available_loopback_port_accepts_kernel_assigned_port() {
        let port = pick_available_loopback_port(0).expect("kernel-assigned port");

        assert_ne!(port, 0);
    }

    #[test]
    fn pick_available_loopback_port_falls_back_when_preferred_port_is_taken() {
        let listener = TcpListener::bind((LOOPBACK_HOST, 0)).expect("reserve occupied port");
        let occupied = listener.local_addr().expect("occupied address").port();

        let picked = pick_available_loopback_port(occupied).expect("fallback port");

        assert_ne!(picked, occupied);
        assert_ne!(picked, 0);
    }

    #[test]
    fn shutdown_request_is_one_shot() {
        let state = BackendState::default();

        assert!(state.begin_shutdown());
        assert!(!state.begin_shutdown());
    }

    #[test]
    fn stop_marks_shutdown_started() {
        let state = BackendState::default();

        state.stop();

        assert!(state.is_shutdown_started());
    }

    #[test]
    fn desktop_commands_require_the_exact_owned_backend_origin() {
        let state = BackendState::default();
        state
            .set_owned_origin("http://127.0.0.1:38430")
            .expect("set owned backend origin");

        assert!(state.accepts_url("http://127.0.0.1:38430/settings"));
        assert!(!state.accepts_url("http://127.0.0.1:38431/settings"));
        assert!(!state.accepts_url("http://localhost:38430/settings"));
        assert!(!state.accepts_url("https://127.0.0.1:38430/settings"));
    }

    #[test]
    fn desktop_commands_are_denied_before_backend_origin_is_set() {
        assert!(!BackendState::default().accepts_url("http://127.0.0.1:38430"));
    }

    #[test]
    fn temporary_test_process_mode_requires_the_internal_launch_argument() {
        assert!(!is_temporary_test_args([OsString::from("kandev")]));
        assert!(is_temporary_test_args([
            OsString::from("kandev"),
            OsString::from(TEMPORARY_TEST_ARGUMENT)
        ]));
    }

    #[test]
    fn isolated_window_action_only_accepts_the_local_startup_origin() {
        assert!(is_local_startup_url("tauri://localhost/"));
        assert!(is_local_startup_url("http://tauri.localhost/"));
        assert!(is_local_startup_url("http://localhost:1420/"));
        assert!(!is_local_startup_url("http://127.0.0.1:38430/"));
        assert!(!is_local_startup_url("https://example.com/"));
    }

    #[test]
    fn temporary_test_action_requires_a_detected_conflict_in_normal_mode() {
        let state = BackendState::default();
        let conflict = StartupConflict {
            version: 1,
            target_kind: ConflictTargetKind::Home,
            target_path: "/tmp/kandev-home".to_string(),
            storage_kind: ConflictStorageKind::SqliteInHome,
            database_path: Some("/tmp/kandev-home/data/kandev.db".to_string()),
            owner: None,
        };
        assert!(!state.can_start_temporary_test("tauri://localhost/"));
        *state
            .startup_conflict
            .lock()
            .expect("startup conflict mutex poisoned") = Some(conflict.clone());
        assert!(state.can_start_temporary_test("tauri://localhost/"));
        assert!(!state.can_start_temporary_test("http://127.0.0.1:38430/"));

        let temporary = BackendState::temporary_test_instance();
        *temporary
            .startup_conflict
            .lock()
            .expect("startup conflict mutex poisoned") = Some(conflict);
        assert!(!temporary.can_start_temporary_test("tauri://localhost/"));
    }

    #[cfg(unix)]
    #[test]
    fn live_child_is_recognized_as_running() {
        let state = BackendState::default();
        let child = Command::new("sh")
            .args(["-c", "sleep 1"])
            .spawn()
            .expect("start child");

        assert!(state.set_child(child));
        assert!(state.has_live_child());
        state.stop();
    }

    #[test]
    fn launcher_exit_message_includes_recent_output() {
        let message = launcher_exit_message("exit status: 1", Some("database failed".to_string()));

        assert!(message.contains("exit status: 1"));
        assert!(message.contains("Recent backend output"));
        assert!(message.contains("database failed"));
    }

    #[test]
    fn startup_output_is_bounded() {
        let mut output = StartupOutput::default();
        let chunk = vec![b'x'; STARTUP_OUTPUT_LIMIT + 256];

        output.push("stderr", &chunk);

        assert!(output.bytes.len() <= STARTUP_OUTPUT_LIMIT);
    }

    #[test]
    fn startup_output_parses_typed_conflict_across_stderr_chunks() {
        let mut output = StartupOutput::default();
        let marker = concat!(
            "KANDEV_DESKTOP_CONFLICT_V1 {\"version\":1,\"target_kind\":\"home\",",
            "\"target_path\":\"/tmp/kandev-home\",\"storage_kind\":\"sqlite_in_home\",",
            "\"database_path\":\"/tmp/kandev-home/data/kandev.db\",",
            "\"owner\":{\"pid\":1234,\"executable\":\"/usr/bin/kandev\",",
            "\"started_at\":\"2026-09-25T12:00:00Z\"}}\n"
        );
        let split = marker.len() / 2;

        output.push("stdout", marker.as_bytes());
        output.push("stderr", &marker.as_bytes()[..split]);
        assert!(output.startup_conflict().is_none());
        output.push("stderr", &marker.as_bytes()[split..]);

        let conflict = output.startup_conflict().expect("complete conflict marker");
        assert_eq!(conflict.target_kind, ConflictTargetKind::Home);
        assert_eq!(conflict.target_path, "/tmp/kandev-home");
        assert_eq!(conflict.storage_kind, ConflictStorageKind::SqliteInHome);
        assert_eq!(
            conflict.database_path.as_deref(),
            Some("/tmp/kandev-home/data/kandev.db")
        );
        assert_eq!(
            conflict.owner.as_ref().and_then(|owner| owner.pid),
            Some(1234)
        );
    }

    #[test]
    fn startup_output_rejects_malformed_duplicate_and_stdout_conflicts() {
        let valid = concat!(
            "KANDEV_DESKTOP_CONFLICT_V1 {\"version\":1,\"target_kind\":\"home\",",
            "\"target_path\":\"/tmp/kandev-home\",\"storage_kind\":\"sqlite_in_home\",",
            "\"database_path\":\"/tmp/kandev-home/data/kandev.db\"}\n"
        );
        let mut output = StartupOutput::default();
        output.push("stderr", b"KANDEV_DESKTOP_CONFLICT_V1 {bad json}\n");
        output.push("stdout", valid.as_bytes());
        assert!(output.startup_conflict().is_none());

        output.push("stderr", valid.as_bytes());
        output.push("stderr", valid.as_bytes());
        assert!(output.startup_conflict().is_none());

        let oversized_owner = format!(
            "KANDEV_DESKTOP_CONFLICT_V1 {{\"version\":1,\"target_kind\":\"home\",\
             \"target_path\":\"/tmp/kandev-home\",\"storage_kind\":\"sqlite_in_home\",\
             \"database_path\":\"/tmp/kandev-home/data/kandev.db\",\
             \"owner\":{{\"executable\":\"{}\"}}}}\n",
            "x".repeat(257)
        );
        let mut output = StartupOutput::default();
        output.push("stderr", oversized_owner.as_bytes());
        assert!(output.startup_conflict().is_none());
    }

    #[test]
    fn temporary_test_homes_are_private_and_contain_an_empty_config() {
        let first = TemporaryHome::create().expect("first temporary home");
        let second = TemporaryHome::create().expect("second temporary home");

        assert_ne!(first.path, second.path);
        assert_eq!(first.path.parent(), Some(first.temp_root.as_path()));
        assert!(first.path.join("config.yaml").is_file());
        assert!(!first.path.join("data/kandev.db").exists());
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mode = fs::metadata(&first.path)
                .expect("temporary home metadata")
                .permissions()
                .mode()
                & 0o777;
            assert_eq!(mode, 0o700);
        }

        assert!(first.remove_if_owned());
        assert!(second.remove_if_owned());
    }

    #[test]
    fn temporary_test_environment_pins_home_database_and_profile() {
        let home = TemporaryHome::create().expect("temporary home");
        let inherited = BTreeMap::from([
            (OsString::from("CUSTOM_ENV"), OsString::from("keep")),
            (OsString::from("PATH"), OsString::from("/existing/bin")),
            (
                OsString::from("KANDEV_HOME_DIR"),
                OsString::from("/shared/home"),
            ),
            (
                OsString::from("KANDEV_DATABASE_DRIVER"),
                OsString::from("postgres"),
            ),
            (
                OsString::from("KANDEV_DATABASE_PATH"),
                OsString::from("/shared/database.db"),
            ),
            (
                OsString::from("KANDEV_INTERNAL_CONFIG_HOME_FILE"),
                OsString::from("/shared/config.yaml"),
            ),
            (
                OsString::from("KANDEV_DESKTOP_PORT"),
                OsString::from("38430"),
            ),
            (
                OsString::from("KANDEV_DEBUG_DEV_MODE"),
                OsString::from("true"),
            ),
            (OsString::from("KANDEV_E2E_MOCK"), OsString::from("true")),
            (
                OsString::from("KANDEV_DESKTOP_RUNTIME_DIR"),
                OsString::from("/test/runtime"),
            ),
        ]);

        let isolated = temporary_test_backend_environment(inherited, &home);

        assert_eq!(
            isolated.get(OsStr::new("CUSTOM_ENV")),
            Some(&OsString::from("keep"))
        );
        assert_eq!(
            isolated.get(OsStr::new("KANDEV_HOME_DIR")),
            Some(&home.path.as_os_str().to_os_string())
        );
        assert_eq!(
            isolated.get(OsStr::new("KANDEV_DATABASE_DRIVER")),
            Some(&OsString::from("sqlite"))
        );
        assert_eq!(
            isolated.get(OsStr::new("KANDEV_DATABASE_PATH")),
            Some(&home.path.join("data/kandev.db").into_os_string())
        );
        assert_eq!(
            isolated.get(OsStr::new("KANDEV_INTERNAL_CONFIG_FILE")),
            Some(&home.path.join("config.yaml").into_os_string())
        );
        assert!(isolated.get(OsStr::new("KANDEV_DESKTOP_PORT")).is_none());
        assert!(isolated
            .get(OsStr::new("KANDEV_INTERNAL_CONFIG_HOME_FILE"))
            .is_none());
        assert_eq!(
            isolated.get(OsStr::new("KANDEV_DEBUG_DEV_MODE")),
            Some(&OsString::from("true"))
        );
        assert_eq!(
            isolated.get(OsStr::new("KANDEV_E2E_MOCK")),
            Some(&OsString::from("true"))
        );
        assert_eq!(
            isolated.get(OsStr::new("KANDEV_DESKTOP_RUNTIME_DIR")),
            Some(&OsString::from("/test/runtime"))
        );
        assert!(home.remove_if_owned());
    }

    #[test]
    fn temporary_home_cleanup_requires_ready_backend_and_clean_stop() {
        let state = BackendState::temporary_test_instance();
        let home = state
            .temporary_home_path()
            .expect("temporary test home path");

        assert!(!state.cleanup_temporary_home_after_stop(true));
        assert!(home.exists());
        state.mark_backend_ready();
        assert!(!state.cleanup_temporary_home_after_stop(false));
        assert!(home.exists());
        assert!(state.cleanup_temporary_home_after_stop(true));
        assert!(!home.exists());
    }

    #[test]
    fn temporary_home_is_retained_after_navigation_failure() {
        let state = BackendState::temporary_test_instance();
        let home = state
            .temporary_home_path()
            .expect("temporary test home path");
        state.mark_backend_ready();
        state.retain_temporary_home();

        assert!(!state.cleanup_temporary_home_after_stop(true));
        assert!(
            home.exists(),
            "failed navigation must retain temporary data"
        );
        fs::remove_dir_all(home).expect("remove retained test home");
    }

    #[cfg(unix)]
    #[test]
    fn temporary_home_is_retained_when_launcher_exits_unsuccessfully() {
        let state = BackendState::temporary_test_instance();
        let home = state
            .temporary_home_path()
            .expect("temporary test home path");
        state.mark_backend_ready();
        assert!(state.set_child(child_with_term_handler("exit 1")));

        state.stop();

        assert!(
            home.exists(),
            "failed launcher shutdown must retain its home"
        );
        fs::remove_dir_all(home).expect("remove retained test home");
    }

    #[test]
    fn desktop_stop_deadline_exceeds_the_launcher_shutdown_budget() {
        let launcher_grace = Duration::from_secs(75);
        let launcher_force_kill_wait = Duration::from_secs(2);

        assert!(
            DESKTOP_LAUNCHER_SHUTDOWN_TIMEOUT > launcher_grace + launcher_force_kill_wait,
            "desktop must allow the launcher to finish graceful and forced cleanup"
        );
    }

    #[cfg(unix)]
    #[test]
    fn temporary_home_cleanup_does_not_follow_a_replaced_symlink() {
        use std::os::unix::fs::symlink;

        let home = TemporaryHome::create().expect("temporary home");
        let outside =
            std::env::temp_dir().join(format!("kandev-test-outside-{}", std::process::id()));
        fs::create_dir(&outside).expect("outside directory");
        let outside_file = outside.join("keep.txt");
        fs::write(&outside_file, "keep").expect("outside file");
        fs::remove_dir_all(&home.path).expect("remove original home");
        symlink(&outside, &home.path).expect("replace home with symlink");

        assert!(!home.remove_if_owned());
        assert_eq!(
            fs::read_to_string(outside_file).expect("read outside file"),
            "keep"
        );

        fs::remove_file(&home.path).expect("remove test symlink");
        fs::remove_dir_all(outside).expect("remove outside fixture");
    }

    #[cfg(unix)]
    #[test]
    fn temporary_home_cleanup_requires_graceful_child_stop() {
        let mut child = child_with_term_handler("exit 0");
        assert!(terminate_child_with_timeout(
            &mut child,
            Duration::from_secs(1)
        ));

        let mut child = child_with_term_handler(":");
        assert!(!terminate_child_with_timeout(
            &mut child,
            Duration::from_millis(20)
        ));
    }

    #[cfg(unix)]
    fn child_with_term_handler(handler: &str) -> Child {
        use std::io::{BufRead, BufReader};

        let mut child = Command::new("sh")
            .arg("-c")
            .arg(format!(
                "trap '{handler}' TERM; printf 'ready\\n'; while :; do :; done"
            ))
            .stdout(Stdio::piped())
            .spawn()
            .expect("start termination test child");
        let mut output = BufReader::new(child.stdout.take().expect("child stdout"));
        let mut ready = String::new();
        output.read_line(&mut ready).expect("read child readiness");
        assert_eq!(ready, "ready\n");
        child
    }

    #[test]
    fn capture_stream_retries_interrupted_reads() {
        let output = Arc::new(Mutex::new(StartupOutput::default()));

        let reader_finished = capture_stream(
            "stdout",
            InterruptedThenData::new(b"backend ready"),
            output.clone(),
        );

        assert!(wait_for_startup_output_readers(
            vec![reader_finished],
            Duration::from_secs(1)
        ));
        assert!(output
            .lock()
            .expect("startup output mutex poisoned")
            .text()
            .is_some_and(|text| text.contains("backend ready")));
    }

    #[test]
    fn startup_conflict_waits_until_stderr_capture_finishes() {
        let output = Arc::new(Mutex::new(StartupOutput::default()));
        let marker = concat!(
            "KANDEV_DESKTOP_CONFLICT_V1 {\"version\":1,\"target_kind\":\"home\",",
            "\"target_path\":\"/tmp/kandev-home\",\"storage_kind\":\"sqlite_in_home\",",
            "\"database_path\":\"/tmp/kandev-home/data/kandev.db\"}\n"
        );
        let (release_tx, release_rx) = std::sync::mpsc::channel();
        let (reader_started_tx, reader_started_rx) = std::sync::mpsc::sync_channel(0);
        let reader_finished = capture_stream(
            "stderr",
            GatedReader {
                release: release_rx,
                started: Some(reader_started_tx),
                delivered: false,
            },
            output.clone(),
        );
        reader_started_rx
            .recv_timeout(Duration::from_secs(1))
            .expect("stderr reader should wait for its final chunk");

        let (drained_tx, drained_rx) = std::sync::mpsc::channel();
        let waiter = thread::spawn(move || {
            let drained =
                wait_for_startup_output_readers(vec![reader_finished], Duration::from_secs(1));
            drained_tx.send(drained).expect("send drain result");
        });
        assert!(
            matches!(
                drained_rx.recv_timeout(Duration::from_millis(50)),
                Err(std::sync::mpsc::RecvTimeoutError::Timeout)
            ),
            "conflict classification must wait for stderr EOF"
        );

        release_tx
            .send(marker.as_bytes().to_vec())
            .expect("release stderr");
        assert!(drained_rx
            .recv_timeout(Duration::from_secs(1))
            .expect("reader drain result"));
        waiter.join().expect("join output reader waiter");
        let output = output.lock().expect("startup output mutex poisoned");
        assert_eq!(
            output
                .startup_conflict()
                .map(|conflict| conflict.target_path),
            Some("/tmp/kandev-home".to_string())
        );
    }

    #[test]
    fn read_http_response_head_reads_past_short_first_chunk() {
        let mut reader = ShortReader::new(
            b"HTTP/1.1 200 OK\r\nx-kandev-desktop-health-token: token\r\n\r\nignored",
            4,
        );

        let prefix = read_http_response_head(&mut reader).expect("response head");

        assert!(prefix.starts_with(b"HTTP/1.1 200"));
        assert!(
            prefix.windows(4).any(|window| window == b"\r\n\r\n"),
            "response head should include the header terminator"
        );
    }

    #[test]
    fn response_header_check_requires_matching_desktop_health_token() {
        let response = b"HTTP/1.1 200 OK\r\nX-Kandev-Desktop-Health-Token: token\r\n\r\n";

        assert!(response_has_header(
            response,
            DESKTOP_HEALTH_TOKEN_HEADER,
            "token"
        ));
        assert!(!response_has_header(
            response,
            DESKTOP_HEALTH_TOKEN_HEADER,
            "other-token"
        ));
        assert!(!response_has_header(
            b"HTTP/1.1 200 OK\r\n\r\n",
            DESKTOP_HEALTH_TOKEN_HEADER,
            "token"
        ));
    }

    #[test]
    fn request_ready_accepts_response_without_health_token_header() {
        // readyHandler never sets X-Kandev-Desktop-Health-Token (that header
        // is /health-only); request_ready must accept a bare 2xx.
        let listener = TcpListener::bind((LOOPBACK_HOST, 0)).expect("bind ready listener");
        let port = listener.local_addr().expect("listener addr").port();
        thread::spawn(move || {
            if let Ok((mut stream, _)) = listener.accept() {
                let mut buf = [0_u8; 512];
                let _ = stream.read(&mut buf);
                let _ = stream.write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n");
            }
        });

        let ready = request_ready(port).expect("request_ready");
        assert!(
            ready,
            "request_ready must treat a bare 2xx as ready with no token check"
        );
    }

    #[test]
    fn request_ready_rejects_non_2xx_response() {
        let listener = TcpListener::bind((LOOPBACK_HOST, 0)).expect("bind ready listener");
        let port = listener.local_addr().expect("listener addr").port();
        thread::spawn(move || {
            if let Ok((mut stream, _)) = listener.accept() {
                let mut buf = [0_u8; 512];
                let _ = stream.read(&mut buf);
                let _ = stream
                    .write_all(b"HTTP/1.1 503 Service Unavailable\r\nContent-Length: 0\r\n\r\n");
            }
        });

        let ready = request_ready(port).expect("request_ready");
        assert!(
            !ready,
            "request_ready must reject a non-2xx response (bootstrap-not-ready-yet)"
        );
    }

    #[cfg(unix)]
    #[test]
    fn stop_terminates_tracked_child() {
        let state = BackendState::default();
        let child = Command::new("sh")
            .arg("-c")
            .arg("trap 'exit 0' TERM; while true; do sleep 1; done")
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .expect("spawn long-running child");
        let pid = child.id();

        assert!(state.set_child(child));
        state.stop();

        assert!(
            !process_exists(pid),
            "backend child {pid} should be terminated"
        );
    }

    #[cfg(unix)]
    #[test]
    fn wait_for_ready_rechecks_child_exit_after_success() {
        // The child exits well before the /ready response arrives, so a
        // wait_for_ready that trusted a bare "GET /ready succeeded" without
        // rechecking would return Ok for a backend that is already gone.
        let listener = TcpListener::bind((LOOPBACK_HOST, 0)).expect("bind ready listener");
        let port = listener.local_addr().expect("listener addr").port();
        thread::spawn(move || {
            if let Ok((mut stream, _)) = listener.accept() {
                let mut buf = [0_u8; 512];
                let _ = stream.read(&mut buf);
                thread::sleep(Duration::from_millis(300));
                let _ = stream.write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n");
            }
        });

        let state = BackendState::default();
        let child = Command::new("sh")
            .arg("-c")
            .arg("sleep 0.1")
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .expect("spawn short-lived child");
        assert!(state.set_child(child));

        let result = wait_for_ready(port, &state);

        assert!(
            result.is_err(),
            "wait_for_ready must not return Ok once the child has exited"
        );
        assert!(
            result
                .unwrap_err()
                .contains("exited before /ready reported ready"),
            "wait_for_ready error should name the exit-before-ready reason"
        );
    }

    #[cfg(unix)]
    #[test]
    fn set_child_terminates_child_when_shutdown_already_started() {
        let state = BackendState::default();
        let child = Command::new("sh")
            .arg("-c")
            .arg("trap 'exit 0' TERM; while true; do sleep 1; done")
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .expect("spawn long-running child");
        let pid = child.id();

        state.stop();

        assert!(!state.set_child(child));
        assert!(
            !process_exists(pid),
            "backend child {pid} should be terminated"
        );
    }

    fn temp_runtime_dir(name: &str) -> PathBuf {
        let dir = temp_root(name);
        let bin = dir.join("bin");
        fs::create_dir_all(&bin).expect("create bin");
        fs::write(bin.join(executable_name("kandev")), b"stub").expect("write launcher");
        fs::write(bin.join(executable_name("agentctl")), b"stub").expect("write agentctl");
        for &(name, label) in REMOTE_AGENTCTL_HELPERS.iter() {
            fs::write(bin.join(name), b"stub").unwrap_or_else(|err| panic!("write {label}: {err}"));
        }
        dir
    }

    fn desktop_helper_manifest(schema_version: u32, variant: &str) -> Vec<u8> {
        let helpers: Vec<_> = REMOTE_HELPER_MANIFEST_RECORDS
            .iter()
            .map(|(platform, asset)| {
                serde_json::json!({
                    "platform": platform,
                    "asset": asset,
                    "sha256": "a".repeat(64),
                    "size_bytes": 1,
                })
            })
            .collect();
        serde_json::to_vec(&serde_json::json!({
            "schema_version": schema_version,
            "version": "1.2.3",
            "commit": "a".repeat(40),
            "variant": variant,
            "helpers": helpers,
        }))
        .expect("serialize remote helper manifest")
    }

    fn temp_root(name: &str) -> PathBuf {
        let dir = env::temp_dir().join(format!("kandev-desktop-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).expect("create temp root");
        dir
    }

    #[cfg(unix)]
    fn process_exists(pid: u32) -> bool {
        unsafe { libc::kill(pid as i32, 0) == 0 }
    }

    struct ShortReader {
        data: &'static [u8],
        position: usize,
        chunk_size: usize,
    }

    struct GatedReader {
        release: std::sync::mpsc::Receiver<Vec<u8>>,
        started: Option<std::sync::mpsc::SyncSender<()>>,
        delivered: bool,
    }

    impl Read for GatedReader {
        fn read(&mut self, buffer: &mut [u8]) -> std::io::Result<usize> {
            if self.delivered {
                return Ok(0);
            }
            self.started
                .take()
                .expect("reader start sender")
                .send(())
                .expect("notify reader start");
            let bytes = self.release.recv().expect("release gated reader");
            let length = buffer.len().min(bytes.len());
            buffer[..length].copy_from_slice(&bytes[..length]);
            self.delivered = true;
            Ok(length)
        }
    }

    impl ShortReader {
        fn new(data: &'static [u8], chunk_size: usize) -> Self {
            Self {
                data,
                position: 0,
                chunk_size,
            }
        }
    }

    impl Read for ShortReader {
        fn read(&mut self, buffer: &mut [u8]) -> std::io::Result<usize> {
            if self.position >= self.data.len() {
                return Ok(0);
            }
            let len = buffer
                .len()
                .min(self.chunk_size)
                .min(self.data.len() - self.position);
            buffer[..len].copy_from_slice(&self.data[self.position..self.position + len]);
            self.position += len;
            Ok(len)
        }
    }

    struct InterruptedThenData {
        data: &'static [u8],
        interrupted: bool,
        drained: bool,
    }

    impl InterruptedThenData {
        fn new(data: &'static [u8]) -> Self {
            Self {
                data,
                interrupted: false,
                drained: false,
            }
        }
    }

    impl Read for InterruptedThenData {
        fn read(&mut self, buffer: &mut [u8]) -> std::io::Result<usize> {
            if !self.interrupted {
                self.interrupted = true;
                return Err(std::io::Error::from(ErrorKind::Interrupted));
            }
            if self.drained {
                return Ok(0);
            }
            let len = buffer.len().min(self.data.len());
            buffer[..len].copy_from_slice(&self.data[..len]);
            self.drained = true;
            Ok(len)
        }
    }
}
