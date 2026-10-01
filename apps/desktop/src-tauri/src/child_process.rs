use std::{io, process::Command, sync::mpsc, thread};

pub(crate) fn spawn_managed(mut command: Command, operation: &'static str) -> io::Result<u32> {
    let (acknowledge, acknowledgement) = mpsc::sync_channel(1);

    thread::Builder::new()
        .name("kandev-aux-child-waiter".to_string())
        .spawn(move || match command.spawn() {
            Ok(mut child) => {
                let _ = acknowledge.send(Ok(child.id()));
                report_wait_result(operation, child.wait());
            }
            Err(error) => {
                let _ = acknowledge.send(Err(error));
            }
        })?;

    acknowledgement.recv().unwrap_or_else(|_| {
        Err(io::Error::other(
            "child waiter exited before acknowledgement",
        ))
    })
}

fn report_wait_result(operation: &str, result: io::Result<std::process::ExitStatus>) {
    match result {
        Ok(status) if !status.success() => match status.code() {
            Some(code) => eprintln!("kandev: {operation} exited with status {code}"),
            None => eprintln!("kandev: {operation} exited after a signal"),
        },
        Err(_) => eprintln!("kandev: failed to wait for {operation}"),
        _ => {}
    }
}

#[cfg(all(test, unix))]
mod tests {
    use super::spawn_managed;
    use std::{
        fs,
        io::{self, Write},
        process::{Command, Stdio},
        sync::mpsc,
        thread,
        time::{Duration, Instant},
    };

    const PARENT_EXIT_TEST: &str =
        "child_process::tests::managed_launch_child_survives_launching_parent_exit";
    const PARENT_EXIT_MODE: &str = "KANDEV_MANAGED_CHILD_TEST_MODE";
    const PARENT_EXIT_ROOT: &str = "KANDEV_MANAGED_CHILD_TEST_ROOT";

    #[test]
    fn managed_launch_reaps_repeated_short_lived_children() {
        let pids = (0..17)
            .map(|_| {
                spawn_managed(Command::new("/usr/bin/true"), "test child")
                    .expect("launch short-lived child")
            })
            .collect::<Vec<_>>();

        let still_waitable = pids
            .into_iter()
            .filter(|pid| !pid_has_been_reaped(*pid))
            .collect::<Vec<_>>();

        assert!(
            still_waitable.is_empty(),
            "child statuses remained waitable: {still_waitable:?}"
        );
    }

    #[test]
    fn managed_launch_acknowledges_before_child_exit() {
        let pid_file = test_path("long-child.pid");
        let mut command = Command::new("/bin/sh");
        command
            .arg("-c")
            .arg("printf '%s' \"$$\" > \"$KANDEV_CHILD_PID_FILE\"; exec /bin/sleep 30")
            .env("KANDEV_CHILD_PID_FILE", &pid_file)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null());

        let (result_sender, result_receiver) = mpsc::sync_channel(1);
        let launch_thread = thread::spawn(move || {
            let _ = result_sender.send(spawn_managed(command, "test long child"));
        });
        let acknowledgement = result_receiver.recv_timeout(Duration::from_secs(1));
        let pid_from_command = read_pid(&pid_file);

        if acknowledgement.is_err() {
            if let Some(pid) = pid_from_command {
                unsafe { libc::kill(pid as i32, libc::SIGKILL) };
            }
        }
        let _ = launch_thread.join();
        let pid = acknowledgement
            .expect("spawn acknowledgement must not wait for child exit")
            .expect("long-lived child must spawn");

        assert_eq!(pid_from_command, Some(pid));
        assert!(
            process_exists(pid),
            "child {pid} must remain alive after acknowledgement"
        );
        unsafe { libc::kill(pid as i32, libc::SIGTERM) };
        assert!(
            pid_has_been_reaped(pid),
            "managed child {pid} was not reaped"
        );
        let _ = fs::remove_file(pid_file);
    }

    #[test]
    fn managed_launch_reports_spawn_failure() {
        let error = spawn_managed(
            Command::new("/path/that/does/not/exist/kandev-child"),
            "test missing child",
        )
        .expect_err("missing executable must return a spawn error");

        assert_eq!(error.kind(), io::ErrorKind::NotFound);
    }

    #[test]
    fn managed_launch_does_not_reap_unrelated_children() {
        let mut exited_sentinel = Command::new("/usr/bin/true")
            .spawn()
            .expect("spawn exited sentinel");
        wait_until_exited_without_reaping(exited_sentinel.id());

        let mut live_sentinel = Command::new("/bin/sleep")
            .arg("30")
            .spawn()
            .expect("spawn live sentinel");
        let managed_pid = spawn_managed(Command::new("/usr/bin/true"), "test managed child")
            .expect("launch managed child");
        let live_before_cleanup = live_sentinel.try_wait().expect("check live sentinel");
        if live_before_cleanup.is_none() {
            let _ = live_sentinel.kill();
        }
        let live_status = live_sentinel.wait().expect("wait for live sentinel");
        let exited_status = exited_sentinel.wait().expect("wait for exited sentinel");
        let managed_was_reaped = pid_has_been_reaped(managed_pid);

        assert!(
            live_before_cleanup.is_none(),
            "managed launch changed live sentinel"
        );
        assert!(live_status.success() || live_status.code().is_none());
        assert!(
            exited_status.success(),
            "exited sentinel lost its original wait owner"
        );
        assert!(
            managed_was_reaped,
            "managed child {managed_pid} was not reaped"
        );
    }

    #[test]
    fn managed_launch_child_survives_launching_parent_exit() {
        match std::env::var(PARENT_EXIT_MODE).as_deref() {
            Ok("launcher") => {
                run_parent_exit_launcher().expect("launch child fixture");
                return;
            }
            Ok("supervisor") => {
                run_parent_exit_supervisor().expect("supervise launcher exit");
                return;
            }
            Ok(mode) => panic!("unexpected process fixture mode: {mode}"),
            Err(_) => {}
        }

        let root = test_path("parent-exit");
        let _ = fs::remove_dir_all(&root);
        fs::create_dir_all(&root).expect("create parent-exit fixture directory");
        create_child_fixture(&root).expect("create long-lived child fixture");
        let mut supervisor = Command::new(std::env::current_exe().expect("locate test process"))
            .arg("--exact")
            .arg(PARENT_EXIT_TEST)
            .arg("--nocapture")
            .env(PARENT_EXIT_MODE, "supervisor")
            .env(PARENT_EXIT_ROOT, &root)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()
            .expect("spawn isolated process supervisor");
        let status = wait_for_process(&mut supervisor, Duration::from_secs(20));

        cleanup_parent_exit_fixture(&root);
        let _ = fs::remove_dir_all(&root);
        assert!(
            status.is_some_and(|status| status.success()),
            "isolated supervisor must prove the child survives and responds after launcher exit"
        );
    }

    fn create_child_fixture(root: &std::path::Path) -> io::Result<()> {
        use std::os::unix::ffi::OsStrExt;
        use std::os::unix::fs::PermissionsExt;

        let script = root.join("child-fixture.sh");
        fs::write(
            &script,
            "#!/bin/sh\nprintf '%s' \"$$\" > \"$4\"\nprintf ready > \"$1\"\nwhile IFS= read -r action; do\n  case \"$action\" in\n    ping) printf responsive > \"$2\" ;;\n    stop) exit 0 ;;\n  esac\ndone < \"$3\"\n",
        )?;
        let mut permissions = fs::metadata(&script)?.permissions();
        permissions.set_mode(0o700);
        fs::set_permissions(script, permissions)?;

        let fifo = std::ffi::CString::new(root.join("commands").as_os_str().as_bytes())
            .map_err(|error| io::Error::new(io::ErrorKind::InvalidInput, error))?;
        if unsafe { libc::mkfifo(fifo.as_ptr(), 0o600) } != 0 {
            return Err(io::Error::last_os_error());
        }
        Ok(())
    }

    fn run_parent_exit_launcher() -> io::Result<()> {
        let root = parent_exit_root();
        let mut command = Command::new("/bin/sh");
        command
            .arg(root.join("child-fixture.sh"))
            .arg(root.join("ready"))
            .arg(root.join("response"))
            .arg(root.join("commands"))
            .arg(root.join("child.pid"))
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null());
        let pid = spawn_managed(command, "test independent child")?;
        fs::write(root.join("launcher-child.pid"), pid.to_string())
    }

    fn run_parent_exit_supervisor() -> io::Result<()> {
        #[cfg(target_os = "linux")]
        if unsafe { libc::prctl(libc::PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0) } != 0 {
            return Err(io::Error::last_os_error());
        }

        let root = parent_exit_root();
        let mut launcher = Command::new(std::env::current_exe()?)
            .arg("--exact")
            .arg(PARENT_EXIT_TEST)
            .arg("--nocapture")
            .env(PARENT_EXIT_MODE, "launcher")
            .env(PARENT_EXIT_ROOT, &root)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .spawn()?;
        fs::write(root.join("launcher.pid"), launcher.id().to_string())?;

        let result = (|| {
            let status = wait_for_process(&mut launcher, Duration::from_secs(5))
                .ok_or_else(|| io::Error::new(io::ErrorKind::TimedOut, "launcher did not exit"))?;
            if !status.success() {
                return Err(io::Error::other("managed-launch subprocess failed"));
            }
            if !wait_for_path(&root.join("ready"), Duration::from_secs(5))
                || !wait_for_path(&root.join("child.pid"), Duration::from_secs(1))
            {
                return Err(io::Error::new(
                    io::ErrorKind::TimedOut,
                    "child fixture did not become ready after launcher exit",
                ));
            }
            let pid = read_pid(&root.join("child.pid"))
                .ok_or_else(|| io::Error::other("child fixture PID was not recorded"))?;
            if !process_exists(pid) {
                return Err(io::Error::other("child exited with its launching parent"));
            }

            let mut commands = open_fifo_writer(&root.join("commands"), Duration::from_secs(5))?;
            commands.write_all(b"ping\n")?;
            if !wait_for_path(&root.join("response"), Duration::from_secs(5))
                || fs::read_to_string(root.join("response"))? != "responsive"
            {
                return Err(io::Error::new(
                    io::ErrorKind::TimedOut,
                    "child did not respond after launcher exit",
                ));
            }
            commands.write_all(b"stop\n")?;
            if !wait_for_fixture_child_exit(pid, Duration::from_secs(5)) {
                return Err(io::Error::new(
                    io::ErrorKind::TimedOut,
                    "child fixture did not honor its stop request",
                ));
            }
            Ok(())
        })();

        if launcher.try_wait()?.is_none() {
            let _ = launcher.kill();
            let _ = launcher.wait();
        }
        cleanup_parent_exit_fixture(&root);
        result
    }

    fn open_fifo_writer(path: &std::path::Path, timeout: Duration) -> io::Result<std::fs::File> {
        use std::os::unix::fs::OpenOptionsExt;

        let deadline = Instant::now() + timeout;
        loop {
            match fs::OpenOptions::new()
                .write(true)
                .custom_flags(libc::O_NONBLOCK)
                .open(path)
            {
                Ok(file) => return Ok(file),
                Err(error) if error.raw_os_error() == Some(libc::ENXIO) => {
                    if Instant::now() >= deadline {
                        return Err(io::Error::new(
                            io::ErrorKind::TimedOut,
                            "child fixture did not open its command pipe",
                        ));
                    }
                    thread::sleep(Duration::from_millis(10));
                }
                Err(error) => return Err(error),
            }
        }
    }

    fn cleanup_parent_exit_fixture(root: &std::path::Path) {
        if let Some(pid) = read_pid(&root.join("child.pid")) {
            if process_exists(pid) {
                if let Ok(mut commands) =
                    open_fifo_writer(&root.join("commands"), Duration::from_secs(1))
                {
                    let _ = commands.write_all(b"stop\n");
                }
                if !wait_for_fixture_child_exit(pid, Duration::from_secs(2)) {
                    unsafe { libc::kill(pid as i32, libc::SIGKILL) };
                    let _ = wait_for_fixture_child_exit(pid, Duration::from_secs(2));
                }
            }
        }
        if let Some(pid) = read_pid(&root.join("launcher.pid")) {
            if process_exists(pid) {
                unsafe { libc::kill(pid as i32, libc::SIGKILL) };
            }
        }
    }

    fn wait_for_fixture_child_exit(pid: u32, timeout: Duration) -> bool {
        let deadline = Instant::now() + timeout;
        loop {
            #[cfg(target_os = "linux")]
            {
                let mut status = 0;
                let result =
                    unsafe { libc::waitpid(pid as libc::pid_t, &mut status, libc::WNOHANG) };
                if result == pid as libc::pid_t {
                    return libc::WIFEXITED(status) && libc::WEXITSTATUS(status) == 0;
                }
                if result == -1 {
                    return false;
                }
            }
            #[cfg(not(target_os = "linux"))]
            if !process_exists(pid) {
                return true;
            }
            if Instant::now() >= deadline {
                return false;
            }
            thread::sleep(Duration::from_millis(10));
        }
    }

    fn wait_for_process(
        child: &mut std::process::Child,
        timeout: Duration,
    ) -> Option<std::process::ExitStatus> {
        let deadline = Instant::now() + timeout;
        loop {
            if let Ok(Some(status)) = child.try_wait() {
                return Some(status);
            }
            if Instant::now() >= deadline {
                let _ = child.kill();
                let _ = child.wait();
                return None;
            }
            thread::sleep(Duration::from_millis(10));
        }
    }

    fn wait_for_path(path: &std::path::Path, timeout: Duration) -> bool {
        let deadline = Instant::now() + timeout;
        while Instant::now() < deadline {
            if path.exists() {
                return true;
            }
            thread::sleep(Duration::from_millis(10));
        }
        path.exists()
    }

    fn parent_exit_root() -> std::path::PathBuf {
        std::env::var_os(PARENT_EXIT_ROOT)
            .expect("parent-exit root environment")
            .into()
    }

    fn pid_has_been_reaped(pid: u32) -> bool {
        let deadline = Instant::now() + Duration::from_secs(1);
        loop {
            let mut info = unsafe { std::mem::zeroed::<libc::siginfo_t>() };
            let result = unsafe {
                libc::waitid(
                    libc::P_PID,
                    pid as libc::id_t,
                    &mut info,
                    libc::WEXITED | libc::WNOHANG | libc::WNOWAIT,
                )
            };
            if result == -1 {
                let error = io::Error::last_os_error();
                assert_eq!(
                    error.raw_os_error(),
                    Some(libc::ECHILD),
                    "waitpid failed for exact child PID {pid}"
                );
                return true;
            }
            if Instant::now() >= deadline {
                let mut status = 0;
                let collected =
                    unsafe { libc::waitpid(pid as libc::pid_t, &mut status, libc::WNOHANG) };
                if collected == pid as libc::pid_t {
                    return false;
                }
                if collected == -1
                    && io::Error::last_os_error().raw_os_error() == Some(libc::ECHILD)
                {
                    return true;
                }
                unsafe { libc::kill(pid as i32, libc::SIGKILL) };
                let _ = unsafe { libc::waitpid(pid as libc::pid_t, &mut status, 0) };
                return false;
            }
            thread::yield_now();
        }
    }

    fn wait_until_exited_without_reaping(pid: u32) {
        let mut info = unsafe { std::mem::zeroed::<libc::siginfo_t>() };
        let result = unsafe {
            libc::waitid(
                libc::P_PID,
                pid as libc::id_t,
                &mut info,
                libc::WEXITED | libc::WNOWAIT,
            )
        };
        assert_eq!(result, 0, "wait for exact sentinel PID {pid} to exit");
    }

    fn process_exists(pid: u32) -> bool {
        unsafe { libc::kill(pid as i32, 0) == 0 }
    }

    fn test_path(name: &str) -> std::path::PathBuf {
        std::env::temp_dir().join(format!(
            "kandev-child-process-{}-{name}",
            std::process::id()
        ))
    }

    fn read_pid(path: &std::path::Path) -> Option<u32> {
        let deadline = Instant::now() + Duration::from_secs(1);
        loop {
            if let Ok(value) = fs::read_to_string(path) {
                if let Ok(pid) = value.parse() {
                    return Some(pid);
                }
            }
            if Instant::now() >= deadline {
                return None;
            }
            thread::sleep(Duration::from_millis(5));
        }
    }
}
