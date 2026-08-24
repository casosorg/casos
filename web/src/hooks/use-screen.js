import {useEffect, useState} from "react";

/** Below this the desktop stops pretending to be a desktop. */
const MOBILE_BREAKPOINT = 768;

function query(width) {
  return `(max-width: ${width - 1}px)`;
}

/** True while the viewport is narrower than the given breakpoint. */
export function useIsNarrow(width = MOBILE_BREAKPOINT) {
  const [narrow, setNarrow] = useState(() => (typeof window === "undefined" ? false : window.matchMedia(query(width)).matches));

  useEffect(() => {
    const media = window.matchMedia(query(width));
    const update = (event) => setNarrow(event.matches);
    setNarrow(media.matches);
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, [width]);

  return narrow;
}
