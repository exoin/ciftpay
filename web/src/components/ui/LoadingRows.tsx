/** Static dotted-leader rows while data loads. No shimmer (design-system §6). */
export function LoadingRows({ rows = 4, label = "Loading" }: { rows?: number; label?: string }) {
  return (
    <div className="loading-rows" role="status" aria-live="polite" aria-label={label}>
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} />
      ))}
    </div>
  );
}
