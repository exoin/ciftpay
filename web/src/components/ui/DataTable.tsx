import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export type Column<Row> = {
  key: string;
  header: ReactNode;
  cell: (row: Row) => ReactNode;
  numeric?: boolean;
  /** Hide on the mobile list layout (primary/secondary/trailing carry the row). */
  desktopOnly?: boolean;
};

export type DataTableProps<Row> = {
  rows: Row[];
  columns: Column<Row>[];
  rowKey: (row: Row) => string;
  /** Mobile list rendering. */
  primary: (row: Row) => ReactNode;
  secondary?: (row: Row) => ReactNode;
  trailing?: (row: Row) => ReactNode;
  onRowClick?: (row: Row) => void;
  caption?: string;
};

/**
 * Hairline rows, sticky header, numeric columns right-aligned mono. Under
 * 768 px it collapses to a ruled list with primary / secondary text and a
 * trailing amount.
 */
export function DataTable<Row>({ rows, columns, rowKey, primary, secondary, trailing, onRowClick, caption }: DataTableProps<Row>) {
  const clickable = Boolean(onRowClick);
  return (
    <>
      <ul className="ruled md:hidden">
        {rows.map((r) => (
          <li key={rowKey(r)}>
            <RowShell clickable={clickable} onClick={onRowClick ? () => onRowClick(r) : undefined}>
              <div className="min-w-0 flex-1">
                <div className="truncate">{primary(r)}</div>
                {secondary && <div className="truncate text-sm text-muted">{secondary(r)}</div>}
              </div>
              {trailing && <div className="shrink-0 text-right">{trailing(r)}</div>}
            </RowShell>
          </li>
        ))}
      </ul>
      <table className="hidden md:table w-full border-collapse text-sm">
        {caption && <caption className="sr-only">{caption}</caption>}
        <thead className="sticky top-0 bg-paper">
          <tr className="border-b border-ink text-left text-xs uppercase tracking-wider text-muted">
            {columns.map((c) => (
              <th key={c.key} scope="col" className={cn("py-2 pr-4 font-medium", c.numeric && "text-right font-mono")}>
                {c.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr
              key={rowKey(r)}
              onClick={onRowClick ? () => onRowClick(r) : undefined}
              className={cn("border-b border-hairline", clickable && "cursor-pointer hover:bg-paper-3")}
            >
              {columns.map((c) => (
                <td key={c.key} className={cn("py-3 pr-4 align-top", c.numeric && "text-right font-mono")}>
                  {c.cell(r)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

function RowShell({ clickable, onClick, children }: { clickable: boolean; onClick?: () => void; children: ReactNode }) {
  const cls = "flex min-h-[var(--row)] w-full items-center gap-3 py-2 text-left";
  if (clickable) {
    return (
      <button type="button" onClick={onClick} className={cn(cls, "hover:bg-paper-3 active:bg-paper-3 -mx-4 px-4 w-[calc(100%+2rem)]")}>
        {children}
      </button>
    );
  }
  return <div className={cls}>{children}</div>;
}
