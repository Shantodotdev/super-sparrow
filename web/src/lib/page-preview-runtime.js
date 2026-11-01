(() => {
  /** @typedef {string | CachedElement} CachedNode */
  /** @typedef {{tag: string, svg: boolean, attrs: [string, string][], children: CachedNode[], scrollTop: number, scrollLeft: number, value?: string, checked?: boolean}} CachedElement */
  /** @typedef {{version: number, url: string, timestamp: number, styles: string[], root: CachedNode, scrollX: number, scrollY: number}} PageSnapshot */

  const host = document.getElementById("page-preview");
  if (!host || host.shadowRoot) return;

  const shadow = host.attachShadow({ mode: "open" });
  const storageKey = "super-sparrow:page-preview:v1";
  const htmlTags = new Set([
    "a",
    "abbr",
    "article",
    "aside",
    "b",
    "blockquote",
    "br",
    "button",
    "caption",
    "code",
    "col",
    "colgroup",
    "dd",
    "del",
    "details",
    "div",
    "dl",
    "dt",
    "em",
    "fieldset",
    "figcaption",
    "figure",
    "footer",
    "form",
    "h1",
    "h2",
    "h3",
    "h4",
    "h5",
    "h6",
    "header",
    "hr",
    "i",
    "img",
    "input",
    "kbd",
    "label",
    "legend",
    "li",
    "main",
    "mark",
    "nav",
    "ol",
    "optgroup",
    "option",
    "p",
    "pre",
    "s",
    "section",
    "select",
    "small",
    "span",
    "strong",
    "sub",
    "summary",
    "sup",
    "table",
    "tbody",
    "td",
    "textarea",
    "th",
    "thead",
    "time",
    "tr",
    "u",
    "ul",
  ]);
  const svgTags = new Set([
    "svg",
    "g",
    "path",
    "circle",
    "ellipse",
    "line",
    "polyline",
    "polygon",
    "rect",
    "defs",
    "clipPath",
    "title",
    "text",
    "tspan",
  ]);
  const attributes = new Set([
    "class",
    "id",
    "title",
    "role",
    "width",
    "height",
    "alt",
    "type",
    "placeholder",
    "checked",
    "disabled",
    "selected",
    "start",
    "reversed",
    "open",
    "colspan",
    "rowspan",
    "dir",
    "lang",
    "viewBox",
    "fill",
    "fill-rule",
    "clip-rule",
    "stroke",
    "stroke-width",
    "stroke-linecap",
    "stroke-linejoin",
    "stroke-dasharray",
    "stroke-dashoffset",
    "d",
    "points",
    "cx",
    "cy",
    "r",
    "rx",
    "ry",
    "x",
    "y",
    "x1",
    "x2",
    "y1",
    "y2",
    "transform",
    "opacity",
  ]);
  let live = false;
  let captureTimer = 0;
  /** @type {PageSnapshot | undefined} */
  let restored;

  function pageURL() {
    return location.pathname + location.search + location.hash;
  }

  /** Accepts only app-owned assets; cached markup cannot introduce remote resources. */
  function localAsset(/** @type {string} */ value) {
    try {
      const url = new URL(value, location.origin);
      return url.origin === location.origin && ["http:", "https:"].includes(url.protocol)
        ? url.href
        : null;
    } catch {
      return null;
    }
  }

  /** Rebuilds visual attributes without event handlers, navigation, or executable HTML. */
  function applyAttribute(
    /** @type {Element} */ element,
    /** @type {string} */ name,
    /** @type {string} */ value,
  ) {
    if (name === "src" && element.localName === "img") {
      const src = localAsset(value);
      if (src) element.setAttribute("src", src);
    } else if (name === "style") {
      if (!/url\s*\(|expression\s*\(|javascript:|@|\\/i.test(value)) {
        if (element instanceof HTMLElement || element instanceof SVGElement)
          element.style.cssText = value;
      }
    } else if (attributes.has(name) || /^(aria|data)-[a-z0-9-]+$/.test(name)) {
      element.setAttribute(name, value);
    }
  }

  /** Serializes a visual tree rather than storing HTML that could execute on restoration. */
  function captureNode(/** @type {Node} */ node, /** @type {{remaining: number}} */ budget) {
    if (--budget.remaining < 0) return null;
    if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? "";
    if (!(node instanceof Element)) return null;
    if (node.hasAttribute("data-page-cache-exclude") || node.getAttribute("role") === "dialog")
      return null;

    const svg = node.namespaceURI === "http://www.w3.org/2000/svg";
    if (!(svg ? svgTags : htmlTags).has(node.localName)) return null;
    if (node instanceof HTMLInputElement && ["password", "hidden", "file"].includes(node.type))
      return null;

    /** @type {CachedElement} */
    const saved = {
      tag: node.localName,
      svg,
      attrs: Array.from(node.attributes, (attribute) => [attribute.name, attribute.value]),
      children: [],
      scrollTop: node.scrollTop,
      scrollLeft: node.scrollLeft,
    };
    if (
      node instanceof HTMLInputElement ||
      node instanceof HTMLTextAreaElement ||
      node instanceof HTMLSelectElement
    )
      saved.value = node.value;
    if (node instanceof HTMLInputElement) saved.checked = node.checked;

    for (const child of node.childNodes) {
      const captured = captureNode(child, budget);
      if (captured !== null) saved.children.push(captured);
    }
    return saved;
  }

  /** Constructs inert DOM from an allowlist; cached scripts and arbitrary elements are discarded. */
  function restoreNode(
    /** @type {CachedNode} */ saved,
    /** @type {{remaining: number}} */ budget,
    /** @type {number} */ depth,
  ) {
    if (--budget.remaining < 0 || depth > 100) return null;
    if (typeof saved === "string") return document.createTextNode(saved);
    if (!saved || typeof saved !== "object" || !(saved.svg ? svgTags : htmlTags).has(saved.tag))
      return null;
    if (!Array.isArray(saved.attrs) || !Array.isArray(saved.children)) return null;

    const element = saved.svg
      ? document.createElementNS("http://www.w3.org/2000/svg", saved.tag)
      : document.createElement(saved.tag);
    for (const pair of saved.attrs) {
      if (Array.isArray(pair) && typeof pair[0] === "string" && typeof pair[1] === "string")
        applyAttribute(element, pair[0], pair[1]);
    }
    for (const child of saved.children) {
      const restoredChild = restoreNode(child, budget, depth + 1);
      if (restoredChild) element.append(restoredChild);
    }
    if (
      typeof saved.value === "string" &&
      (element instanceof HTMLInputElement ||
        element instanceof HTMLTextAreaElement ||
        element instanceof HTMLSelectElement)
    )
      element.value = saved.value;
    if (element instanceof HTMLInputElement && typeof saved.checked === "boolean")
      element.checked = saved.checked;
    if (saved.scrollTop || saved.scrollLeft)
      requestAnimationFrame(() => {
        if (Number.isFinite(saved.scrollTop)) element.scrollTop = saved.scrollTop;
        if (Number.isFinite(saved.scrollLeft)) element.scrollLeft = saved.scrollLeft;
      });
    return element;
  }

  /** Saves the current signed-in frame, including its scroll position, independently of React boot. */
  function capturePage() {
    captureTimer = 0;
    if (!live || location.pathname.replace(/\/+$/, "") === "/signin") return;
    const frame = document.querySelector("[data-page-cache]");
    if (!frame) return;

    try {
      const root = captureNode(frame, { remaining: 50_000 });
      if (!root) return;
      const styles = Array.from(document.querySelectorAll('link[rel="stylesheet"]'))
        .map((link) => localAsset(link.getAttribute("href") ?? ""))
        .filter((href) => href !== null);
      /** @type {PageSnapshot} */
      const snapshot = {
        version: 1,
        url: pageURL(),
        timestamp: Date.now(),
        styles,
        root,
        scrollX: window.scrollX,
        scrollY: window.scrollY,
      };
      sessionStorage.setItem(storageKey, JSON.stringify(snapshot));
    } catch {
      // An unavailable or full store leaves the normal app startup available.
    }
  }

  function scheduleCapture() {
    if (live && !captureTimer) captureTimer = window.setTimeout(capturePage, 300);
  }

  /** Releases the live page without changing the last successfully saved preview. */
  function dismissPreview() {
    live = false;
    restored = undefined;
    clearTimeout(captureTimer);
    captureTimer = 0;
    host?.removeAttribute("data-active");
    shadow.replaceChildren();
    const content = document.getElementById("app-content");
    if (content) content.inert = false;
  }

  /** Removes both the preview and its saved copy as soon as the session ends. */
  function clearPage() {
    dismissPreview();
    try {
      sessionStorage.removeItem(storageKey);
    } catch {
      // Sign-out does not depend on browser storage being available.
    }
  }

  // The shadow tree is outside React's hydrated tree and remains non-interactive until handoff.
  try {
    const raw = sessionStorage.getItem(storageKey);
    if (raw && location.pathname.replace(/\/+$/, "") !== "/signin") {
      /** @type {PageSnapshot} */
      const snapshot = JSON.parse(raw);
      const age = Date.now() - snapshot.timestamp;
      if (
        snapshot.version === 1 &&
        snapshot.url === pageURL() &&
        Number.isFinite(age) &&
        age >= 0 &&
        age < 60 * 60 * 1000 &&
        Array.isArray(snapshot.styles)
      ) {
        const root = restoreNode(snapshot.root, { remaining: 50_000 }, 0);
        if (root) {
          const currentStyles = Array.from(document.querySelectorAll('link[rel="stylesheet"]'))
            .map((link) => link.getAttribute("href"))
            .filter((href) => href !== null);
          for (const href of currentStyles.length ? currentStyles : snapshot.styles) {
            if (typeof href !== "string") continue;
            const src = localAsset(href);
            if (!src) continue;
            const link = document.createElement("link");
            link.rel = "stylesheet";
            link.href = src;
            shadow.append(link);
          }
          const viewport = document.createElement("div");
          viewport.style.height = "100dvh";
          viewport.style.overflow = "auto";
          viewport.append(root);
          shadow.append(viewport);
          const content = document.getElementById("app-content");
          if (content) content.inert = true;
          host.setAttribute("data-active", "");
          restored = snapshot;
          requestAnimationFrame(() => {
            viewport.scrollTop = Number.isFinite(snapshot.scrollY) ? snapshot.scrollY : 0;
            viewport.scrollLeft = Number.isFinite(snapshot.scrollX) ? snapshot.scrollX : 0;
          });
        }
      }
    }
  } catch {
    clearPage();
  }

  window.addEventListener("sparrow:page-ready", () => {
    if (restored?.url === pageURL()) window.scrollTo(restored.scrollX, restored.scrollY);
    dismissPreview();
    live = true;
    capturePage();
  });
  window.addEventListener("sparrow:page-error", dismissPreview);
  window.addEventListener("sparrow:clear-cache", clearPage);
  window.addEventListener("pagehide", capturePage);
  window.addEventListener("scroll", scheduleCapture, true);
  new MutationObserver(scheduleCapture).observe(document.body, {
    subtree: true,
    childList: true,
    characterData: true,
    attributes: true,
  });
})();
