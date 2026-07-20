/** Formats a lat/lon pair with correct hemisphere suffixes (N/S, E/W) derived from sign. */
export function formatCoord(latitude: number, longitude: number): string {
  const latSuffix = latitude < 0 ? 'S' : 'N';
  const lonSuffix = longitude < 0 ? 'W' : 'E';
  return `${Math.abs(latitude).toFixed(4)}°${latSuffix}, ${Math.abs(longitude).toFixed(4)}°${lonSuffix}`;
}
