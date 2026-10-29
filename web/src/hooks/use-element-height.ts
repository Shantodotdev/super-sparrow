import { useEffect, useRef, useState } from "react";

/** Tracks the rendered height of an element, updating as it resizes. */
export function useElementHeight<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [height, setHeight] = useState(0);

  useEffect(() => {
    const element = ref.current;
    if (!element) return;

    const observer = new ResizeObserver(() => setHeight(element.getBoundingClientRect().height));
    observer.observe(element);

    return () => observer.disconnect();
  }, []);

  return { ref, height };
}
