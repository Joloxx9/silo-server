import { useEffect, useState } from "react";

/**
 * Draft-and-commit state for a typed percentage input (min-100): free typing
 * while focused, clamped and committed on blur/Enter.
 *
 * An emptied field reverts to the current value rather than committing
 * `min`: `Number("")` is `0`, a valid-looking parse that isn't a value the
 * user actually typed, and silently saving `min` on a stray clear is how a
 * subtitle opacity field would jump to nearly invisible with no
 * confirmation.
 */
export function usePercentDraft(value: number, min: number, onChange: (v: number) => void) {
  const [draft, setDraft] = useState(String(value));

  useEffect(() => {
    setDraft(String(value));
  }, [value]);

  function commit(raw: string) {
    const trimmed = raw.trim();
    const parsed = trimmed === "" ? NaN : Math.round(Number(trimmed));
    if (Number.isFinite(parsed)) {
      const clamped = Math.min(100, Math.max(min, parsed));
      // Set the draft even when the clamped value equals the current value:
      // typing "999" at 100 must snap the field back to "100", but if the
      // value prop doesn't change, the effect above never re-fires to do it.
      setDraft(String(clamped));
      onChange(clamped);
    } else {
      setDraft(String(value));
    }
  }

  return { draft, setDraft, commit };
}
