fn main() {
    if std::env::var_os("CARGO_FEATURE_DESKTOP_RUNTIME").is_some() {
        let manifest = tauri_build::AppManifest::new().commands(&[
            "get_update_state",
            "check_for_updates",
            "install_update",
            "show_native_notification",
            "open_external_url",
        ]);
        tauri_build::try_build(tauri_build::Attributes::new().app_manifest(manifest))
            .expect("failed to build Tauri desktop application");
    }
}
