use std::{
    collections::BTreeMap,
    env,
    ffi::{OsStr, OsString},
    io::{Read, Write},
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

#[cfg(feature = "desktop-runtime")]
use tauri::{AppHandle, Manager, WebviewWindow};

const HEALTH_TIMEOUT: Duration = Duration::from_secs(60);
const LOOPBACK_HOST: &str = "127.0.0.1";
const STARTUP_OUTPUT_LIMIT: usize = 12 * 1024;

#[derive(Clone)]
pub struct BackendState {
    child: Arc<Mutex<Option<Child>>>,
    startup_output: Arc<Mutex<StartupOutput>>,
    shutdown_started: Arc<AtomicBool>,
}

impl Default for BackendState {
    fn default() -> Self {
        Self {
            child: Arc::new(Mutex::new(None)),
            startup_output: Arc::new(Mutex::new(StartupOutput::default())),
            shutdown_started: Arc::new(AtomicBool::new(false)),
        }
    }
}

impl BackendState {
    pub fn begin_shutdown(&self) -> bool {
        !self.shutdown_started.swap(true, Ordering::SeqCst)
    }

    pub fn stop(&self) {
        self.shutdown_started.store(true, Ordering::SeqCst);
        let child = self.child.lock().expect("backend child mutex poisoned").take();
        if let Some(mut child) = child {
            terminate_child(&mut child);
        }
    }

    fn is_shutdown_started(&self) -> bool {
        self.shutdown_started.load(Ordering::SeqCst)
    }

    fn set_child(&self, child: Child) {
        let mut guard = self.child.lock().expect("backend child mutex poisoned");
        *guard = Some(child);
    }

    fn reset_startup_output(&self) {
        self.startup_output
            .lock()
            .expect("startup output mutex poisoned")
            .clear();
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
                thread::sleep(Duration::from_millis(50));
                return Ok(Some(launcher_exit_message(
                    &status.to_string(),
                    self.recent_startup_output(),
                )));
            }
        }
        Ok(None)
    }
}

#[derive(Default)]
struct StartupOutput {
    bytes: Vec<u8>,
}

impl StartupOutput {
    fn clear(&mut self) {
        self.bytes.clear();
    }

    fn push(&mut self, stream: &str, chunk: &[u8]) {
        if chunk.is_empty() {
            return;
        }

        self.bytes
            .extend_from_slice(format!("\n[{stream}] ").as_bytes());
        self.bytes.extend_from_slice(chunk);
        if self.bytes.len() > STARTUP_OUTPUT_LIMIT {
            let overflow = self.bytes.len() - STARTUP_OUTPUT_LIMIT;
            self.bytes.drain(0..overflow);
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
        set_status(
            &window,
            "Starting backend",
            "Preparing the local runtime and opening your workspace.",
            false,
        );
        match launch_and_wait(&app, &state) {
            Ok(url) => {
                if let Err(err) = navigate_to_backend(&window, &url) {
                    state.stop();
                    set_status(
                        &window,
                        "Desktop startup failed",
                        &format!("Backend started, but the window could not navigate: {err}"),
                        true,
                    );
                }
            }
            Err(err) => {
                state.stop();
                set_status(&window, "Desktop startup failed", &err, true);
            }
        }
    });
}

#[cfg(feature = "desktop-runtime")]
fn launch_and_wait(app: &AppHandle, state: &BackendState) -> Result<String, String> {
    let runtime_dir = resolve_runtime_dir(app)?;
    let port = pick_loopback_port()?;
    let spec = build_backend_command(
        &runtime_dir,
        port,
        env::vars_os().collect(),
        current_home_dir().as_deref(),
    )?;
    state.reset_startup_output();
    let mut child = spawn_backend_command(&spec)?;
    capture_child_output(&mut child, state.startup_output.clone());
    state.set_child(child);
    wait_for_backend(port, state, HEALTH_TIMEOUT)?;
    Ok(format!("http://{LOOPBACK_HOST}:{port}/"))
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
        .ok_or_else(|| format!("Kandev launcher has no parent directory: {}", program.display()))?
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
    require_runtime_file(&bin_dir.join(executable_name("kandev")), "Kandev launcher binary")?;
    require_runtime_file(&bin_dir.join(executable_name("agentctl")), "agentctl binary")?;
    if requires_agentctl_linux_amd64(env::consts::OS, env::consts::ARCH) {
        require_runtime_file(&bin_dir.join("agentctl-linux-amd64"), "agentctl linux/amd64 helper")?;
    }
    Ok(())
}

fn require_runtime_file(path: &Path, label: &str) -> Result<(), String> {
    if path.is_file() {
        Ok(())
    } else {
        Err(format!("{label} is missing at {}", path.display()))
    }
}

fn requires_agentctl_linux_amd64(os: &str, arch: &str) -> bool {
    os != "linux" || arch != "x86_64"
}

pub fn desktop_environment(
    runtime_dir: &Path,
    mut env: BTreeMap<OsString, OsString>,
    home_dir: Option<&Path>,
) -> BTreeMap<OsString, OsString> {
    let path = normalized_path(env.get(OsStr::new("PATH")), home_dir);
    env.insert(
        OsString::from("KANDEV_SERVER_HOST"),
        OsString::from(LOOPBACK_HOST),
    );
    env.insert(
        OsString::from("KANDEV_BUNDLE_DIR"),
        runtime_dir.as_os_str().to_os_string(),
    );
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

fn capture_child_output(child: &mut Child, output: Arc<Mutex<StartupOutput>>) {
    if let Some(stdout) = child.stdout.take() {
        capture_stream("stdout", stdout, output.clone());
    }
    if let Some(stderr) = child.stderr.take() {
        capture_stream("stderr", stderr, output);
    }
}

fn capture_stream<R>(stream: &'static str, mut reader: R, output: Arc<Mutex<StartupOutput>>)
where
    R: Read + Send + 'static,
{
    thread::spawn(move || {
        let mut buffer = [0_u8; 1024];
        loop {
            match reader.read(&mut buffer) {
                Ok(0) | Err(_) => return,
                Ok(n) => output
                    .lock()
                    .expect("startup output mutex poisoned")
                    .push(stream, &buffer[..n]),
            }
        }
    });
}

fn wait_for_backend(port: u16, state: &BackendState, timeout: Duration) -> Result<(), String> {
    let deadline = Instant::now() + timeout;
    loop {
        if state.is_shutdown_started() {
            return Err("Desktop startup cancelled".to_string());
        }
        if health_ready(port) {
            return Ok(());
        }
        if let Some(message) = state.child_exit_message()? {
            return Err(format!("Kandev launcher exited before /health became ready ({message})"));
        }
        if Instant::now() >= deadline {
            let mut message = format!("Timed out waiting for http://{LOOPBACK_HOST}:{port}/health");
            append_recent_output(&mut message, state.recent_startup_output());
            return Err(message);
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

fn health_ready(port: u16) -> bool {
    request_health(port).unwrap_or(false)
}

fn request_health(port: u16) -> Result<bool, String> {
    let addr = (LOOPBACK_HOST, port)
        .to_socket_addrs()
        .map_err(|err| err.to_string())?
        .next()
        .ok_or_else(|| format!("Could not resolve {LOOPBACK_HOST}:{port}"))?;
    let mut stream =
        TcpStream::connect_timeout(&addr, Duration::from_millis(250)).map_err(|err| err.to_string())?;
    stream
        .set_read_timeout(Some(Duration::from_millis(250)))
        .map_err(|err| err.to_string())?;
    stream
        .write_all(
            format!("GET /health HTTP/1.1\r\nHost: {LOOPBACK_HOST}:{port}\r\nConnection: close\r\n\r\n")
                .as_bytes(),
        )
        .map_err(|err| err.to_string())?;
    let response = read_http_status_prefix(&mut stream)?;
    Ok(response.starts_with(b"HTTP/1.1 200") || response.starts_with(b"HTTP/1.0 200"))
}

fn read_http_status_prefix<R: Read>(reader: &mut R) -> Result<Vec<u8>, String> {
    let mut response = Vec::with_capacity(64);
    let mut buffer = [0_u8; 16];
    while response.len() < 64 {
        let read_len = buffer.len().min(64 - response.len());
        let n = reader
            .read(&mut buffer[..read_len])
            .map_err(|err| err.to_string())?;
        if n == 0 {
            break;
        }
        response.extend_from_slice(&buffer[..n]);
        if response.windows(2).any(|window| window == b"\r\n") {
            break;
        }
    }
    Ok(response)
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

fn executable_name(name: &str) -> OsString {
    if cfg!(windows) {
        OsString::from(format!("{name}.exe"))
    } else {
        OsString::from(name)
    }
}

#[cfg(feature = "desktop-runtime")]
fn set_status(window: &WebviewWindow, title: &str, detail: &str, failed: bool) {
    let payload = serde_json::json!({
        "title": title,
        "detail": detail,
        "failed": failed,
    });
    let script = format!("window.__KANDEV_DESKTOP_SET_STATUS?.({payload});");
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
fn terminate_child(child: &mut Child) {
    let _ = unsafe { libc::kill(child.id() as i32, libc::SIGTERM) };
    wait_or_kill(child, Duration::from_secs(5));
}

#[cfg(windows)]
fn terminate_child(child: &mut Child) {
    wait_or_kill(child, Duration::from_secs(0));
}

#[cfg(not(any(unix, windows)))]
fn terminate_child(child: &mut Child) {
    wait_or_kill(child, Duration::from_secs(0));
}

fn wait_or_kill(child: &mut Child, graceful_timeout: Duration) {
    let deadline = Instant::now() + graceful_timeout;
    loop {
        match child.try_wait() {
            Ok(Some(_)) => return,
            Ok(None) if Instant::now() < deadline => thread::sleep(Duration::from_millis(100)),
            Ok(None) | Err(_) => break,
        }
    }
    force_kill_child(child);
    let _ = child.wait();
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
        inherited.insert(OsString::from("PATH"), OsString::from("/existing/bin"));

        let spec = build_backend_command(
            &runtime_dir,
            48123,
            inherited,
            Some(Path::new("/home/example")),
        )
        .expect("command spec");

        assert_eq!(spec.program, runtime_dir.join("bin").join(executable_name("kandev")));
        assert_eq!(
            spec.args,
            vec![
                OsString::from("--headless"),
                OsString::from("--port"),
                OsString::from("48123"),
            ]
        );
        assert_eq!(spec.env.get(OsStr::new("CUSTOM_ENV")), Some(&OsString::from("kept")));
        assert_eq!(
            spec.env.get(OsStr::new("KANDEV_SERVER_HOST")),
            Some(&OsString::from(LOOPBACK_HOST))
        );
        assert_eq!(
            spec.env.get(OsStr::new("KANDEV_BUNDLE_DIR")),
            Some(&runtime_dir.as_os_str().to_os_string())
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
    fn missing_launcher_returns_readable_error() {
        let dir = temp_root("missing-launcher");
        let err = build_backend_command(&dir, 48123, BTreeMap::new(), None).expect_err("missing launcher");
        assert!(err.contains("Kandev launcher binary is missing"), "{err}");
    }

    #[test]
    fn missing_agentctl_returns_readable_error() {
        let dir = temp_root("missing-agentctl");
        let bin = dir.join("bin");
        fs::create_dir_all(&bin).expect("create bin");
        fs::write(bin.join(executable_name("kandev")), b"stub").expect("write launcher");

        let err = build_backend_command(&dir, 48123, BTreeMap::new(), None).expect_err("missing agentctl");

        assert!(err.contains("agentctl binary is missing"), "{err}");
    }

    #[test]
    fn linux_amd64_helper_requirement_matches_native_launcher() {
        assert!(!requires_agentctl_linux_amd64("linux", "x86_64"));
        assert!(requires_agentctl_linux_amd64("linux", "aarch64"));
        assert!(requires_agentctl_linux_amd64("macos", "aarch64"));
        assert!(requires_agentctl_linux_amd64("windows", "x86_64"));
    }

    #[test]
    fn pick_loopback_port_returns_valid_port() {
        let port = pick_loopback_port().expect("loopback port");
        assert_ne!(port, 0);
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
    fn read_http_status_prefix_reads_past_short_first_chunk() {
        let mut reader = ShortReader::new(b"HTTP/1.1 200 OK\r\nignored", 4);

        let prefix = read_http_status_prefix(&mut reader).expect("status prefix");

        assert!(prefix.starts_with(b"HTTP/1.1 200"));
        assert!(
            prefix.windows(2).any(|window| window == b"\r\n"),
            "status prefix should include the line terminator"
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

        state.set_child(child);
        state.stop();

        assert!(!process_exists(pid), "backend child {pid} should be terminated");
    }

    fn temp_runtime_dir(name: &str) -> PathBuf {
        let dir = temp_root(name);
        let bin = dir.join("bin");
        fs::create_dir_all(&bin).expect("create bin");
        fs::write(bin.join(executable_name("kandev")), b"stub").expect("write launcher");
        fs::write(bin.join(executable_name("agentctl")), b"stub").expect("write agentctl");
        if requires_agentctl_linux_amd64(env::consts::OS, env::consts::ARCH) {
            fs::write(bin.join("agentctl-linux-amd64"), b"stub").expect("write linux helper");
        }
        dir
    }

    fn temp_root(name: &str) -> PathBuf {
        let dir = env::temp_dir().join(format!(
            "kandev-desktop-{name}-{}",
            std::process::id()
        ));
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
}
