//! Validated global capture accelerators. Personal preferences arrive from the authenticated page.

use std::str::FromStr;
use tauri_plugin_global_shortcut::Shortcut;

/// The capture accelerator used before a person customizes their account.
pub const DEFAULT_SHORTCUT: &str = "Meta+Shift+Q";

/// Accepts one modified accelerator, excluding focus and cancellation keys. Meta is translated
/// to the native shortcut library's Super name, which also represents Command on macOS.
pub fn parse(label: &str) -> Option<Shortcut> {
    if label.len() > 80 || label.contains(' ') {
        return None;
    }
    let parts: Vec<&str> = label.split('+').collect();
    let key = *parts.last()?;
    if matches!(key, "Escape" | "Tab") || parts.len() < 2 {
        return None;
    }
    let mut previous = None;
    let mut primary = false;
    for modifier in &parts[..parts.len() - 1] {
        let rank = match *modifier {
            "Ctrl" => {
                primary = true;
                0
            }
            "Alt" => {
                primary = true;
                1
            }
            "Meta" => {
                primary = true;
                2
            }
            "Shift" => 3,
            _ => return None,
        };
        if previous.is_some_and(|previous| previous >= rank) {
            return None;
        }
        previous = Some(rank);
    }
    if !primary {
        return None;
    }
    Shortcut::from_str(&label.replace("Meta+", "Super+")).ok()
}

#[cfg(test)]
mod tests {
    use super::*;
    use tauri_plugin_global_shortcut::{Code, Modifiers};

    #[test]
    fn default_uses_meta_shift_q() {
        assert_eq!(
            parse(DEFAULT_SHORTCUT),
            Some(Shortcut::new(
                Some(Modifiers::SUPER | Modifiers::SHIFT),
                Code::KeyQ
            ))
        );
    }

    #[test]
    fn accepts_personal_modified_chords() {
        for label in [
            "Ctrl+Alt+K",
            "Meta+Shift+Space",
            "Alt+F8",
            "Ctrl+Shift+Slash",
        ] {
            assert!(parse(label).is_some(), "{label}");
        }
    }

    #[test]
    fn refuses_sequences_reserved_keys_and_unmodified_accelerators() {
        for label in [
            "",
            "Q",
            "Shift+Q",
            "Ctrl+Tab",
            "Meta+Escape",
            "G I",
            "Mod+K",
            "Meta+Ctrl+K",
            "Ctrl+Ctrl+K",
        ] {
            assert!(parse(label).is_none(), "{label}");
        }
    }
}
