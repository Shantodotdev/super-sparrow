import type { QueryClient } from "@tanstack/react-query";
import previewScript from "./page-preview-runtime.js?url";

/** Releases the preview and its interaction lock when the live page cannot render successfully. */
export function dismissPagePreview(): void {
  window.dispatchEvent(new Event("sparrow:page-error"));
}

/** Hands the pre-React page preview to live UI after uncached reads have settled. */
export function watchPagePreview(client: QueryClient): () => void {
  let frame = 0;
  let stopped = false;

  const schedule = () => {
    if (frame || stopped) return;
    frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() => {
        frame = 0;
        if (stopped) return;
        if (window.location.pathname.replace(/\/+$/, "") === "/signin") {
          window.dispatchEvent(new Event("sparrow:clear-cache"));
          return;
        }
        if (!client.getQueryData(["me"])) return;
        if (
          client
            .getQueryCache()
            .findAll({ type: "active" })
            .some(
              (query) => query.state.data === undefined && query.state.fetchStatus === "fetching",
            )
        )
          return;

        window.dispatchEvent(new Event("sparrow:page-ready"));
      });
    });
  };

  // Hot updates can insert the shell without executing its parser-time script.
  let bootstrap: HTMLScriptElement | undefined;
  if (!document.getElementById("page-preview")?.shadowRoot) {
    bootstrap = document.createElement("script");
    bootstrap.src = previewScript;
    bootstrap.addEventListener("load", schedule, { once: true });
    document.head.append(bootstrap);
  }

  const unsubscribe = client.getQueryCache().subscribe(schedule);
  schedule();

  return () => {
    stopped = true;
    cancelAnimationFrame(frame);
    bootstrap?.removeEventListener("load", schedule);
    unsubscribe();
  };
}
