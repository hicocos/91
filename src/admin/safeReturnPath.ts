export function safeReturnPath(value: unknown, fallback: string): string {
  if (
    typeof value === "string" &&
    value.startsWith("/") &&
    !value.startsWith("//") &&
    !value.includes("\\")
  ) {
    return value;
  }
  return fallback;
}
