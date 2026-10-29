/// Starts the desktop app. The window shows the local setup page until the server address is set.
pub fn run() {
    tauri::Builder::default()
        .run(tauri::generate_context!())
        .expect("failed to start the Super Sparrow desktop app");
}
