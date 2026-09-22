import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/cn";

type Variant = "primary" | "secondary" | "ghost" | "danger";
type Size = "md" | "sm";

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  size?: Size;
  loading?: boolean;
  block?: boolean;
  children: ReactNode;
};

const base =
  "inline-flex items-center justify-center gap-2 rounded-r2 border font-medium leading-none select-none " +
  "transition-colors duration-[var(--dur-1)] ease-[var(--ease-out)] disabled:opacity-60 disabled:cursor-not-allowed";

const variants: Record<Variant, string> = {
  primary: "bg-green text-paper border-green hover:bg-green-2 hover:border-green-2",
  secondary: "bg-paper-2 text-ink border-ink hover:bg-paper-3",
  ghost: "bg-transparent text-ink border-transparent hover:bg-paper-3",
  danger: "bg-paper-2 text-red border-red hover:bg-bad-bg",
};

const sizes: Record<Size, string> = {
  md: "min-h-[var(--touch)] px-4 text-base",
  sm: "min-h-9 px-3 text-sm",
};

/** Always labelled with a verb. Loading displays an accessible spinning loader and disables interaction. */
export function Button({ variant = "primary", size = "md", loading = false, block = false, className, children, disabled, ...rest }: ButtonProps) {
  return (
    <button
      type={rest.type ?? "button"}
      className={cn(base, variants[variant], sizes[size], block && "w-full", className)}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...rest}
    >
      {loading ? (
        <span
          aria-hidden
          className="inline-block size-4 animate-spin rounded-full border-2 border-current border-t-transparent"
        />
      ) : (
        children
      )}
    </button>
  );
}
