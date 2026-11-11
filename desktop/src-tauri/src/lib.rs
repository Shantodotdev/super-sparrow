//! The Super Sparrow desktop app. Its main window shows the team's Super Sparrow server, and it
//! stays in the tray when that window is closed, so its shortcut, Meta+Shift+Q unless the person picks
//! another, opens the quick capture window over any application.

mod capture;
mod cli_login;
mod server_url;
mod settings;
mod shortcut;
mod updater;

use std::sync::Mutex;

use tauri::ipc::CapabilityBuilder;
use tauri::menu::{CheckMenuItem, Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{
    AppHandle, Manager, State, WebviewUrl, WebviewWindow, WebviewWindowBuilder, WindowEvent, Wry,
};
use tauri_plugin_autostart::{MacosLauncher, ManagerExt};
use tauri_plugin_deep_link::DeepLinkExt;
use tauri_plugin_global_shortcut::{GlobalShortcutExt, ShortcutState};
use tauri_plugin_notification::NotificationExt;

use settings::Settings;

const MAIN_WINDOW: &str = "main";
const CAPTURE_WINDOW: &str = "capture";
/// The argument the login item starts the app with, so it starts in the tray without a window.
const HIDDEN_ARGUMENT: &str = "--hidden";
/// The User-Agent of the app's windows. The server serves its pages only to requests that carry
/// the `SuperSparrowDesktop/` token, so browsers are turned away.
const USER_AGENT: &str = concat!("SuperSparrowDesktop/", env!("CARGO_PKG_VERSION"));
const MAX_NOTIFICATION_TITLE: usize = 120;
const MAX_NOTIFICATION_BODY: usize = 400;

/// The server embedded at build time. Native features are granted to its address only.
struct Server(url::Url);

/// The capture shortcut's label and whether it is paused.
struct ActiveShortcut(Mutex<(String, bool)>);

/// The tray capture item, kept in sync with the active personal binding.
struct CaptureMenu(Mutex<Option<MenuItem<Wry>>>);

/// Starts the desktop app.
pub fn run() {
    let context = tauri::generate_context!();
    #[cfg(target_os = "linux")]
    glib::set_prgname(Some(context.config().identifier.as_str()));

    let server = server_url::parse(env!("SPARROW_SERVER_URL"))
        .expect("the build must contain a valid PUBLIC_URL");

    tauri::Builder::default()
        // A second launch, such as clicking the app again, shows the running app instead. With
        // the deep-link feature it also hands a super-sparrow:// link to the running app.
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            show_main(app)
        }))
        .plugin(tauri_plugin_deep_link::init())
        .plugin(tauri_plugin_autostart::init(
            MacosLauncher::LaunchAgent,
            Some(vec![HIDDEN_ARGUMENT]),
        ))
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
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
        .manage(CaptureMenu(Mutex::new(None)))
        .manage(capture::CaptureState::default())
        .invoke_handler(tauri::generate_handler![
            hide_capture,
            current_shortcut,
            configure_shortcut,
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
            listen_for_links(&handle);
            // A development build would otherwise replace itself with the published release.
            if !cfg!(debug_assertions) {
                updater::start(&handle);
            }
            Ok(())
        })
        .run(context)
        .expect("failed to start the Super Sparrow desktop app");
}

/// Opens the CLI approval page for super-sparrow:// links, both the one the app was launched
/// with and those that arrive while it runs.
fn listen_for_links(app: &AppHandle) {
    // Installers register the scheme. A development build, or an AppImage on Linux, which no
    // installer sets up, registers itself here.
    #[cfg(any(target_os = "linux", all(debug_assertions, windows)))]
    if let Err(error) = app.deep_link().register_all() {
        eprintln!("could not register super-sparrow:// links: {error}");
    }

    let handle = app.clone();
    app.deep_link().on_open_url(move |event| {
        for link in event.urls() {
            open_link(&handle, &link);
        }
    });
    if let Ok(Some(links)) = app.deep_link().get_current() {
        for link in links {
            open_link(app, &link);
        }
    }
}

/// Shows the main window on the approval page a link names, and ignores any other link.
fn open_link(app: &AppHandle, link: &url::Url) {
    let server = app.state::<Server>();
    let Some(page) = cli_login::approval_page(link, &server.0) else {
        return;
    };
    if let Some(window) = app.get_webview_window(MAIN_WINDOW) {
        if let Err(error) = window.navigate(page) {
            eprintln!("could not open the CLI approval page: {error}");
        }
    }
    show_main(app);
}

/// Hides the capture window after a save or when the person closes it.
#[tauri::command]
fn hide_capture(app: AppHandle) {
    if let Some(window) = app.get_webview_window(CAPTURE_WINDOW) {
        let _ = window.hide();
    }
}

/// Returns the capture shortcut's label, such as "Meta+Shift+Q", or "" while it is paused, so the page
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

/// Applies the account's capture binding, preserving the previous accelerator if registration fails.
/// Temporary changes suspend capture during key recording without changing the saved binding.
#[tauri::command]
fn configure_shortcut(app: AppHandle, binding: String, temporary: bool) -> Result<(), String> {
    let paused = binding.is_empty();
    let label = if paused {
        shortcut::DEFAULT_SHORTCUT
    } else {
        &binding
    };
    apply_shortcut(&app, label, paused)?;
    if temporary {
        return Ok(());
    }
    let mut settings = Settings::load_or_empty(&app);
    settings.set_shortcut(label, paused);
    settings
        .save(&app)
        .map_err(|error| format!("save capture shortcut: {error}"))?;
    Ok(())
}

/// Registers one personal accelerator or disables it, rolling back a failed replacement.
fn apply_shortcut(app: &AppHandle, label: &str, paused: bool) -> Result<(), String> {
    let next = shortcut::parse(label).ok_or_else(|| format!("invalid global shortcut {label}"))?;
    let state = app.state::<ActiveShortcut>();
    let mut previous = state
        .0
        .lock()
        .map_err(|_| "capture shortcut state is unavailable")?;
    if previous.0 == label && previous.1 == paused {
        return Ok(());
    }
    let shortcuts = app.global_shortcut();
    shortcuts
        .unregister_all()
        .map_err(|error| format!("release capture shortcut: {error}"))?;
    if !paused && let Err(error) = shortcuts.register(next) {
        if !previous.1
            && let Some(old) = shortcut::parse(&previous.0)
            && let Err(restore) = shortcuts.register(old)
        {
            previous.1 = true;
            return Err(format!(
                "could not register {label}: {error}; restore previous shortcut: {restore}"
            ));
        }
        return Err(format!("could not register {label}: {error}"));
    }
    *previous = (label.to_string(), paused);
    if let Ok(item) = app.state::<CaptureMenu>().0.lock()
        && let Some(item) = item.as_ref()
    {
        item.set_accelerator(menu_accelerator(label, paused).as_deref())
            .map_err(|error| error.to_string())?;
    }
    if let Some(tray) = app.tray_by_id("main") {
        tray.set_tooltip(Some(tooltip(label, paused)))
            .map_err(|error| error.to_string())?;
    }
    Ok(())
}

/// Returns the active capture label and whether it is disabled.
fn active_shortcut(app: &AppHandle) -> (String, bool) {
    app.state::<ActiveShortcut>()
        .0
        .lock()
        .map(|current| current.clone())
        .unwrap_or_else(|_| (shortcut::DEFAULT_SHORTCUT.to_string(), true))
}

/// Returns the accelerator the tray's capture item shows, or None when it shows none. Windows
/// menus cannot show the Windows key, and the menu library refuses to build an item that names
/// it, so on Windows a Meta binding is left off the menu; the tooltip still names it.
fn menu_accelerator(label: &str, paused: bool) -> Option<String> {
    if paused || (cfg!(target_os = "windows") && label.split('+').any(|part| part == "Meta")) {
        return None;
    }
    Some(label.replace("Meta+", "Super+"))
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
            .permission("core:window:allow-start-dragging")
            .permission("allow-hide-capture")
            .permission("allow-current-shortcut")
            .permission("allow-configure-shortcut")
            .permission("allow-notify")
            .permission("allow-capture-screen")
            .permission("allow-end-capture"),
    )?;
    let main = WebviewWindowBuilder::new(app, MAIN_WINDOW, WebviewUrl::External(server.clone()))
        .title("Super Sparrow")
        .user_agent(USER_AGENT)
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
        .user_agent(USER_AGENT)
        .inner_size(560.0, 640.0)
        .min_inner_size(380.0, 360.0)
        .decorations(false)
        .transparent(true)
        .shadow(false)
        .always_on_top(true)
        .visible_on_all_workspaces(true)
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
    let _ = window.set_visible_on_all_workspaces(true);
    let _ = window.show();
    let _ = window.unminimize();
    let _ = window.set_focus();
    // Puts the keyboard in the title, which the page cannot do until the window owns the focus.
    let _ = window.eval("document.querySelector('#quick-capture-title')?.focus()");
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

fn build_tray(app: &AppHandle) -> tauri::Result<()> {
    let (label, paused) = active_shortcut(app);
    let accelerator = menu_accelerator(&label, paused);
    let open = MenuItem::with_id(app, "open", "Open Super Sparrow", true, None::<&str>)?;
    let capture = MenuItem::with_id(
        app,
        "capture",
        "Quick capture",
        true,
        accelerator.as_deref(),
    )?;
    let shortcut_settings =
        MenuItem::with_id(app, "shortcuts", "Keyboard shortcuts…", true, None::<&str>)?;

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
            &shortcut_settings,
            &start_at_login,
            &PredefinedMenuItem::separator(app)?,
            &quit,
        ],
    )?;
    *app.state::<CaptureMenu>().0.lock().map_err(|_| {
        tauri::Error::Anyhow(std::io::Error::other("capture menu is unavailable").into())
    })? = Some(capture);

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
            "shortcuts" => {
                show_main(app);
                if let Some(window) = app.get_webview_window(MAIN_WINDOW) {
                    match app.state::<Server>().0.join("settings/shortcuts") {
                        Ok(url) => {
                            if let Err(error) = window.navigate(url) {
                                eprintln!("open keyboard shortcuts: {error}");
                            }
                        }
                        Err(error) => eprintln!("open keyboard shortcuts: {error}"),
                    }
                }
            }
            _ => {}
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
    use super::{menu_accelerator, tooltip, truncate};

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

    #[test]
    fn menu_accelerator_leaves_out_paused_and_windows_key_bindings() {
        assert_eq!(
            menu_accelerator("Ctrl+Shift+Q", false).as_deref(),
            Some("Ctrl+Shift+Q")
        );
        assert_eq!(menu_accelerator("Ctrl+Shift+Q", true), None);
        assert_eq!(menu_accelerator("Meta+Shift+Q", true), None);

        let meta = menu_accelerator("Meta+Shift+Q", false);
        if cfg!(target_os = "windows") {
            assert_eq!(meta, None);
        } else {
            assert_eq!(meta.as_deref(), Some("Super+Shift+Q"));
        }
    }
}
