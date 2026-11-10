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
  // Each time the window is shown again, the title takes the focus so typing can start at once.
  useEffect(() => {
    const focusTitle = () => {
      if (document.activeElement && document.activeElement !== document.body) return;
      document.querySelector<HTMLInputElement>("#quick-capture-title")?.focus();
    };
    window.addEventListener("focus", focusTitle);
    return () => window.removeEventListener("focus", focusTitle);
  }, []);

  const hide = () => {
    void desktopInvoke()?.("hide_capture");
  };

  return <QuickCapture open standalone onClose={hide} />;
}
