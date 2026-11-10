import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { createPortal } from "react-dom";
import { Camera, MousePointer2, ScanLine, X } from "lucide-react";
import { apiUploadImage } from "../lib/api";
import { cropToPng, dragRegion, grabScreen, type Region, type ScreenGrab } from "../lib/screenshot";
import type { Attachment } from "../lib/types";

type ScreenshotButtonProps = {
  projectKey: string;
  onCaptured: (attachment: Attachment) => void;
  onError: (message: string) => void;
  /** Shows only the camera icon, for toolbars. */
  compact?: boolean;
  className?: string;
};

/**
 * Takes a screenshot of part of the screen: it grabs the screen, lets the person drag over the
 * part they want and confirm it, and uploads that part to the project.
 */
export function ScreenshotButton({
  projectKey,
  onCaptured,
  onError,
  compact = false,
  className = "",
}: ScreenshotButtonProps) {
  const [grab, setGrab] = useState<ScreenGrab | null>(null);
  const [busy, setBusy] = useState(false);
  const button = useRef<HTMLButtonElement>(null);

  const start = async () => {
    if (busy || !projectKey) return;
    setBusy(true);
    try {
      const taken = await grabScreen();
      if (taken) setGrab(taken);
      else setBusy(false);
    } catch (error) {
      setBusy(false);
      onError(describeFailure(error, "Could not take a screenshot"));
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
      onCaptured(await apiUploadImage(projectKey, png));
    } catch (error) {
      onError(describeFailure(error, "Could not save the screenshot"));
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
        aria-label="Take a screenshot"
        className={
          compact
            ? `grid size-8 place-items-center rounded-lg border border-[#36383e] bg-[#2a2c31] text-[#d7dfeb] hover:border-[#484b54] hover:bg-[#383b42] disabled:opacity-60 ${busy ? "animate-pulse" : ""} ${className}`
            : `inline-flex items-center gap-[6px] rounded-full border border-[#36383e] bg-[#2a2c31] px-[9px] py-[5px] text-[12px] whitespace-nowrap text-[#d6d7dc] hover:border-[#484b54] hover:bg-[#383b42] disabled:opacity-60 ${className}`
        }
        onClick={() => void start()}
        disabled={busy || !projectKey}
        title="Drag over part of the screen to attach it"
      >
        <Camera aria-hidden="true" className="size-4" />
        {!compact && (busy ? "Capturing…" : "Screenshot")}
      </button>
      {grab && createPortal(<RegionSelector image={grab.image} onDone={finish} />, document.body)}
    </>
  );
}

/** Says what went wrong; the desktop app reports its failures as plain text, not errors. */
function describeFailure(error: unknown, fallback: string): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "string" && error) return `${fallback}: ${error}`;
  return fallback;
}

type Point = { x: number; y: number };

const handles = ["-top-1 -left-1", "-top-1 -right-1", "-bottom-1 -left-1", "-bottom-1 -right-1"];

/**
 * Shows a grabbed screen over the whole window. The person drags out a region, which they can
 * drag again to change, then confirms it with Capture or Enter; Escape or × cancels.
 */
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
  const [dragging, setDragging] = useState(false);
  const [pointer, setPointer] = useState<Point | null>(null);
  const [size, setSize] = useState({ width: window.innerWidth, height: window.innerHeight });
  // The screen is shown whole, fitted to the window; scale converts shown pixels to image pixels.
  const scale = Math.max(image.width / size.width, image.height / size.height);
  const shown = { width: image.width / scale, height: image.height / scale };
  const region = start && end ? dragRegion(start, end, scale, image) : null;
  const latest = useRef({ region, onDone });
  latest.current = { region, onDone };

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
      if (event.key !== "Escape" && event.key !== "Enter") return;
      event.preventDefault();
      event.stopImmediatePropagation();
      const { region: selected, onDone: done } = latest.current;
      if (event.key === "Escape") done(null);
      else done(selected ?? { x: 0, y: 0, width: image.width, height: image.height });
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, [image]);

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
  const settled = !dragging && region && box;
  // The confirm bar sits under the selection, or above it when the selection reaches the bottom.
  const barBelow = box ? box.top + box.height + 48 < shown.height : true;

  return (
    <div
      className="fixed inset-0 z-1000 grid place-items-center bg-black select-none"
      role="dialog"
      aria-label="Select part of the screen"
    >
      <div
        className="relative cursor-crosshair touch-none"
        style={{ width: shown.width, height: shown.height }}
        onPointerDown={(event) => {
          if ((event.target as HTMLElement).closest("button")) return;
          event.currentTarget.setPointerCapture(event.pointerId);
          setStart(point(event));
          setEnd(point(event));
          setDragging(true);
        }}
        onPointerMove={(event) => {
          setPointer(point(event));
          if (dragging) setEnd(point(event));
        }}
        onPointerUp={(event) => {
          if (!dragging) return;
          setDragging(false);
          setEnd(point(event));
        }}
        onPointerLeave={() => setPointer(null)}
      >
        <canvas ref={canvas} className="block size-full" />
        <div className={`absolute inset-0 ${box ? "" : "bg-black/40"}`} />

        {box && (
          <div
            className="absolute border-2 border-dashed border-white/90 shadow-[0_0_0_9999px_rgba(0,0,0,0.5)]"
            style={box}
          >
            {handles.map((place) => (
              <span
                key={place}
                className={`absolute size-2 rounded-[2px] border border-[#2455eb] bg-white ${place}`}
              />
            ))}
            {region && (
              <span className="absolute -top-7 right-0 rounded-md bg-[#15171b]/90 px-2 py-0.5 text-[11px] whitespace-nowrap text-[#d7dfeb]">
                {region.width} × {region.height}
              </span>
            )}
          </div>
        )}

        {!settled && pointer && (
          <span
            className="pointer-events-none absolute flex items-center gap-1.5 rounded-md bg-[#15171b]/95 px-2 py-1 text-[11px] whitespace-nowrap text-[#e3e8f2] shadow-lg"
            style={{ left: pointer.x + 14, top: pointer.y - 30 }}
          >
            <MousePointer2 aria-hidden="true" className="size-3" />
            Drag to select area
          </span>
        )}

        {settled && (
          <div
            className="absolute flex items-center gap-1 rounded-lg border border-[#343842] bg-[#15171b] p-1 shadow-[0_8px_24px_#000a]"
            style={{
              left: Math.min(box.left, shown.width - 150),
              top: barBelow ? box.top + box.height + 8 : box.top - 44,
            }}
          >
            <button
              type="button"
              className="inline-flex items-center gap-1.5 rounded-md border-0 bg-[#2455eb] px-2.5 py-1 text-[12px] font-semibold text-white hover:bg-[#3566f6]"
              onClick={() => onDone(region)}
            >
              <ScanLine aria-hidden="true" className="size-3.5" />
              Capture
            </button>
            <span aria-hidden="true" className="h-4 w-px bg-[#343842]" />
            <button
              type="button"
              aria-label="Cancel screenshot"
              className="grid size-6 place-items-center rounded-md border-0 bg-transparent text-[#a5a8b1] hover:bg-[#2b2e35] hover:text-white"
              onClick={() => onDone(null)}
            >
              <X aria-hidden="true" className="size-3.5" />
            </button>
          </div>
        )}
      </div>
      <p className="pointer-events-none fixed top-4 left-1/2 m-0 -translate-x-1/2 rounded-full bg-[#15171b]/95 px-4 py-2 text-[12px] text-[#c9d1dd] shadow-lg">
        Drag to select · Enter to capture · Esc to cancel
      </p>
    </div>
  );
}
