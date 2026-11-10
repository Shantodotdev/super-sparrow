import { useKeyboardShortcuts } from "../components/keyboard-shortcuts-provider";

/** Returns the signed-in person's capture binding, shared by the browser and desktop. */
export function useCaptureShortcut(): string {
  return useKeyboardShortcuts().bindings.quickCapture;
}
