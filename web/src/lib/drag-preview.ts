import type { DragEvent } from "react";

const transparentPixel =
  "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7";

let blankImage: HTMLImageElement | undefined;

/** Hides the browser's translucent drag snapshot so the app can draw its own preview at the cursor. */
export function hideNativeDragImage(event: DragEvent): void {
  if (!blankImage) {
    blankImage = new Image();
    blankImage.src = transparentPixel;
  }

  event.dataTransfer.setDragImage(blankImage, 0, 0);
}
