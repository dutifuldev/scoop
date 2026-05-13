const DAY_REGEX = /^\d{4}-\d{2}-\d{2}$/;

export function parseDayString(value: string): Date | undefined {
  if (!DAY_REGEX.test(value)) {
    return undefined;
  }

  const [yearText, monthText, dayText] = value.split("-");
  const year = Number(yearText);
  const month = Number(monthText);
  const day = Number(dayText);
  if (!Number.isFinite(year) || !Number.isFinite(month) || !Number.isFinite(day)) {
    return undefined;
  }

  const parsed = new Date(year, month - 1, day);
  if (Number.isNaN(parsed.getTime())) {
    return undefined;
  }

  if (
    parsed.getFullYear() !== year ||
    parsed.getMonth() !== month - 1 ||
    parsed.getDate() !== day
  ) {
    return undefined;
  }

  return parsed;
}

export function toDayString(value: Date): string {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function addCalendarDays(day: string, offset: number): string {
  const parsed = parseDayString(day);
  if (!parsed || !Number.isFinite(offset)) {
    return "";
  }

  const next = new Date(parsed);
  next.setDate(next.getDate() + offset);
  return toDayString(next);
}
