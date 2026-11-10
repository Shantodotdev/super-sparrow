#[path = "src/server_url.rs"]
mod server_url;

fn main() {
    println!("cargo:rerun-if-env-changed=PUBLIC_URL");
    let address = std::env::var("PUBLIC_URL")
        .expect("PUBLIC_URL is required: use make dev-desktop or set PUBLIC_URL before building");
    let server = server_url::parse(&address).unwrap_or_else(|error| panic!("{error}"));
    println!("cargo:rustc-env=SPARROW_SERVER_URL={server}");
    // Tauri refuses custom commands from a remote page unless the app declares them here and a
    // capability grants them; the server's page is remote.
    tauri_build::try_build(tauri_build::Attributes::new().app_manifest(
        tauri_build::AppManifest::new().commands(&[
            "hide_capture",
            "current_shortcut",
            "configure_shortcut",
            "notify",
            "capture_screen",
            "end_capture",
        ]),
    ))
    .expect("build the desktop app");
}
