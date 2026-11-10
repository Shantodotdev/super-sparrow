//! The desktop app's own settings: the address of the team's Super Sparrow server and the
//! quick capture shortcut.

use std::fs;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Manager};

const FILE_NAME: &str = "settings.json";

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Settings {
    server_url: Option<String>,
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

    pub fn set_server(&mut self, server: &url::Url) {
        self.server_url = Some(server.to_string());
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

    /// Returns the saved server address when it is still a valid one.
    pub fn server_url(&self) -> Option<url::Url> {
        parse_server_url(self.server_url.as_deref()?).ok()
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

/// Reads a server address typed by a person: an http or https URL, kept to its origin so the app
/// always opens the server's start page. http is accepted for a server on this machine only.
pub fn parse_server_url(address: &str) -> Result<url::Url, String> {
    let address = address.trim();
    let with_scheme = if address.contains("://") {
        address.to_string()
    } else {
        format!("https://{address}")
    };
    let parsed = url::Url::parse(&with_scheme).map_err(|_| {
        "Enter the server's address, such as https://sparrow.example.com".to_string()
    })?;

    let host = parsed.host_str().unwrap_or_default();
    let local = matches!(host, "localhost" | "127.0.0.1" | "[::1]");
    match parsed.scheme() {
        "https" => {}
        "http" if local => {}
        "http" => {
            return Err(
                "Use an https address; plain http is only for a server on this computer".into(),
            );
        }
        _ => return Err("The address must start with https://".into()),
    }
    if host.is_empty() {
        return Err("Enter the server's address, such as https://sparrow.example.com".into());
    }

    let origin = parsed.origin().ascii_serialization();
    url::Url::parse(&format!("{origin}/")).map_err(|error| error.to_string())
}

#[cfg(test)]
mod tests {
    use super::parse_server_url;

    #[test]
    fn keeps_only_the_origin() {
        assert_eq!(
            parse_server_url(" sparrow.example.com/WP/issues ")
                .unwrap()
                .as_str(),
            "https://sparrow.example.com/"
        );
        assert_eq!(
            parse_server_url("http://localhost:3000/issues")
                .unwrap()
                .as_str(),
            "http://localhost:3000/"
        );
    }

    #[test]
    fn refuses_plain_http_to_other_computers_and_other_schemes() {
        assert!(parse_server_url("http://sparrow.example.com").is_err());
        assert!(parse_server_url("file:///etc/passwd").is_err());
        assert!(parse_server_url("javascript:alert(1)").is_err());
        assert!(parse_server_url("").is_err());
    }
}
