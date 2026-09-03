"use client";

import { useEffect, useRef, type ReactNode } from "react";
import { X } from "lucide-react";
import { cn } from "@/lib/cn";

export type SheetProps = {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  closeLabel: string;
  children: ReactNode;
  footer?: ReactNode;
};

/**
 * Bottom sheet on mobile, right drawer on desktop. The only component allowed
 * a shadow. Native <dialog> gives us focus trap, Esc and inert background.
 */
export function Sheet({ open, onClose, title, closeLabel, children, footer }: SheetProps) {
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (open && !el.open) el.showModal();
    if (!open && el.open) el.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      className={cn(
        "sheet-enter m-0 max-h-[92dvh] w-full max-w-none overflow-hidden bg-paper-2 text-ink p-0",
        "fixed inset-x-0 bottom-0 top-auto rounded-t-r3 border-t border-hairline",
        "md:inset-y-0 md:left-auto md:right-0 md:h-dvh md:max-h-none md:w-[440px] md:rounded-none md:border-l md:border-t-0",
        "backdrop:bg-ink/40 [box-shadow:var(--shadow-sheet)]",
      )}
    >
      <div className="flex h-full max-h-[92dvh] md:max-h-dvh flex-col">
        <header className="flex items-center justify-between gap-4 border-b border-hairline px-4 py-3">
          <h2 className="text-lg">{title}</h2>
          <button type="button" onClick={onClose} aria-label={closeLabel} className="grid size-[var(--touch)] place-items-center rounded-r2 hover:bg-paper-3">
            <X size={20} strokeWidth={1.5} aria-hidden />
          </button>
        </header>
        <div className="flex-1 overflow-y-auto px-4 py-4">{children}</div>
        {footer && <footer className="border-t border-hairline px-4 py-3">{footer}</footer>}
      </div>
    </dialog>
  );
}
