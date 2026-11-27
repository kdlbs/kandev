use serde::Serialize;
use std::{path::Path, sync::Mutex, time::Duration};

#[cfg(feature = "desktop-runtime")]
use crate::backend::BackendState;
#[cfg(feature = "desktop-runtime")]
use std::time::{Instant, SystemTime, UNIX_EPOCH};
#[cfg(feature = "desktop-runtime")]
use tauri::{AppHandle, Manager, State, WebviewWindow};
#[cfg(feature = "desktop-runtime")]
use tauri_plugin_updater::{Update, UpdaterExt};

pub const AUTOMATIC_CHECK_INTERVAL: Duration = Duration::from_secs(24 * 60 * 60);
const LINUX_INSTALL_UNSUPPORTED: &str = "Automatic update installation is available only when Kandev is running from an AppImage. For .deb or .rpm installations, download the latest package from Release notes and update it with your package manager.";
const NO_UPDATE_READY: &str = "No desktop update is ready to install.";

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "kebab-case")]
pub enum UpdatePhase {
    Idle,
    Checking,
    Available,
    UpToDate,
    Downloading,
    Installing,
    Error,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateSnapshot {
    pub phase: UpdatePhase,
    pub current_version: String,
    pub latest_version: Option<String>,
    pub release_notes: Option<String>,
    pub release_url: Option<String>,
    pub checked_at_epoch_ms: Option<u64>,
    pub downloaded_bytes: Option<u64>,
    pub total_bytes: Option<u64>,
    pub install_supported: bool,
    pub install_unsupported_reason: Option<String>,
    pub error: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct InstallSupport {
    supported: bool,
    unsupported_reason: Option<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum UpdatePlatform {
    Linux,
    MacOs,
    Windows,
    Other,
}

struct InstallEnvironment<'a> {
    appimage: Option<&'a Path>,
    appdir: Option<&'a Path>,
    current_exe: Option<&'a Path>,
    temp_dir: &'a Path,
    appimage_is_file: bool,
}

impl InstallSupport {
    fn supported() -> Self {
        Self {
            supported: true,
            unsupported_reason: None,
        }
    }

    fn unsupported(reason: impl Into<String>) -> Self {
        Self {
            supported: false,
            unsupported_reason: Some(reason.into()),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AvailableUpdate {
    pub version: String,
    pub release_notes: Option<String>,
    pub release_url: Option<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Operation {
    Check,
    Install,
}

struct Machine<U> {
    snapshot: UpdateSnapshot,
    pending: Option<U>,
    operation: Option<Operation>,
    last_automatic_check: Option<Duration>,
}

#[cfg(feature = "desktop-runtime")]
pub type UpdaterState = UpdateState<Update>;
#[cfg(not(feature = "desktop-runtime"))]
pub type UpdaterState = UpdateState<()>;

pub struct UpdateState<U> {
    machine: Mutex<Machine<U>>,
    install_enabled: bool,
}

impl<U> UpdateState<U> {
    pub fn new(current_version: impl Into<String>) -> Self {
        Self::new_with_install_enabled(current_version, true)
    }

    pub fn new_with_install_enabled(
        current_version: impl Into<String>,
        install_enabled: bool,
    ) -> Self {
        let support = if install_enabled {
            native_install_support()
        } else {
            InstallSupport::unsupported(
                "Update installation is disabled for temporary test instances.",
            )
        };
        Self::with_install_policy(current_version, support, install_enabled)
    }

    #[cfg(test)]
    fn with_install_support(
        current_version: impl Into<String>,
        install_support: InstallSupport,
    ) -> Self {
        Self::with_install_policy(current_version, install_support, true)
    }

    fn with_install_policy(
        current_version: impl Into<String>,
        install_support: InstallSupport,
        install_enabled: bool,
    ) -> Self {
        Self {
            machine: Mutex::new(Machine {
                snapshot: UpdateSnapshot {
                    phase: UpdatePhase::Idle,
                    current_version: current_version.into(),
                    latest_version: None,
                    release_notes: None,
                    release_url: None,
                    checked_at_epoch_ms: None,
                    downloaded_bytes: None,
                    total_bytes: None,
                    install_supported: install_support.supported,
                    install_unsupported_reason: install_support.unsupported_reason,
                    error: None,
                },
                pending: None,
                operation: None,
                last_automatic_check: None,
            }),
            install_enabled,
        }
    }

    pub fn should_run_automatic_check(&self, elapsed: Duration) -> bool {
        let mut machine = self.machine.lock().expect("updater state mutex poisoned");
        let due = machine
            .last_automatic_check
            .map(|last| elapsed.saturating_sub(last) >= AUTOMATIC_CHECK_INTERVAL)
            .unwrap_or(true);
        if due {
            machine.last_automatic_check = Some(elapsed);
        }
        due
    }

    pub fn begin_check(&self) -> Result<(), String> {
        let mut machine = self.machine.lock().expect("updater state mutex poisoned");
        ensure_idle(machine.operation)?;
        machine.operation = Some(Operation::Check);
        machine.snapshot.phase = UpdatePhase::Checking;
        machine.snapshot.error = None;
        machine.snapshot.downloaded_bytes = None;
        machine.snapshot.total_bytes = None;
        Ok(())
    }

    pub fn begin_install(&self) -> Result<U, String>
    where
        U: Clone,
    {
        let mut guard = self.machine.lock().expect("updater state mutex poisoned");
        let machine = &mut *guard;
        let update = machine
            .pending
            .as_ref()
            .ok_or_else(|| NO_UPDATE_READY.to_string())?;
        if !self.install_enabled {
            return Err(
                "Update installation is disabled for temporary test instances.".to_string(),
            );
        }
        ensure_idle(machine.operation)?;
        if machine.snapshot.phase != UpdatePhase::Available {
            return Err(NO_UPDATE_READY.to_string());
        }
        if !machine.snapshot.install_supported {
            return Err(machine
                .snapshot
                .install_unsupported_reason
                .clone()
                .unwrap_or_else(|| "Automatic update installation is unavailable.".to_string()));
        }
        machine.operation = Some(Operation::Install);
        machine.snapshot.phase = UpdatePhase::Downloading;
        machine.snapshot.error = None;
        machine.snapshot.downloaded_bytes = Some(0);
        machine.snapshot.total_bytes = None;
        Ok(update.clone())
    }

    pub fn snapshot(&self) -> UpdateSnapshot {
        self.machine
            .lock()
            .expect("updater state mutex poisoned")
            .snapshot
            .clone()
    }

    pub fn complete_check(&self, update: Option<(AvailableUpdate, U)>, checked_at_epoch_ms: u64) {
        let (update, pending) = update.unzip();
        let mut machine = self.machine.lock().expect("updater state mutex poisoned");
        machine.pending = pending;
        machine.operation = None;
        machine.snapshot.checked_at_epoch_ms = Some(checked_at_epoch_ms);
        machine.snapshot.downloaded_bytes = None;
        machine.snapshot.total_bytes = None;
        machine.snapshot.error = None;
        match update {
            Some(update) => {
                machine.snapshot.phase = UpdatePhase::Available;
                machine.snapshot.latest_version = Some(update.version);
                machine.snapshot.release_notes = update.release_notes;
                machine.snapshot.release_url = update.release_url;
            }
            None => {
                machine.snapshot.phase = UpdatePhase::UpToDate;
                machine.snapshot.latest_version = None;
                machine.snapshot.release_notes = None;
                machine.snapshot.release_url = None;
            }
        }
    }

    pub fn update_download_progress(&self, chunk_bytes: usize, total_bytes: Option<u64>) {
        let mut machine = self.machine.lock().expect("updater state mutex poisoned");
        let downloaded = machine.snapshot.downloaded_bytes.unwrap_or(0);
        machine.snapshot.downloaded_bytes = Some(downloaded.saturating_add(chunk_bytes as u64));
        if total_bytes.is_some() {
            machine.snapshot.total_bytes = total_bytes;
        }
    }

    pub fn mark_installing(&self) {
        self.machine
            .lock()
            .expect("updater state mutex poisoned")
            .snapshot
            .phase = UpdatePhase::Installing;
    }

    pub fn fail_operation(&self, message: impl Into<String>) {
        let mut machine = self.machine.lock().expect("updater state mutex poisoned");
        let operation = machine.operation.take();
        machine.snapshot.phase = if machine.snapshot.latest_version.is_some()
            && matches!(operation, Some(Operation::Check | Operation::Install))
        {
            UpdatePhase::Available
        } else {
            UpdatePhase::Error
        };
        machine.snapshot.error = Some(message.into());
    }
}

fn native_install_support() -> InstallSupport {
    let appimage = std::env::var_os("APPIMAGE").map(std::path::PathBuf::from);
    let appdir = std::env::var_os("APPDIR").map(std::path::PathBuf::from);
    let current_exe = std::env::current_exe().ok();
    let temp_dir = std::env::temp_dir();
    let environment = InstallEnvironment {
        appimage: appimage.as_deref(),
        appdir: appdir.as_deref(),
        current_exe: current_exe.as_deref(),
        temp_dir: &temp_dir,
        appimage_is_file: appimage.as_ref().is_some_and(|path| path.is_file()),
    };
    install_support_for(current_platform(), environment)
}

fn current_platform() -> UpdatePlatform {
    match std::env::consts::OS {
        "linux" => UpdatePlatform::Linux,
        "macos" => UpdatePlatform::MacOs,
        "windows" => UpdatePlatform::Windows,
        _ => UpdatePlatform::Other,
    }
}

fn install_support_for(
    platform: UpdatePlatform,
    environment: InstallEnvironment<'_>,
) -> InstallSupport {
    match platform {
        UpdatePlatform::MacOs | UpdatePlatform::Windows => InstallSupport::supported(),
        UpdatePlatform::Linux if is_validated_appimage(&environment) => InstallSupport::supported(),
        UpdatePlatform::Linux => InstallSupport::unsupported(LINUX_INSTALL_UNSUPPORTED),
        UpdatePlatform::Other => InstallSupport::unsupported(
            "Automatic update installation is not supported on this operating system.",
        ),
    }
}

fn is_validated_appimage(environment: &InstallEnvironment<'_>) -> bool {
    let (Some(appimage), Some(appdir), Some(current_exe)) = (
        environment.appimage,
        environment.appdir,
        environment.current_exe,
    ) else {
        return false;
    };
    appimage.is_absolute()
        && environment.appimage_is_file
        && appdir.is_absolute()
        && appdir.starts_with(environment.temp_dir)
        && appdir
            .file_name()
            .is_some_and(|name| name.to_string_lossy().starts_with(".mount_"))
        && current_exe.starts_with(appdir)
}

fn ensure_idle(operation: Option<Operation>) -> Result<(), String> {
    if operation.is_some() {
        Err("Another update operation is already in progress.".to_string())
    } else {
        Ok(())
    }
}

pub trait BackendShutdown {
    fn stop_owned_backend(&self);
}

pub fn prepare_updater_restart(backend: &impl BackendShutdown) {
    backend.stop_owned_backend();
}

#[cfg(feature = "desktop-runtime")]
impl BackendShutdown for BackendState {
    fn stop_owned_backend(&self) {
        self.stop();
    }
}

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub fn get_update_state(
    state: State<'_, UpdaterState>,
    backend: State<'_, BackendState>,
    webview: WebviewWindow,
) -> Result<UpdateSnapshot, String> {
    backend.require_owned_origin(&webview)?;
    Ok(state.snapshot())
}

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub async fn check_for_updates(
    app: AppHandle,
    state: State<'_, UpdaterState>,
    backend: State<'_, BackendState>,
    webview: WebviewWindow,
) -> Result<UpdateSnapshot, String> {
    backend.require_owned_origin(&webview)?;
    run_check(&app, &state).await
}

#[cfg(feature = "desktop-runtime")]
#[tauri::command]
pub async fn install_update(
    app: AppHandle,
    state: State<'_, UpdaterState>,
    backend: State<'_, BackendState>,
    webview: WebviewWindow,
) -> Result<UpdateSnapshot, String> {
    backend.require_owned_origin(&webview)?;
    let update = state.begin_install()?;
    let download_result = update
        .download_and_install(
            |chunk_bytes, total_bytes| {
                state.update_download_progress(chunk_bytes, total_bytes);
            },
            || state.mark_installing(),
        )
        .await;
    if let Err(err) = download_result {
        let message = format!("Desktop update installation failed: {err}");
        state.fail_operation(&message);
        return Err(message);
    }

    let backend = app.state::<BackendState>().inner().clone();
    prepare_updater_restart(&backend);
    app.restart();
}

#[cfg(feature = "desktop-runtime")]
pub fn start_automatic_checks(app: AppHandle) {
    std::thread::spawn(move || {
        let started = Instant::now();
        loop {
            let state = app.state::<UpdaterState>();
            if state.should_run_automatic_check(started.elapsed()) {
                let _ = tauri::async_runtime::block_on(run_check(&app, &state));
            }
            std::thread::sleep(AUTOMATIC_CHECK_INTERVAL);
        }
    });
}

#[cfg(feature = "desktop-runtime")]
async fn run_check(app: &AppHandle, state: &UpdaterState) -> Result<UpdateSnapshot, String> {
    state.begin_check()?;
    let backend = app.state::<BackendState>().inner().clone();
    let updater = match app
        .updater_builder()
        .on_before_exit(move || backend.stop())
        .build()
    {
        Ok(updater) => updater,
        Err(err) => return fail_check(state, err),
    };
    match updater.check().await {
        Ok(update) => {
            let release = update.map(|update| {
                let metadata = AvailableUpdate {
                    version: update.version.clone(),
                    release_notes: update.body.clone(),
                    release_url: Some(release_url(&update.version)),
                };
                (metadata, update)
            });
            state.complete_check(release, now_epoch_ms());
            Ok(state.snapshot())
        }
        Err(err) => fail_check(state, err),
    }
}

#[cfg(feature = "desktop-runtime")]
fn fail_check(
    state: &UpdaterState,
    error: impl std::fmt::Display,
) -> Result<UpdateSnapshot, String> {
    let message = format!("Desktop update check failed: {error}");
    state.fail_operation(&message);
    Err(message)
}

#[cfg(feature = "desktop-runtime")]
fn release_url(version: &str) -> String {
    let tag = if version.starts_with('v') {
        version.to_string()
    } else {
        format!("v{version}")
    };
    format!("https://github.com/kdlbs/kandev/releases/tag/{tag}")
}

#[cfg(feature = "desktop-runtime")]
fn now_epoch_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis()
        .try_into()
        .unwrap_or(u64::MAX)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::rc::Rc;
    use std::sync::atomic::{AtomicUsize, Ordering};

    type TestUpdaterState = UpdateState<()>;

    fn install_environment<'a>(
        appimage: Option<&'a Path>,
        appdir: Option<&'a Path>,
        current_exe: Option<&'a Path>,
        appimage_is_file: bool,
    ) -> InstallEnvironment<'a> {
        InstallEnvironment {
            appimage,
            appdir,
            current_exe,
            temp_dir: Path::new("/tmp"),
            appimage_is_file,
        }
    }

    fn supported_updater_state() -> TestUpdaterState {
        TestUpdaterState::with_install_support("1.0.0", InstallSupport::supported())
    }

    fn publish_available_update(state: &TestUpdaterState) {
        state.begin_check().unwrap();
        state.complete_check(
            Some((
                AvailableUpdate {
                    version: "1.1.0".to_string(),
                    release_notes: None,
                    release_url: None,
                },
                (),
            )),
            42,
        );
    }

    #[test]
    fn automatic_checks_run_immediately_then_no_more_than_once_per_day() {
        let state = TestUpdaterState::new("1.0.0");

        assert!(state.should_run_automatic_check(Duration::ZERO));
        assert!(
            !state.should_run_automatic_check(AUTOMATIC_CHECK_INTERVAL - Duration::from_secs(1))
        );
        assert!(state.should_run_automatic_check(AUTOMATIC_CHECK_INTERVAL));
    }

    #[test]
    fn temporary_test_instance_cannot_install_updates() {
        let state = TestUpdaterState::new_with_install_enabled("1.0.0", false);
        publish_available_update(&state);

        assert_eq!(
            state.begin_install(),
            Err("Update installation is disabled for temporary test instances.".to_string())
        );
    }

    #[test]
    fn forged_appimage_environment_is_not_installable() {
        let support = install_support_for(
            UpdatePlatform::Linux,
            install_environment(
                Some(Path::new("/opt/Kandev.AppImage")),
                Some(Path::new("/tmp/.mount_kandev")),
                Some(Path::new("/usr/bin/kandev-desktop")),
                false,
            ),
        );

        assert!(!support.supported);
    }

    #[test]
    fn validated_appimage_and_native_desktop_platforms_are_installable() {
        let valid_appimage = install_environment(
            Some(Path::new("/opt/Kandev.AppImage")),
            Some(Path::new("/tmp/.mount_kandev")),
            Some(Path::new("/tmp/.mount_kandev/usr/bin/kandev-desktop")),
            true,
        );

        assert!(install_support_for(UpdatePlatform::Linux, valid_appimage).supported);
        assert!(
            install_support_for(
                UpdatePlatform::MacOs,
                install_environment(None, None, None, false)
            )
            .supported
        );
        assert!(
            install_support_for(
                UpdatePlatform::Windows,
                install_environment(None, None, None, false)
            )
            .supported
        );
    }

    #[test]
    fn linux_package_install_exposes_manual_update_guidance() {
        let support = install_support_for(
            UpdatePlatform::Linux,
            install_environment(None, None, None, false),
        );

        assert!(!support.supported);
        assert!(support
            .unsupported_reason
            .as_deref()
            .unwrap_or_default()
            .contains("package manager"));
    }

    #[test]
    fn only_one_update_operation_can_be_in_flight() {
        let state = TestUpdaterState::new("1.0.0");

        publish_available_update(&state);
        state.begin_check().expect("refresh should start");

        assert_eq!(
            state.begin_check(),
            Err("Another update operation is already in progress.".to_string())
        );
        assert_eq!(
            state.begin_install(),
            Err("Another update operation is already in progress.".to_string())
        );
    }

    #[test]
    fn install_reserves_the_release_it_clones() {
        struct FakeUpdate {
            version: &'static str,
            before_clone: Option<Box<dyn Fn()>>,
        }

        impl Clone for FakeUpdate {
            fn clone(&self) -> Self {
                if let Some(before_clone) = &self.before_clone {
                    before_clone();
                }
                Self {
                    version: self.version,
                    before_clone: None,
                }
            }
        }

        fn publish(
            state: &UpdateState<FakeUpdate>,
            version: &'static str,
            before_clone: Option<Box<dyn Fn()>>,
        ) {
            state.complete_check(
                Some((
                    AvailableUpdate {
                        version: version.to_string(),
                        release_notes: None,
                        release_url: None,
                    },
                    FakeUpdate {
                        version,
                        before_clone,
                    },
                )),
                42,
            );
        }

        let state = Rc::new(UpdateState::with_install_support(
            "1.0.0",
            InstallSupport::supported(),
        ));
        let weak_state = Rc::downgrade(&state);
        state.begin_check().unwrap();
        publish(
            &state,
            "1.1.0",
            Some(Box::new(move || {
                let state = weak_state.upgrade().unwrap();
                // Publish the second release during the clone if the check can start.
                let can_start_check = state.machine.try_lock().is_ok();
                if can_start_check {
                    state.begin_check().unwrap();
                    publish(&state, "1.2.0", None);
                }
            })),
        );

        let update = state.begin_install().unwrap();
        let snapshot = state.snapshot();
        assert_eq!(snapshot.latest_version.as_deref(), Some(update.version));
        assert_eq!(update.version, "1.1.0");
        assert_eq!(snapshot.phase, UpdatePhase::Downloading);
        assert!(state.begin_check().is_err());

        state.fail_operation("Download failed");
        state.begin_check().unwrap();
        assert!(state.begin_install().is_err());
        publish(&state, "1.2.0", None);

        let update = state.begin_install().unwrap();
        assert_eq!(
            state.snapshot().latest_version.as_deref(),
            Some(update.version)
        );
        assert_eq!(update.version, "1.2.0");
    }

    #[test]
    fn install_requires_a_confirmed_available_update() {
        let state = TestUpdaterState::new("1.0.0");

        assert_eq!(
            state.begin_install(),
            Err("No desktop update is ready to install.".to_string())
        );
        assert_eq!(state.snapshot().phase, UpdatePhase::Idle);
    }

    #[cfg(target_os = "linux")]
    #[test]
    fn install_is_blocked_outside_a_validated_appimage_runtime() {
        let state = TestUpdaterState::new("1.0.0");
        publish_available_update(&state);

        assert_eq!(
            state.begin_install(),
            Err(LINUX_INSTALL_UNSUPPORTED.to_string())
        );
        assert_eq!(state.snapshot().phase, UpdatePhase::Available);
    }

    #[test]
    fn updater_restart_stops_the_owned_backend_once() {
        struct FakeBackend(AtomicUsize);
        impl BackendShutdown for FakeBackend {
            fn stop_owned_backend(&self) {
                self.0.fetch_add(1, Ordering::SeqCst);
            }
        }
        let backend = FakeBackend(AtomicUsize::new(0));

        prepare_updater_restart(&backend);

        assert_eq!(backend.0.load(Ordering::SeqCst), 1);
    }

    #[test]
    fn successful_checks_publish_available_and_no_update_states() {
        let state = supported_updater_state();
        state.begin_check().unwrap();
        state.complete_check(
            Some((
                AvailableUpdate {
                    version: "1.1.0".to_string(),
                    release_notes: Some("Changes".to_string()),
                    release_url: Some("https://example.test/v1.1.0".to_string()),
                },
                (),
            )),
            42,
        );

        assert_eq!(
            state.snapshot(),
            UpdateSnapshot {
                phase: UpdatePhase::Available,
                current_version: "1.0.0".to_string(),
                latest_version: Some("1.1.0".to_string()),
                release_notes: Some("Changes".to_string()),
                release_url: Some("https://example.test/v1.1.0".to_string()),
                checked_at_epoch_ms: Some(42),
                downloaded_bytes: None,
                total_bytes: None,
                install_supported: true,
                install_unsupported_reason: None,
                error: None,
            }
        );

        state.begin_check().unwrap();
        state.complete_check(None, 84);
        let snapshot = state.snapshot();
        assert_eq!(snapshot.phase, UpdatePhase::UpToDate);
        assert_eq!(snapshot.checked_at_epoch_ms, Some(84));
        assert_eq!(snapshot.latest_version, None);
    }

    #[test]
    fn download_progress_and_failure_preserve_a_retryable_update() {
        let state = supported_updater_state();
        publish_available_update(&state);
        state.begin_install().unwrap();
        state.update_download_progress(25, Some(100));

        assert_eq!(state.snapshot().downloaded_bytes, Some(25));
        assert_eq!(state.snapshot().total_bytes, Some(100));

        state.mark_installing();
        assert_eq!(state.snapshot().phase, UpdatePhase::Installing);
        state.fail_operation("Signature verification failed");

        let snapshot = state.snapshot();
        assert_eq!(snapshot.phase, UpdatePhase::Available);
        assert_eq!(snapshot.latest_version.as_deref(), Some("1.1.0"));
        assert_eq!(
            snapshot.error.as_deref(),
            Some("Signature verification failed")
        );
        state
            .begin_install()
            .expect("failed installs remain retryable");
    }
}
