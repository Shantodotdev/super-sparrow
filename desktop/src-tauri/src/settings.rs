//! The desktop app's saved quick capture shortcut.

use std::fs;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Manager};

const FILE_NAME: &str = "settings.json";

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Settings {
    #[serde(default)]
    shortcut: Option<String>,
    #[serde(default)]
    shortcut_paused: bool,
}

impl Settings {
    /// Loads the saved settings, or empty ones when none are saved or the file cannot be read.
    pub fn load_or_empty(app: &AppHandle) -> Settings {
        Settings::load(app).unwrap_or_default()
    }

    /// Returns the chosen capture shortcut's label, or the default when none valid is saved.
    pub fn shortcut(&self) -> &str {
        self.shortcut
            .as_deref()
            .filter(|label| crate::shortcut::parse(label).is_some())
            .unwrap_or(crate::shortcut::DEFAULT_SHORTCUT)
    }

    pub fn shortcut_paused(&self) -> bool {
        self.shortcut_paused
    }

    pub fn set_shortcut(&mut self, label: &str, paused: bool) {
        self.shortcut = Some(label.to_string());
        self.shortcut_paused = paused;
    }

    pub fn load(app: &AppHandle) -> Result<Settings, Box<dyn std::error::Error>> {
        let text = fs::read_to_string(path(app)?)?;
        Ok(serde_json::from_str(&text)?)
    }

    pub fn save(&self, app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
        let file = path(app)?;
        if let Some(folder) = file.parent() {
            fs::create_dir_all(folder)?;
        }
        fs::write(file, serde_json::to_string_pretty(self)?)?;
        Ok(())
    }
}

fn path(app: &AppHandle) -> tauri::Result<PathBuf> {
    Ok(app.path().app_config_dir()?.join(FILE_NAME))
}
