import { forwardRef, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, useId } from "react";
import { cn } from "@/lib/cn";

type Common = {
  label: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
};

const control =
  "block w-full min-h-[var(--touch)] rounded-r2 border border-hairline bg-paper-2 px-3 text-base text-ink placeholder:text-muted " +
  "focus:border-ink focus:outline-none focus-visible:outline-2 focus-visible:outline-ochre focus-visible:outline-offset-2 " +
  "aria-[invalid=true]:border-red";

export type FieldProps = Common & InputHTMLAttributes<HTMLInputElement> & { mono?: boolean };

/** Label above, 44 px input, hairline border, ochre focus ring, error below in kra-red. */
export const Field = forwardRef<HTMLInputElement, FieldProps>(function Field({ label, hint, error, mono, className, id, ...rest }, ref) {
  const auto = useId();
  const inputId = id ?? auto;
  const hintId = `${inputId}-hint`;
  const errId = `${inputId}-err`;
  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <label htmlFor={inputId} className="text-sm font-medium text-ink-2">
        {label}
      </label>
      <input
        ref={ref}
        id={inputId}
        aria-invalid={error ? true : undefined}
        aria-describedby={cn(hint ? hintId : null, error ? errId : null) || undefined}
        className={cn(control, mono && "font-mono")}
        {...rest}
      />
      {hint && !error && (
        <p id={hintId} className="text-xs text-muted">
          {hint}
        </p>
      )}
      {error && (
        <p id={errId} className="text-xs text-red" role="alert">
          {error}
        </p>
      )}
    </div>
  );
});

export type SelectFieldProps = Common & SelectHTMLAttributes<HTMLSelectElement> & { children: ReactNode };

export const SelectField = forwardRef<HTMLSelectElement, SelectFieldProps>(function SelectField({ label, hint, error, className, id, children, ...rest }, ref) {
  const auto = useId();
  const inputId = id ?? auto;
  const hintId = `${inputId}-hint`;
  const errId = `${inputId}-err`;
  return (
    <div className={cn("flex flex-col gap-1.5", className)}>
      <label htmlFor={inputId} className="text-sm font-medium text-ink-2">
        {label}
      </label>
      <select ref={ref} id={inputId} aria-invalid={error ? true : undefined} aria-describedby={cn(hint ? hintId : null, error ? errId : null) || undefined} className={control} {...rest}>
        {children}
      </select>
      {hint && !error && (
        <p id={hintId} className="text-xs text-muted">
          {hint}
        </p>
      )}
      {error && (
        <p id={errId} className="text-xs text-red" role="alert">
          {error}
        </p>
      )}
    </div>
  );
});
