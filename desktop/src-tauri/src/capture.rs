//! Native screenshots for the server's page. The window that asks steps aside while its monitor
//! is captured, then covers that monitor so the person can drag out the part they want; the page
//! crops the picture and uploads it.

use std::io::Cursor;
use std::sync::Mutex;
use std::time::Duration;

use image::ImageEncoder;
use image::codecs::png::{CompressionType, FilterType, PngEncoder};
use tauri::ipc::Response;
use tauri::{PhysicalPosition, PhysicalSize, State, WebviewWindow};

/// How long a hidden window takes to leave the screen before the capture, so it is not in it.
const HIDE_DELAY: Duration = Duration::from_millis(220);

/// Where each window was before it covered a monitor, so it can go back afterwards.
#[derive(Default)]
pub struct CaptureState(Mutex<Vec<(String, PhysicalPosition<i32>, PhysicalSize<u32>, bool)>>);

/// Captures the monitor the calling window is on and returns it as a PNG, leaving the window
/// covering that monitor until the page calls end_capture.
#[tauri::command]
pub async fn capture_screen(
    window: WebviewWindow,
    state: State<'_, CaptureState>,
) -> Result<Response, String> {
    let monitor = window
        .current_monitor()
        .map_err(|error| error.to_string())?
        .ok_or("the window is not on a monitor")?;
    let position = window.outer_position().map_err(|error| error.to_string())?;
    let size = window.inner_size().map_err(|error| error.to_string())?;
    let on_top = window.is_always_on_top().unwrap_or(false);
    if let Ok(mut saved) = state.0.lock() {
        saved.retain(|(label, ..)| label != window.label());
        saved.push((window.label().to_string(), position, size, on_top));
    }

    window.hide().map_err(|error| error.to_string())?;
    let center = (
        monitor.position().x + monitor.size().width as i32 / 2,
        monitor.position().y + monitor.size().height as i32 / 2,
    );
    let captured = tauri::async_runtime::spawn_blocking(move || {
        std::thread::sleep(HIDE_DELAY);
        capture_png(center.0, center.1)
    })
    .await
    .map_err(|error| error.to_string())
    .and_then(|result| result);

    // The window comes back whether or not the capture worked, covering the monitor on success.
    if captured.is_ok() {
        let _ = window.set_position(*monitor.position());
        let _ = window.set_size(*monitor.size());
        let _ = window.set_always_on_top(true);
    }
    let _ = window.show();
    let _ = window.set_focus();

    captured.map(Response::new)
}

/// Puts the calling window back where it was before capture_screen.
#[tauri::command]
pub fn end_capture(window: WebviewWindow, state: State<'_, CaptureState>) -> Result<(), String> {
    let previous = state.0.lock().ok().and_then(|mut saved| {
        let index = saved
            .iter()
            .position(|(label, ..)| label == window.label())?;
        Some(saved.remove(index))
    });
    if let Some((_, position, size, on_top)) = previous {
        window.set_size(size).map_err(|error| error.to_string())?;
        window
            .set_position(position)
            .map_err(|error| error.to_string())?;
        window
            .set_always_on_top(on_top)
            .map_err(|error| error.to_string())?;
    }
    window.set_focus().map_err(|error| error.to_string())
}

/// Captures the monitor at a point, in physical pixels, and encodes it as PNG.
fn capture_png(x: i32, y: i32) -> Result<Vec<u8>, String> {
    let monitor =
        xcap::Monitor::from_point(x, y).map_err(|error| format!("find the monitor: {error}"))?;
    let picture = monitor
        .capture_image()
        .map_err(|error| format!("capture the screen: {error}"))?;

    let mut png = Vec::new();
    // Fast compression keeps a 4K screen quick to encode; the page crops it before uploading.
    PngEncoder::new_with_quality(
        Cursor::new(&mut png),
        CompressionType::Fast,
        FilterType::Adaptive,
    )
    .write_image(
        picture.as_raw(),
        picture.width(),
        picture.height(),
        image::ExtendedColorType::Rgba8,
    )
    .map_err(|error| format!("encode the screenshot: {error}"))?;
    Ok(png)
}
