// formatBytes formats storage quantities using explicit decimal or binary units.
export function formatBytes(value: number, base: 1000 | 1024 = 1000): string {
  if (!Number.isFinite(value) || value < 0) return "—";
  if (value < base) return `${Math.round(value)} B`;
  const units =
    base === 1000
      ? ["B", "kB", "MB", "GB", "TB"]
      : ["B", "KiB", "MiB", "GiB", "TiB"];
  const unit = Math.min(
    Math.floor(Math.log(value) / Math.log(base)),
    units.length - 1,
  );
  return `${(value / base ** unit).toFixed(1)} ${units[unit]}`;
}

// formatBinaryBytes displays file sizes and transfer rates in binary units.
export function formatBinaryBytes(value: number): string {
  return formatBytes(value, 1024);
}

// localDateTime converts an API timestamp to a datetime-local input without shifting its instant.
export function localDateTime(timestamp: string): string {
  const date = new Date(timestamp);
  if (!Number.isFinite(date.getTime())) return "";
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}
