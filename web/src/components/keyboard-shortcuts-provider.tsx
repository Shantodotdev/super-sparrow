import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useLocation } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  defaultBindings,
  resolveBindings,
  type ShortcutBindings,
  type ShortcutID,
} from "../lib/shortcuts";
import { desktopInvoke } from "../lib/desktop";
import { meQuery } from "../queries/project";
import { keyboardPreferencesQuery } from "../queries/keyboard-shortcuts";

const KeyboardContext = createContext<{
  bindings: ShortcutBindings;
  desktopError: string;
  recording: ShortcutID | null;
  setRecording: Dispatch<SetStateAction<ShortcutID | null>>;
  recordingReady: boolean;
}>({
  bindings: defaultBindings,
  desktopError: "",
  recording: null,
  setRecording: () => undefined,
  recordingReady: false,
});

/** Shares server-backed personal bindings and synchronizes the desktop's one global accelerator. */
export function KeyboardShortcutsProvider({ children }: { children: ReactNode }) {
  const path = useLocation({ select: (location) => location.pathname });
  const identity = useQuery({ ...meQuery, enabled: path !== "/signin", retry: false });
  const preferences = useQuery(
    keyboardPreferencesQuery(path === "/signin" ? "" : (identity.data?.id ?? "")),
  );
  const bindings = resolveBindings(preferences.data?.overrides ?? {});
  const [desktopError, setDesktopError] = useState("");
  const [recording, setRecording] = useState<ShortcutID | null>(null);
  const [readyRecording, setReadyRecording] = useState<ShortcutID | null>(null);
  const capture = bindings.quickCapture;
  const ready = preferences.isSuccess;

  useEffect(() => {
    const invoke = desktopInvoke();
    if (!ready) return;
    if (!invoke) {
      setReadyRecording(recording);
      return;
    }
    let active = true;
    void invoke("configure_shortcut", {
      binding: recording ? "" : capture,
      temporary: !!recording,
    }).then(
      () => {
        if (active) {
          setDesktopError("");
          setReadyRecording(recording);
        }
      },
      (error) => {
        if (active) setDesktopError(String(error));
      },
    );
    return () => {
      active = false;
    };
  }, [capture, ready, preferences.data?.revision, recording]);

  useEffect(() => {
    const reset = () => {
      setRecording(null);
      void desktopInvoke()?.("configure_shortcut", {
        binding: defaultBindings.quickCapture,
        temporary: false,
      }).catch(() => undefined);
    };
    window.addEventListener("sparrow:clear-cache", reset);
    return () => window.removeEventListener("sparrow:clear-cache", reset);
  }, []);

  useEffect(() => {
    const restore = () => {
      if (recording)
        void desktopInvoke()?.("configure_shortcut", { binding: capture, temporary: false }).catch(
          () => undefined,
        );
    };
    window.addEventListener("pagehide", restore);
    return () => window.removeEventListener("pagehide", restore);
  }, [recording, capture]);

  return (
    <KeyboardContext.Provider
      value={{
        bindings,
        desktopError,
        recording,
        setRecording,
        recordingReady: recording !== null && readyRecording === recording,
      }}
    >
      {children}
    </KeyboardContext.Provider>
  );
}

/** Reads the effective bindings without copying server state into component state. */
export function useKeyboardShortcuts() {
  return useContext(KeyboardContext);
}
