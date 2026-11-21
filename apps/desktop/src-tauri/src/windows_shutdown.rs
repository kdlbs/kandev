// Sets the per-process stop event registered by the Kandev launcher so a
// console-less child can run its normal teardown. The caller still waits for
// the child to exit and keeps its existing fallback when this returns false.
use std::ffi::c_void;

const EVENT_MODIFY_STATE: u32 = 0x0002;

#[link(name = "kernel32")]
extern "system" {
    fn OpenEventW(access: u32, inherit: i32, name: *const u16) -> *mut c_void;
    fn SetEvent(event: *mut c_void) -> i32;
    fn CloseHandle(handle: *mut c_void) -> i32;
}

pub(crate) fn request(pid: u32) -> bool {
    if pid == 0 {
        return false;
    }
    let name: Vec<u16> = format!(r"Local\KandevShutdown-{pid}")
        .encode_utf16()
        .chain(std::iter::once(0))
        .collect();
    // SAFETY: `name` is a NUL-terminated UTF-16 buffer that outlives the call,
    // and the handle is only used and closed here after a non-null check.
    unsafe {
        let event = OpenEventW(EVENT_MODIFY_STATE, 0, name.as_ptr());
        if event.is_null() {
            return false;
        }
        let sent = SetEvent(event) != 0;
        CloseHandle(event);
        sent
    }
}
