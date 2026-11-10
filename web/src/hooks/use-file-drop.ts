import { useRef, useState, type DragEvent } from "react";
import { dragHasFiles, transferredFiles } from "../lib/attachments";

/**
 * Makes an element take dropped files. Spread `handlers` on it; `dragging` is true while files
 * are held over it. Drags that carry text or links are left alone.
 */
export function useFileDrop(onFiles: (files: File[]) => void, disabled = false) {
  const [dragging, setDragging] = useState(false);
  const depth = useRef(0);

  const accepts = (event: DragEvent) => !disabled && dragHasFiles(event.dataTransfer);

  const handlers = {
    onDragEnter: (event: DragEvent) => {
      if (!accepts(event)) return;
      depth.current += 1;
      setDragging(true);
    },
    onDragOver: (event: DragEvent) => {
      if (!accepts(event)) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = "copy";
    },
    onDragLeave: (event: DragEvent) => {
      if (!accepts(event)) return;
      depth.current = Math.max(0, depth.current - 1);
      if (depth.current === 0) setDragging(false);
    },
    onDrop: (event: DragEvent) => {
      if (!accepts(event)) return;
      event.preventDefault();
      depth.current = 0;
      setDragging(false);
      onFiles(transferredFiles(event.dataTransfer));
    },
  };

  return { dragging, handlers };
}
