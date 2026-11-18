const storageKey = "sparrow.lastProject";

/** The slug of the project the person last opened on this device, if any. */
export function readLastProject(): string | undefined {
  try {
    return window.localStorage.getItem(storageKey) ?? undefined;
  } catch {
    // Blocked storage means no remembered project.
    return undefined;
  }
}

/** Remembers the project so the next visit opens it. */
export function saveLastProject(slug: string): void {
  try {
    window.localStorage.setItem(storageKey, slug);
  } catch {
    // Remembering the project is a convenience; blocked storage forgets it.
  }
}
