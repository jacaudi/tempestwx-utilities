/** Placeholder rendered for a missing/invalid numeric field instead of throwing. */
const MISSING_VALUE = '—';

/**
 * Formats a possibly-missing numeric field. Returns an em-dash when `value` is
 * `undefined`, `null`, or `NaN`; otherwise applies `fmt`. Prevents crashes from
 * calling number methods (e.g. `.toFixed`) on a field the station/API omitted.
 */
export function formatX(
  value: number | undefined | null,
  fmt: (n: number) => string
): string {
  if (value === undefined || value === null || Number.isNaN(value)) {
    return MISSING_VALUE;
  }
  return fmt(value);
}
