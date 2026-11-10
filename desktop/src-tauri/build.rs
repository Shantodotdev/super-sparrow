/// The app's own commands are declared so capabilities can grant each one: the setup page gets
/// the server settings, and the server's page gets capture, the shortcut label, and
/// notifications. Undeclared, Tauri refuses them to the server's page.
const COMMANDS: &[&str] = &[
    "save_server",
    "current_server",
    "hide_capture",
    "current_shortcut",
    "notify",
    "capture_screen",
    "end_capture",
];

fn main() {
    tauri_build::try_build(
        tauri_build::Attributes::new()
            .app_manifest(tauri_build::AppManifest::new().commands(COMMANDS)),
    )
    .expect("failed to build the Super Sparrow desktop app");
}
