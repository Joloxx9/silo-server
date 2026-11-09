import { useCallback, useState, type SyntheticEvent } from "react";

/**
 * Title logos from fanart.tv and TVDB come on a fixed 800×310 canvas, with
 * the artwork centred in up to a third of transparent padding on each side.
 * Drawn as-is, a logo looks small, floats away from the left edge it is
 * aligned to, and leaves a gap above the text under it. This trims that
 * padding in the browser, once per logo URL, so a logo can be sized and
 * aligned by its visible artwork.
 */

// The trimmed image for each logo URL: an object URL, or null when the logo
// has nothing to trim or cannot be read (a cross-origin image taints the
// canvas). Object URLs live for the session; a cap keeps a long session from
// holding every logo it ever showed.
const trimmed = new Map<string, string | null>();
const MAX_CACHED = 300;

// Alpha at or below this counts as empty, so faint antialiasing or a stray
// near-transparent pixel at the canvas edge does not hold the padding.
const ALPHA_THRESHOLD = 8;
// The measuring pass runs on a copy no wider than this.
const MEASURE_WIDTH = 400;

function remember(url: string, value: string | null) {
  if (trimmed.size >= MAX_CACHED) {
    const oldest = trimmed.keys().next().value;
    if (oldest !== undefined) {
      const stale = trimmed.get(oldest);
      if (stale) URL.revokeObjectURL(stale);
      trimmed.delete(oldest);
    }
  }
  trimmed.set(url, value);
}

type Box = { left: number; top: number; right: number; bottom: number };

/** The visible bounds of a loaded image in its own pixels, or null. */
function visibleBounds(img: HTMLImageElement): Box | null {
  const width = img.naturalWidth;
  const height = img.naturalHeight;
  if (!width || !height) return null;
  const scale = Math.min(1, MEASURE_WIDTH / width);
  const w = Math.max(1, Math.round(width * scale));
  const h = Math.max(1, Math.round(height * scale));
  const canvas = document.createElement("canvas");
  canvas.width = w;
  canvas.height = h;
  const ctx = canvas.getContext("2d", { willReadFrequently: true });
  if (!ctx) return null;
  ctx.drawImage(img, 0, 0, w, h);
  const data = ctx.getImageData(0, 0, w, h).data;
  let left = w;
  let top = h;
  let right = -1;
  let bottom = -1;
  for (let y = 0; y < h; y++) {
    const row = y * w * 4;
    for (let x = 0; x < w; x++) {
      if ((data[row + x * 4 + 3] ?? 0) > ALPHA_THRESHOLD) {
        if (x < left) left = x;
        if (x > right) right = x;
        if (y < top) top = y;
        bottom = y;
      }
    }
  }
  if (right < 0) return null;
  // Map back to full size, widened by a measuring pixel so scaling never
  // clips an edge.
  return {
    left: Math.max(0, Math.floor((left - 1) / scale)),
    top: Math.max(0, Math.floor((top - 1) / scale)),
    right: Math.min(width, Math.ceil((right + 2) / scale)),
    bottom: Math.min(height, Math.ceil((bottom + 2) / scale)),
  };
}

async function trim(img: HTMLImageElement): Promise<string | null> {
  const box = visibleBounds(img);
  if (!box) return null;
  const width = box.right - box.left;
  const height = box.bottom - box.top;
  // Less than 2% padding overall is not worth a second image.
  if (width * height >= img.naturalWidth * img.naturalHeight * 0.98) return null;
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) return null;
  ctx.drawImage(img, box.left, box.top, width, height, 0, 0, width, height);
  // Browsers that cannot encode WebP fall back to PNG.
  const blob = await new Promise<Blob | null>((resolve) =>
    canvas.toBlob(resolve, "image/webp", 0.92),
  );
  return blob ? URL.createObjectURL(blob) : null;
}

export type TrimmedLogo = {
  /** The image to draw: the trimmed logo once known, else the original. */
  src: string;
  /**
   * False until the trimmed logo is known: draw the image invisibly until
   * then, so the padded original never flashes before the trimmed one.
   */
  ready: boolean;
  /** Pass to the image's onLoad. */
  onLoad: (event: SyntheticEvent<HTMLImageElement>) => void;
};

/** A title logo with its transparent padding trimmed. */
export function useTrimmedLogo(url: string): TrimmedLogo {
  // Bumped when a trim this component started finishes, to read the cache again.
  const [, setTrimmedVersion] = useState(0);
  // The last image that finished loading, so the logo stays invisible until
  // the image it now shows has loaded.
  const [loadedSrc, setLoadedSrc] = useState<string | null>(null);
  const known = trimmed.has(url);
  const src = (known ? trimmed.get(url) : null) ?? url;

  const onLoad = useCallback(
    (event: SyntheticEvent<HTMLImageElement>) => {
      const img = event.currentTarget;
      const loaded = img.getAttribute("src");
      setLoadedSrc(loaded);
      if (loaded !== url || trimmed.has(url)) return;
      let pending: Promise<string | null>;
      try {
        pending = trim(img);
      } catch {
        pending = Promise.resolve(null);
      }
      void pending
        .catch(() => null)
        .then((result) => {
          if (!trimmed.has(url)) remember(url, result);
          setTrimmedVersion((version) => version + 1);
        });
    },
    [url],
  );

  return { src, ready: known && loadedSrc === src, onLoad };
}
