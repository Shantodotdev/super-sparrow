//! Keeps the installed app current from the signed releases published on GitHub.

use std::time::Duration;

use tauri::{AppHandle, Manager};
use tauri_plugin_updater::UpdaterExt;

const FIRST_CHECK_DELAY: Duration = Duration::from_secs(30);
const CHECK_INTERVAL: Duration = Duration::from_secs(6 * 60 * 60);
const RETRY_WHILE_OPEN: Duration = Duration::from_secs(10 * 60);

/// Checks for a release shortly after launch and then every few hours. An update is installed
/// only while the main window is hidden, so a restart never interrupts someone who is working.
pub fn start(app: &AppHandle) {
    let app = app.clone();
    std::thread::spawn(move || {
        std::thread::sleep(FIRST_CHECK_DELAY);
        loop {
            let wait = match tauri::async_runtime::block_on(check(&app)) {
                Ok(Outcome::Waiting) => RETRY_WHILE_OPEN,
                Ok(Outcome::Current) => CHECK_INTERVAL,
                Err(error) => {
                    eprintln!("update check failed: {error}");
                    CHECK_INTERVAL
                }
            };
            std::thread::sleep(wait);
        }
    });
}

enum Outcome {
    Current,
    Waiting,
}

async fn check(app: &AppHandle) -> Result<Outcome, tauri_plugin_updater::Error> {
    let Some(update) = app.updater()?.check().await? else {
        return Ok(Outcome::Current);
    };
    if main_window_open(app) {
        return Ok(Outcome::Waiting);
    }

    update.download_and_install(|_, _| {}, || {}).await?;
    app.restart()
}

fn main_window_open(app: &AppHandle) -> bool {
    app.get_webview_window("main")
        .and_then(|window| window.is_visible().ok())
        .unwrap_or(false)
}
