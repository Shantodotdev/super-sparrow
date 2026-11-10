import { desktopInvoke } from "./desktop";

/** A picture of the screen to select a region from, and how to put the window back afterwards. */
export type ScreenGrab = { image: ImageBitmap; finish: () => Promise<void> };

/** A rectangle in the grabbed image's own pixels. */
export type Region = { x: number; y: number; width: number; height: number };

/**
 * Takes a picture of the screen. The desktop app captures the monitor natively and stretches its
 * window over it so the region can be chosen in place; a browser asks which screen, window, or
 * tab to share and takes one frame of it. Returns null when the person cancels the browser's
 * picker.
 */
export async function grabScreen(): Promise<ScreenGrab | null> {
  const invoke = desktopInvoke();
  if (invoke) {
    // The bytes arrive as a buffer, or as a list of numbers when the app falls back to messages.
    const png = await invoke<ArrayBuffer | number[]>("capture_screen");
    const bytes = png instanceof ArrayBuffer ? png : new Uint8Array(png);
    const image = await createImageBitmap(new Blob([bytes], { type: "image/png" }));
    return { image, finish: () => invoke<void>("end_capture") };
  }

  if (!navigator.mediaDevices?.getDisplayMedia) {
    throw new Error("This browser cannot take screenshots. Paste an image instead.");
  }
  let stream: MediaStream;
  try {
    stream = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: false });
  } catch (error) {
    if (error instanceof DOMException && error.name === "NotAllowedError") return null;
    throw error;
  }

  try {
    const video = document.createElement("video");
    video.muted = true;
    video.srcObject = stream;
    await video.play();
    // The first frames can still be blank while the capture starts.
    await new Promise((resolve) => setTimeout(resolve, 150));
    const image = await createImageBitmap(video);
    return { image, finish: async () => undefined };
  } finally {
    for (const track of stream.getTracks()) track.stop();
  }
}

/** Cuts a region out of an image and encodes it as PNG. */
export async function cropToPng(image: ImageBitmap, region: Region): Promise<Blob> {
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(region.width));
  canvas.height = Math.max(1, Math.round(region.height));
  const context = canvas.getContext("2d");
  if (!context) throw new Error("Could not prepare the screenshot");
  context.drawImage(
    image,
    region.x,
    region.y,
    region.width,
    region.height,
    0,
    0,
    canvas.width,
    canvas.height,
  );

  return new Promise((resolve, reject) =>
    canvas.toBlob(
      (blob) => (blob ? resolve(blob) : reject(new Error("Could not encode the screenshot"))),
      "image/png",
    ),
  );
}

/**
 * Turns a drag between two points on the shown image into a region of the image's own pixels.
 * scale is the image's width divided by its shown width. Returns null for a drag too small to be
 * a deliberate selection.
 */
export function dragRegion(
  start: { x: number; y: number },
  end: { x: number; y: number },
  scale: number,
  bounds: { width: number; height: number },
): Region | null {
  const left = Math.max(0, Math.min(start.x, end.x));
  const top = Math.max(0, Math.min(start.y, end.y));
  const right = Math.min(bounds.width / scale, Math.max(start.x, end.x));
  const bottom = Math.min(bounds.height / scale, Math.max(start.y, end.y));
  if (right - left < 4 || bottom - top < 4) return null;

  return {
    x: Math.round(left * scale),
    y: Math.round(top * scale),
    width: Math.round((right - left) * scale),
    height: Math.round((bottom - top) * scale),
  };
}
