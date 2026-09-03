/** Tiny class joiner; avoids a dependency for the few conditional classes we use. */
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(" ");
}
