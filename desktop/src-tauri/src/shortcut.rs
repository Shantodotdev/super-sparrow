//! The system-wide shortcut that opens quick capture. People pick one of a few combinations, so
//! they can move it off a key another application needs, or pause it.

use tauri_plugin_global_shortcut::{Code, Modifiers, Shortcut};

/// The shortcut used until the person picks another.
pub const DEFAULT_SHORTCUT: &str = "Ctrl+Q";

/// The shortcuts offered in the tray menu, by label.
pub const CHOICES: [&str; 4] = ["Ctrl+Q", "Ctrl+Shift+Q", "Ctrl+Alt+Q", "Ctrl+Shift+Space"];

/// Turns one of the offered labels, such as "Ctrl+Shift+Q", into a shortcut. Other labels are
/// refused, so a hand-edited settings file cannot register an arbitrary key.
pub fn parse(label: &str) -> Option<Shortcut> {
    let (modifiers, code) = match label {
        "Ctrl+Q" => (Modifiers::CONTROL, Code::KeyQ),
        "Ctrl+Shift+Q" => (Modifiers::CONTROL | Modifiers::SHIFT, Code::KeyQ),
        "Ctrl+Alt+Q" => (Modifiers::CONTROL | Modifiers::ALT, Code::KeyQ),
        "Ctrl+Shift+Space" => (Modifiers::CONTROL | Modifiers::SHIFT, Code::Space),
        _ => return None,
    };
    Some(Shortcut::new(Some(modifiers), code))
}

/// The menu item ID of a shortcut choice.
pub fn menu_id(label: &str) -> String {
    format!("shortcut:{label}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_every_offered_choice_to_a_distinct_shortcut() {
        let shortcuts: Vec<Shortcut> = CHOICES.iter().map(|label| parse(label).unwrap()).collect();
        for (index, shortcut) in shortcuts.iter().enumerate() {
            assert!(!shortcuts[index + 1..].contains(shortcut));
        }
        assert_eq!(
            parse("Ctrl+Q"),
            Some(Shortcut::new(Some(Modifiers::CONTROL), Code::KeyQ))
        );
    }

    #[test]
    fn refuses_labels_that_are_not_offered() {
        assert_eq!(parse("Alt+F4"), None);
        assert_eq!(parse("ctrl+q"), None);
        assert_eq!(parse(""), None);
    }

    #[test]
    fn menu_ids_round_trip_through_the_label() {
        for label in CHOICES {
            assert_eq!(menu_id(label).strip_prefix("shortcut:"), Some(label));
        }
    }
}
