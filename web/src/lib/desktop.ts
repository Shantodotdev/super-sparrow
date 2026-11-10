/** Calls a command of the Super Sparrow desktop app, which the app grants to the server's page. */
type Invoke = <T>(command: string, args?: Record<string, unknown>) => Promise<T>;

type DesktopWindow = { __TAURI__?: { core?: { invoke?: Invoke } } };

/**
 * Returns the desktop app's command bridge when the page runs inside the desktop app, and null in
 * a browser. The app exposes the bridge only to the server's address it was set up with.
 */
export function desktopInvoke(): Invoke | null {
  if (typeof window === "undefined") return null;
  return (window as unknown as DesktopWindow).__TAURI__?.core?.invoke ?? null;
}

/** Whether the page runs inside the desktop app. */
export function isDesktop(): boolean {
  return desktopInvoke() !== null;
}
