import { mkdir, writeFile } from "node:fs/promises";
import { homedir } from "node:os";
import { isAbsolute, resolve } from "node:path";
import config from "../src-tauri/tauri.conf.json";
import development from "../src-tauri/tauri.dev.conf.json";

/** Quotes an absolute path for the desktop entry Exec field, including its field-code escapes. */
function quoteExecutable(path: string): string {
  return `"${path.replace(/%/g, "%%").replace(/[\\"`$]/g, "\\$&")}"`;
}

// Wayland resolves taskbar artwork through a desktop entry rather than a window's pixel icon.
if (process.platform === "linux") {
  const dataHome = process.env.XDG_DATA_HOME || resolve(homedir(), ".local/share");
  if (!isAbsolute(dataHome)) throw new Error("XDG_DATA_HOME must be an absolute path");

  const desktopRoot = resolve(import.meta.dir, "..");
  const applications = resolve(dataHome, "applications");
  const icon = resolve(desktopRoot, "src-tauri/icons/128x128.png");
  const executable = resolve(desktopRoot, "src-tauri/target/debug/super-sparrow");
  const entry = [
    "[Desktop Entry]",
    "Type=Application",
    `Name=${config.productName} (Development)`,
    `Exec=${quoteExecutable(executable)}`,
    `Icon=${icon}`,
    `StartupWMClass=${development.identifier}`,
    "Terminal=false",
    "NoDisplay=true",
    "Categories=Development;",
    "",
  ].join("\n");

  await mkdir(applications, { recursive: true });
  await writeFile(resolve(applications, `${development.identifier}.desktop`), entry);
}
