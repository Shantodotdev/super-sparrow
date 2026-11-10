//! The Super Sparrow desktop app. Its main window shows the team's Super Sparrow server, and it
//! stays in the tray when that window is closed, so its shortcut, Ctrl+Q unless the person picks
//! another, opens the quick capture window over any application.

mod capture;
mod server_url;
mod settings;
mod shortcut;

use std::sync::Mutex;

use tauri::ipc::CapabilityBuilder;
use tauri::menu::{CheckMenuItem, IsMenuItem, Menu, MenuItem, PredefinedMenuItem, Submenu};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{
    AppHandle, Manager, State, WebviewUrl, WebviewWindow, WebviewWindowBuilder, WindowEvent, Wry,
};
use tauri_plugin_autostart::{MacosLauncher, ManagerExt};
use tauri_plugin_global_shortcut::{GlobalShortcutExt, ShortcutState};
use tauri_plugin_notification::NotificationExt;

use settings::Settings;

const MAIN_WINDOW: &str = "main";
const CAPTURE_WINDOW: &str = "capture";
/// The argument the login item starts the app with, so it starts in the tray without a window.
const HIDDEN_ARGUMENT: &str = "--hidden";
const PAUSE_ID: &str = "shortcut-pause";
const MAX_NOTIFICATION_TITLE: usize = 120;
const MAX_NOTIFICATION_BODY: usize = 400;

/// The server embedded at build time. Native features are granted to its address only.
struct Server(url::Url);

/// The capture shortcut's label and whether it is paused.
struct ActiveShortcut(Mutex<(String, bool)>);

/// Starts the desktop app.
pub fn run() {
    let server = server_url::parse(env!("SPARROW_SERVER_URL"))
        .expect("the build must contain a valid PUBLIC_URL");

    tauri::Builder::default()
        // A second launch, such as clicking the app again, shows the running app instead.
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            show_main(app)
        }))
        .plugin(tauri_plugin_autostart::init(
            MacosLauncher::LaunchAgent,
            Some(vec![HIDDEN_ARGUMENT]),
        ))
        .plugin(tauri_plugin_notification::init())
        .plugin(
            tauri_plugin_global_shortcut::Builder::new()
                .with_handler(|app, _shortcut, event| {
                    if event.state() == ShortcutState::Pressed {
                        toggle_capture(app);
                    }
                })
                .build(),
        )
        .manage(Server(server))
        // Nothing is registered yet; setup registers the saved shortcut.
        .manage(ActiveShortcut(Mutex::new((
            shortcut::DEFAULT_SHORTCUT.to_string(),
            true,
        ))))
        .manage(capture::CaptureState::default())
        .invoke_handler(tauri::generate_handler![
            hide_capture,
            current_shortcut,
            notify,
            capture::capture_screen,
            capture::end_capture,
        ])
        .setup(|app| {
            let handle = app.handle().clone();
            let started_hidden = std::env::args().any(|argument| argument == HIDDEN_ARGUMENT);

            let (settings, first_launch) = match Settings::load(&handle) {
                Ok(settings) => (settings, false),
                Err(_) => (Settings::default(), true),
            };
            connect(&handle, handle.state::<Server>().0.clone(), !started_hidden)?;

            if first_launch {
                if let Err(error) = handle.autolaunch().enable() {
                    eprintln!("could not start at login: {error}");
                }
                settings.save(&handle).map_err(|error| {
                    std::io::Error::other(format!("save desktop settings: {error}"))
                })?;
            }

            // Another app may already own the shortcut; the app still works from the tray.
            if let Err(error) =
                apply_shortcut(&handle, settings.shortcut(), settings.shortcut_paused())
            {
                eprintln!("{error}");
            }
            build_tray(&handle)?;
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("failed to start the Super Sparrow desktop app");
}

/// Hides the capture window after a save or when the person closes it.
#[tauri::command]
fn hide_capture(app: AppHandle) {
    if let Some(window) = app.get_webview_window(CAPTURE_WINDOW) {
        let _ = window.hide();
    }
}

/// Returns the capture shortcut's label, such as "Ctrl+Q", or "" while it is paused, so the page
/// can show the key that opens and closes quick capture.
#[tauri::command]
fn current_shortcut(active: State<'_, ActiveShortcut>) -> String {
    match active.0.lock() {
        Ok(current) if !current.1 => current.0.clone(),
        _ => String::new(),
    }
}

/// Shows a native notification, such as for a new finding. Long text is cut short, since the
/// server's page supplies it.
#[tauri::command]
fn notify(app: AppHandle, title: String, body: String) -> Result<(), String> {
    app.notification()
        .builder()
        .title(truncate(&title, MAX_NOTIFICATION_TITLE))
        .body(truncate(&body, MAX_NOTIFICATION_BODY))
        .show()
        .map_err(|error| error.to_string())
}

/// Registers the capture shortcut named by label, or none while paused, in place of the current
/// one. When the new shortcut cannot be registered, such as when another application holds it,
/// the previous one is registered again and the error is returned.
fn apply_shortcut(app: &AppHandle, label: &str, paused: bool) -> Result<(), String> {
    let next = shortcut::parse(label).ok_or_else(|| format!("unknown shortcut {label}"))?;
    let previous = active_shortcut(app);

    let shortcuts = app.global_shortcut();
    shortcuts
        .unregister_all()
        .map_err(|error| format!("release the capture shortcut: {error}"))?;
    if !paused && let Err(error) = shortcuts.register(next) {
        if !previous.1
            && let Some(old) = shortcut::parse(&previous.0)
        {
            let _ = shortcuts.register(old);
        }
        return Err(format!("could not register {label}: {error}"));
    }

    if let Ok(mut current) = app.state::<ActiveShortcut>().0.lock() {
        *current = (label.to_string(), paused);
    }
    Ok(())
}

/// Returns the capture shortcut's label and whether it is paused.
fn active_shortcut(app: &AppHandle) -> (String, bool) {
    app.state::<ActiveShortcut>()
        .0
        .lock()
        .map(|current| current.clone())
        .unwrap_or_else(|_| (shortcut::DEFAULT_SHORTCUT.to_string(), true))
}

/// Shortens text to at most max characters, ending it with an ellipsis when cut.
fn truncate(text: &str, max: usize) -> String {
    if text.chars().count() <= max {
        return text.to_string();
    }
    let mut cut: String = text.chars().take(max.saturating_sub(1)).collect();
    cut.push('…');
    cut
}

/// Grants the configured server's page native features and opens the main window.
fn connect(app: &AppHandle, server: url::Url, show: bool) -> tauri::Result<()> {
    let origin = server.origin().ascii_serialization();
    app.add_capability(
        CapabilityBuilder::new(format!("server-{}", sanitize(&origin)))
            .remote(format!("{origin}/*"))
            .window(MAIN_WINDOW)
            .window(CAPTURE_WINDOW)
            .permission("core:default")
            .permission("core:window:allow-start-dragging"),
    )?;
    let main = WebviewWindowBuilder::new(app, MAIN_WINDOW, WebviewUrl::External(server.clone()))
        .title("Super Sparrow")
        .inner_size(1280.0, 800.0)
        .min_inner_size(900.0, 600.0)
        .visible(show)
        .build()?;
    hide_on_close(&main);
    Ok(())
}

/// Creates the capture window on the server, shown. It is made on the first shortcut press
/// rather than at startup, so an app idling in the tray runs one page instead of two; after
/// that it is only hidden and shown.
fn open_capture(app: &AppHandle, server: &url::Url) -> tauri::Result<WebviewWindow> {
    let capture_url = server
        .join("capture")
        .map_err(|error| tauri::Error::Anyhow(error.into()))?;
    let window = WebviewWindowBuilder::new(app, CAPTURE_WINDOW, WebviewUrl::External(capture_url))
        .title("Quick capture")
        .inner_size(560.0, 640.0)
        .min_inner_size(380.0, 360.0)
        .decorations(false)
        .always_on_top(true)
        .skip_taskbar(true)
        .center()
        .focused(true)
        .build()?;
    hide_on_close(&window);
    Ok(window)
}

/// Shows the capture window over everything, or hides it when it is already showing.
fn toggle_capture(app: &AppHandle) {
    let Some(window) = app.get_webview_window(CAPTURE_WINDOW) else {
        let server = app.state::<Server>();
        if let Err(error) = open_capture(app, &server.0) {
            eprintln!("could not open the capture window: {error}");
        }
        return;
    };
    if window.is_visible().unwrap_or(false) {
        let _ = window.hide();
        return;
    }
    let _ = window.set_always_on_top(true);
    let _ = window.show();
    let _ = window.unminimize();
    let _ = window.set_focus();
}

fn show_main(app: &AppHandle) {
    if let Some(window) = app.get_webview_window(MAIN_WINDOW) {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

/// Closing a window only hides it, so the app keeps running in the tray.
fn hide_on_close(window: &WebviewWindow) {
    let handle = window.clone();
    window.on_window_event(move |event| {
        if let WindowEvent::CloseRequested { api, .. } = event {
            api.prevent_close();
            let _ = handle.hide();
        }
    });
}

/// The tray menu items that show the capture shortcut, kept to update them after a change.
struct ShortcutItems {
    capture: MenuItem<Wry>,
    choices: Vec<CheckMenuItem<Wry>>,
    pause: CheckMenuItem<Wry>,
}

impl ShortcutItems {
    /// Shows the shortcut that applies. Clicking a check item toggles it, so every item is set
    /// again, including after a change that failed.
    fn refresh(&self, app: &AppHandle) {
        let (label, paused) = active_shortcut(app);
        for (choice, item) in shortcut::CHOICES.iter().zip(&self.choices) {
            let _ = item.set_checked(*choice == label);
        }
        let _ = self.pause.set_checked(paused);
        let _ = self
            .capture
            .set_accelerator((!paused).then_some(label.as_str()));
        if let Some(tray) = app.tray_by_id("main") {
            let _ = tray.set_tooltip(Some(tooltip(&label, paused)));
        }
    }
}

/// Applies a shortcut chosen in the tray menu and saves it when it could be registered.
fn choose_shortcut(app: &AppHandle, label: &str, paused: bool) {
    if let Err(error) = apply_shortcut(app, label, paused) {
        eprintln!("{error}");
        return;
    }
    let mut settings = Settings::load_or_empty(app);
    settings.set_shortcut(label, paused);
    if let Err(error) = settings.save(app) {
        eprintln!("could not save the capture shortcut: {error}");
    }
}

fn build_tray(app: &AppHandle) -> tauri::Result<()> {
    let (label, paused) = active_shortcut(app);
    let open = MenuItem::with_id(app, "open", "Open Super Sparrow", true, None::<&str>)?;
    let capture = MenuItem::with_id(
        app,
        "capture",
        "Quick capture",
        true,
        (!paused).then_some(label.as_str()),
    )?;
    let choices = shortcut::CHOICES
        .iter()
        .map(|choice| {
            CheckMenuItem::with_id(
                app,
                shortcut::menu_id(choice),
                *choice,
                true,
                *choice == label,
                None::<&str>,
            )
        })
        .collect::<tauri::Result<Vec<_>>>()?;
    let pause =
        CheckMenuItem::with_id(app, PAUSE_ID, "Pause shortcut", true, paused, None::<&str>)?;
    let shortcut_separator = PredefinedMenuItem::separator(app)?;
    let mut shortcut_entries: Vec<&dyn IsMenuItem<Wry>> = choices
        .iter()
        .map(|item| item as &dyn IsMenuItem<Wry>)
        .collect();
    shortcut_entries.push(&shortcut_separator);
    shortcut_entries.push(&pause);
    let shortcut_menu = Submenu::with_items(app, "Capture shortcut", true, &shortcut_entries)?;

    let start_at_login = CheckMenuItem::with_id(
        app,
        "autostart",
        "Start at login",
        true,
        app.autolaunch().is_enabled().unwrap_or(false),
        None::<&str>,
    )?;
    let quit = MenuItem::with_id(app, "quit", "Quit Super Sparrow", true, None::<&str>)?;
    let menu = Menu::with_items(
        app,
        &[
            &open,
            &capture,
            &PredefinedMenuItem::separator(app)?,
            &shortcut_menu,
            &start_at_login,
            &PredefinedMenuItem::separator(app)?,
            &quit,
        ],
    )?;
    let items = ShortcutItems {
        capture,
        choices,
        pause,
    };

    let mut tray = TrayIconBuilder::with_id("main")
        .tooltip(tooltip(&label, paused))
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(move |app, event| match event.id.as_ref() {
            "open" => show_main(app),
            "capture" => toggle_capture(app),
            "autostart" => {
                let launcher = app.autolaunch();
                let enabled = launcher.is_enabled().unwrap_or(false);
                let _ = if enabled {
                    launcher.disable()
                } else {
                    launcher.enable()
                };
                let _ = start_at_login.set_checked(launcher.is_enabled().unwrap_or(false));
            }
            "quit" => app.exit(0),
            PAUSE_ID => {
                let (label, paused) = active_shortcut(app);
                choose_shortcut(app, &label, !paused);
                items.refresh(app);
            }
            id => {
                if let Some(choice) = id.strip_prefix("shortcut:") {
                    let (_, paused) = active_shortcut(app);
                    choose_shortcut(app, choice, paused);
                    items.refresh(app);
                }
            }
        })
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                show_main(tray.app_handle());
            }
        });
    if let Some(icon) = app.default_window_icon() {
        tray = tray.icon(icon.clone());
    }
    tray.build(app)?;
    Ok(())
}

/// The tray icon's tooltip, naming the capture shortcut.
fn tooltip(label: &str, paused: bool) -> String {
    if paused {
        "Super Sparrow · capture shortcut paused".to_string()
    } else {
        format!("Super Sparrow · {label} to capture")
    }
}

/// Turns an origin into characters a capability identifier allows.
fn sanitize(origin: &str) -> String {
    origin
        .chars()
        .map(|c| if c.is_ascii_alphanumeric() { c } else { '-' })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::{tooltip, truncate};

    #[test]
    fn truncate_keeps_short_text_and_cuts_long_text_by_characters() {
        assert_eq!(truncate("Finding R1", 120), "Finding R1");
        assert_eq!(truncate("abc", 3), "abc");
        assert_eq!(truncate("ééééé", 3), "éé…");
    }

    #[test]
    fn tooltip_names_the_shortcut_or_says_it_is_paused() {
        assert_eq!(
            tooltip("Ctrl+Shift+Q", false),
            "Super Sparrow · Ctrl+Shift+Q to capture"
        );
        assert!(tooltip("Ctrl+Q", true).contains("paused"));
    }
}
