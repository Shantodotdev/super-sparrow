import { useEffect } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { QuickCapture } from "../components/quick-capture";
import { desktopInvoke } from "../lib/desktop";

export const Route = createFileRoute("/capture")({
  staticData: { bare: true },
  component: CapturePage,
});

/**
 * The page of the desktop app's capture window, which the capture shortcut shows over any application. It holds
 * only the quick capture panel; saving or closing hides the window, keeping it ready for next time.
 */
function CapturePage() {
  // The title takes the focus as soon as the window opens and whenever it is shown again, so
  // typing can start at once. A webview may not own the keyboard yet when the page loads, so the
  // first attempts repeat for a moment.
  useEffect(() => {
    const focusTitle = () => {
      const title = document.querySelector<HTMLInputElement>("#quick-capture-title");
      if (!title || document.activeElement === title) return;
      const active = document.activeElement;
      if (active && active !== document.body && active.closest("[role=dialog]")) return;
      title.focus();
    };
    const timers = [0, 50, 150, 400].map((delay) => window.setTimeout(focusTitle, delay));
    window.addEventListener("focus", focusTitle);
    document.addEventListener("visibilitychange", focusTitle);
    return () => {
      timers.forEach((timer) => window.clearTimeout(timer));
      window.removeEventListener("focus", focusTitle);
      document.removeEventListener("visibilitychange", focusTitle);
    };
  }, []);

  // The window is transparent so the panel's rounded corners show; the page must not paint behind it.
  useEffect(() => {
    const { documentElement, body } = document;
    const previousRoot = documentElement.style.background;
    const previousBody = body.style.background;
    documentElement.style.background = "transparent";
    body.style.background = "transparent";
    return () => {
      documentElement.style.background = previousRoot;
      body.style.background = previousBody;
    };
  }, []);

  const hide = () => {
    void desktopInvoke()?.("hide_capture");
  };

  return <QuickCapture open standalone onClose={hide} />;
}
