#[cfg(feature = "desktop-runtime")]
use serde::{Deserialize, Serialize};
#[cfg(feature = "desktop-runtime")]
use std::{
    fs,
    path::PathBuf,
    sync::{
        mpsc::{self, Receiver, RecvTimeoutError, Sender},
        Arc, Mutex,
    },
    thread,
    time::Duration,
};
#[cfg(feature = "desktop-runtime")]
use tauri::{PhysicalPosition, PhysicalSize, WebviewWindow};

const MIN_VISIBLE_WIDTH: i32 = 120;
const MIN_VISIBLE_HEIGHT: i32 = 80;
#[cfg(feature = "desktop-runtime")]
const SAVE_DEBOUNCE: Duration = Duration::from_millis(350);

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[cfg_attr(feature = "desktop-runtime", derive(Serialize, Deserialize))]
pub struct WindowBounds {
    pub x: i32,
    pub y: i32,
    pub width: u32,
    pub height: u32,
    pub maximized: bool,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct MonitorBounds {
    pub x: i32,
    pub y: i32,
    pub width: u32,
    pub height: u32,
}

pub fn visible_restore(saved: WindowBounds, monitors: &[MonitorBounds]) -> WindowBounds {
    if monitors.is_empty() || monitors.iter().any(|monitor| intersects(saved, *monitor)) {
        return saved;
    }

    let monitor = monitors[0];
    let width = saved.width.max(MIN_VISIBLE_WIDTH as u32).min(monitor.width);
    let height = saved
        .height
        .max(MIN_VISIBLE_HEIGHT as u32)
        .min(monitor.height);
    WindowBounds {
        x: monitor.x,
        y: monitor.y,
        width,
        height,
        maximized: saved.maximized,
    }
}

fn intersects(window: WindowBounds, monitor: MonitorBounds) -> bool {
    let window_right = window.x.saturating_add(window.width as i32);
    let window_bottom = window.y.saturating_add(window.height as i32);
    let monitor_right = monitor.x.saturating_add(monitor.width as i32);
    let monitor_bottom = monitor.y.saturating_add(monitor.height as i32);

    window_right >= monitor.x.saturating_add(MIN_VISIBLE_WIDTH)
        && window.x <= monitor_right.saturating_sub(MIN_VISIBLE_WIDTH)
        && window_bottom >= monitor.y.saturating_add(MIN_VISIBLE_HEIGHT)
        && window.y <= monitor_bottom.saturating_sub(MIN_VISIBLE_HEIGHT)
}

#[derive(Debug, Default)]
struct SaveSchedule {
    revision: u64,
}

impl SaveSchedule {
    fn next(&mut self) -> u64 {
        self.revision = self.revision.wrapping_add(1);
        self.revision
    }

    fn is_current(&self, revision: u64) -> bool {
        self.revision == revision
    }
}

#[cfg(feature = "desktop-runtime")]
pub struct WindowStateStore {
    path: PathBuf,
    last_normal: Mutex<Option<WindowBounds>>,
    pending: Sender<(u64, WindowBounds)>,
    schedule: Arc<Mutex<SaveSchedule>>,
    write_lock: Arc<Mutex<()>>,
}

#[cfg(feature = "desktop-runtime")]
impl WindowStateStore {
    pub fn new(path: PathBuf) -> Self {
        let (pending, receiver) = mpsc::channel();
        let schedule = Arc::new(Mutex::new(SaveSchedule::default()));
        let write_lock = Arc::new(Mutex::new(()));
        start_save_worker(path.clone(), receiver, schedule.clone(), write_lock.clone());
        Self {
            path,
            last_normal: Mutex::new(None),
            pending,
            schedule,
            write_lock,
        }
    }

    pub fn restore(&self, window: &WebviewWindow) -> Result<(), String> {
        let saved = match fs::read(&self.path) {
            Ok(bytes) => serde_json::from_slice::<WindowBounds>(&bytes)
                .map_err(|err| format!("invalid saved desktop window state: {err}"))?,
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => return Ok(()),
            Err(err) => return Err(format!("could not read desktop window state: {err}")),
        };
        let monitors = window
            .available_monitors()
            .map_err(|err| err.to_string())?
            .into_iter()
            .map(|monitor| {
                let work_area = monitor.work_area();
                MonitorBounds {
                    x: work_area.position.x,
                    y: work_area.position.y,
                    width: work_area.size.width,
                    height: work_area.size.height,
                }
            })
            .collect::<Vec<_>>();
        let restored = visible_restore(saved, &monitors);
        window
            .set_size(PhysicalSize::new(restored.width, restored.height))
            .map_err(|err| err.to_string())?;
        window
            .set_position(PhysicalPosition::new(restored.x, restored.y))
            .map_err(|err| err.to_string())?;
        *self
            .last_normal
            .lock()
            .expect("window state mutex poisoned") = Some(WindowBounds {
            maximized: false,
            ..restored
        });
        if restored.maximized {
            window.maximize().map_err(|err| err.to_string())?;
        }
        Ok(())
    }

    pub fn schedule_save(&self, window: &WebviewWindow) -> Result<(), String> {
        let Some(bounds) = self.capture(window)? else {
            return Ok(());
        };
        let revision = self
            .schedule
            .lock()
            .expect("window save schedule mutex poisoned")
            .next();
        self.pending
            .send((revision, bounds))
            .map_err(|_| "desktop window state writer stopped unexpectedly".to_string())
    }

    pub fn save(&self, window: &WebviewWindow) -> Result<(), String> {
        let Some(bounds) = self.capture(window)? else {
            return Ok(());
        };
        self.schedule
            .lock()
            .expect("window save schedule mutex poisoned")
            .next();
        let _guard = self
            .write_lock
            .lock()
            .expect("window state writer mutex poisoned");
        write_bounds(&self.path, bounds)
    }

    fn capture(&self, window: &WebviewWindow) -> Result<Option<WindowBounds>, String> {
        if window.is_minimized().unwrap_or(false) {
            return Ok(None);
        }

        let maximized = window.is_maximized().unwrap_or(false);
        let bounds = if maximized {
            self.last_normal
                .lock()
                .expect("window state mutex poisoned")
                .map(|bounds| WindowBounds {
                    maximized: true,
                    ..bounds
                })
        } else {
            let position = window.outer_position().map_err(|err| err.to_string())?;
            let size = window.outer_size().map_err(|err| err.to_string())?;
            let bounds = WindowBounds {
                x: position.x,
                y: position.y,
                width: size.width,
                height: size.height,
                maximized: false,
            };
            *self
                .last_normal
                .lock()
                .expect("window state mutex poisoned") = Some(bounds);
            Some(bounds)
        };
        Ok(bounds)
    }
}

#[cfg(feature = "desktop-runtime")]
fn start_save_worker(
    path: PathBuf,
    receiver: Receiver<(u64, WindowBounds)>,
    schedule: Arc<Mutex<SaveSchedule>>,
    write_lock: Arc<Mutex<()>>,
) {
    thread::spawn(move || {
        while let Ok(mut pending) = receiver.recv() {
            loop {
                match receiver.recv_timeout(SAVE_DEBOUNCE) {
                    Ok(next) => pending = next,
                    Err(RecvTimeoutError::Timeout) => break,
                    Err(RecvTimeoutError::Disconnected) => return,
                }
            }
            let _guard = write_lock
                .lock()
                .expect("window state writer mutex poisoned");
            if schedule
                .lock()
                .expect("window save schedule mutex poisoned")
                .is_current(pending.0)
            {
                if let Err(err) = write_bounds(&path, pending.1) {
                    eprintln!("Could not persist desktop window state: {err}");
                }
            }
        }
    });
}

#[cfg(feature = "desktop-runtime")]
fn write_bounds(path: &PathBuf, bounds: WindowBounds) -> Result<(), String> {
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)
            .map_err(|err| format!("could not create desktop state directory: {err}"))?;
    }
    let bytes = serde_json::to_vec(&bounds)
        .map_err(|err| format!("could not serialize desktop window state: {err}"))?;
    fs::write(path, bytes).map_err(|err| format!("could not save desktop window state: {err}"))
}

#[cfg(test)]
mod tests {
    use super::*;

    const PRIMARY: MonitorBounds = MonitorBounds {
        x: 0,
        y: 0,
        width: 1920,
        height: 1080,
    };

    #[test]
    fn visible_saved_bounds_are_preserved() {
        let saved = WindowBounds {
            x: 100,
            y: 80,
            width: 1280,
            height: 900,
            maximized: true,
        };
        assert_eq!(visible_restore(saved, &[PRIMARY]), saved);
    }

    #[test]
    fn disconnected_monitor_bounds_restore_on_the_primary_monitor() {
        let saved = WindowBounds {
            x: 3000,
            y: 200,
            width: 1280,
            height: 900,
            maximized: false,
        };
        assert_eq!(
            visible_restore(saved, &[PRIMARY]),
            WindowBounds {
                x: 0,
                y: 0,
                width: 1280,
                height: 900,
                maximized: false,
            }
        );
    }

    #[test]
    fn oversized_bounds_are_clamped_to_the_restore_monitor() {
        let saved = WindowBounds {
            x: -5000,
            y: -5000,
            width: 4000,
            height: 3000,
            maximized: false,
        };
        let restored = visible_restore(saved, &[PRIMARY]);
        assert_eq!(restored.width, PRIMARY.width);
        assert_eq!(restored.height, PRIMARY.height);
        assert_eq!((restored.x, restored.y), (PRIMARY.x, PRIMARY.y));
    }

    #[test]
    fn newer_save_revisions_supersede_pending_work() {
        let mut schedule = SaveSchedule::default();
        let first = schedule.next();
        let second = schedule.next();

        assert!(!schedule.is_current(first));
        assert!(schedule.is_current(second));

        schedule.next();
        assert!(!schedule.is_current(second));
    }
}
