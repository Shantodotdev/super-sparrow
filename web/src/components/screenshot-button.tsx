import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { createPortal } from "react-dom";
import { Camera } from "lucide-react";
import { screenshotFilename } from "../lib/attachments";
import { cropToPng, dragRegion, grabScreen, type Region, type ScreenGrab } from "../lib/screenshot";

type ScreenshotButtonProps = {
  onCaptured: (file: File) => void;
  onError: (message: string) => void;
  className?: string;
};

/**
 * Takes a screenshot of part of the screen: it grabs the screen, lets the person drag over the
 * part they want, and hands that part over as a PNG file.
 */
export function ScreenshotButton({ onCaptured, onError, className = "" }: ScreenshotButtonProps) {
  const [grab, setGrab] = useState<ScreenGrab | null>(null);
  const [busy, setBusy] = useState(false);
  const button = useRef<HTMLButtonElement>(null);

  const start = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const taken = await grabScreen();
      if (taken) setGrab(taken);
      else setBusy(false);
    } catch (error) {
      setBusy(false);
      onError(error instanceof Error ? error.message : "Could not take a screenshot");
    }
  };

  const finish = async (region: Region | null) => {
    const taken = grab;
    setGrab(null);
    if (!taken) return;
    try {
      await taken.finish();
      if (!region) return;
      const png = await cropToPng(taken.image, region);
      onCaptured(new File([png], screenshotFilename(new Date()), { type: "image/png" }));
    } catch (error) {
      onError(error instanceof Error ? error.message : "Could not save the screenshot");
    } finally {
      taken.image.close();
      setBusy(false);
      // Focus returns to where the person was, so shortcuts such as Ctrl+Enter keep working.
      requestAnimationFrame(() => button.current?.focus());
    }
  };

  return (
    <>
      <button
        ref={button}
        type="button"
        className={`inline-flex items-center gap-[6px] rounded-full border border-[#36383e] bg-[#2a2c31] px-[9px] py-[5px] text-[12px] whitespace-nowrap text-[#d6d7dc] hover:border-[#484b54] hover:bg-[#383b42] disabled:opacity-60 ${className}`}
        onClick={() => void start()}
        disabled={busy}
        title="Drag over part of the screen to attach it"
      >
        <Camera aria-hidden="true" className="size-4" />
        {busy ? "Capturing…" : "Screenshot"}
      </button>
      {grab && createPortal(<RegionSelector image={grab.image} onDone={finish} />, document.body)}
    </>
  );
}

type Point = { x: number; y: number };

/** Shows a grabbed screen over the whole window and lets the person drag out a region of it. */
function RegionSelector({
  image,
  onDone,
}: {
  image: ImageBitmap;
  onDone: (region: Region | null) => void;
}) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [start, setStart] = useState<Point | null>(null);
  const [end, setEnd] = useState<Point | null>(null);
  const [size, setSize] = useState({ width: window.innerWidth, height: window.innerHeight });
  // The screen is shown whole, fitted to the window; scale converts shown pixels to image pixels.
  const scale = Math.max(image.width / size.width, image.height / size.height);
  const shown = { width: image.width / scale, height: image.height / scale };

  useEffect(() => {
    const resize = () => setSize({ width: window.innerWidth, height: window.innerHeight });
    window.addEventListener("resize", resize);
    return () => window.removeEventListener("resize", resize);
  }, []);

  useEffect(() => {
    const target = canvas.current;
    const context = target?.getContext("2d");
    if (!target || !context) return;
    const ratio = window.devicePixelRatio || 1;
    target.width = Math.round(shown.width * ratio);
    target.height = Math.round(shown.height * ratio);
    context.drawImage(image, 0, 0, target.width, target.height);
  }, [image, shown.width, shown.height]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopImmediatePropagation();
        onDone(null);
      } else if (event.key === "Enter") {
        event.preventDefault();
        event.stopImmediatePropagation();
        onDone({ x: 0, y: 0, width: image.width, height: image.height });
      }
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, [image, onDone]);

  const point = (event: ReactPointerEvent<HTMLElement>): Point => {
    const box = event.currentTarget.getBoundingClientRect();
    return { x: event.clientX - box.left, y: event.clientY - box.top };
  };

  const box =
    start && end
      ? {
          left: Math.min(start.x, end.x),
          top: Math.min(start.y, end.y),
          width: Math.abs(end.x - start.x),
          height: Math.abs(end.y - start.y),
        }
      : null;

  return (
    <div
      className="fixed inset-0 z-1000 grid cursor-crosshair place-items-center bg-black select-none"
      role="dialog"
      aria-label="Select part of the screen"
    >
      <div
        className="relative touch-none"
        style={{ width: shown.width, height: shown.height }}
        onPointerDown={(event) => {
          event.currentTarget.setPointerCapture(event.pointerId);
          setStart(point(event));
          setEnd(point(event));
        }}
        onPointerMove={(event) => {
          if (start) setEnd(point(event));
        }}
        onPointerUp={(event) => {
          if (!start) return;
          const region = dragRegion(start, point(event), scale, image);
          setStart(null);
          setEnd(null);
          if (region) onDone(region);
        }}
      >
        <canvas ref={canvas} className="block size-full" />
        <div className={`absolute inset-0 ${box ? "" : "bg-black/35"}`} />
        {box && (
          <div
            className="absolute border-2 border-[#4f7cff] shadow-[0_0_0_9999px_rgba(0,0,0,0.45)]"
            style={box}
          >
            <span className="absolute -top-7 left-0 rounded bg-[#1d2333] px-2 py-0.5 text-[11px] whitespace-nowrap text-white">
              {Math.round(box.width * scale)} × {Math.round(box.height * scale)}
            </span>
          </div>
        )}
      </div>
      <p className="pointer-events-none fixed top-4 left-1/2 m-0 -translate-x-1/2 rounded-full bg-[#1d2333]/95 px-4 py-2 text-[13px] text-[#e3e8f2] shadow-lg">
        Drag over the part you want · Enter for the whole screen · Esc to cancel
      </p>
    </div>
  );
}
