import { useEffect, useState } from "react";
import { desktopInvoke } from "../lib/desktop";

/** The browser's quick capture shortcut, which the app shell handles on its pages. */
const browserShortcut = "Ctrl Q";

/**
 * Returns the label of the shortcut that opens quick capture. The desktop app reports the
 * system-wide shortcut the person chose, or "" while it is paused; it is read again whenever the
 * window gains focus, since the tray menu can change it at any time.
 */
export function useCaptureShortcut(): string {
  const [shortcut, setShortcut] = useState(browserShortcut);

  useEffect(() => {
    const invoke = desktopInvoke();
    if (!invoke) return;

    const read = () => {
      invoke<string>("current_shortcut").then(setShortcut, () => undefined);
    };
    read();
    window.addEventListener("focus", read);
    return () => window.removeEventListener("focus", read);
  }, []);

  return shortcut;
}
