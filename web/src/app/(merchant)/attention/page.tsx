"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import { TopBar } from "@/components/shell/TopBar";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { LoadingRows } from "@/components/ui/LoadingRows";
import { Money } from "@/components/ui/Money";
import { useToast } from "@/components/ui/Toast";
import { useAttention, useVerifyShortcode } from "@/lib/api/queries";
import { ApiRequestError, type Schemas } from "@/lib/api/client";
import { formatKES } from "@/lib/format";
import { ConvertPaymentSheet } from "@/components/sale/ConvertPaymentSheet";

/** Failed submissions, unmatched payments, unverified shortcodes — each with one fix action. */
export default function AttentionPage() {
  const t = useTranslations("attention");
  const tc = useTranslations("common");
  const toast = useToast();
  const { data, isPending } = useAttention();
  const verify = useVerifyShortcode();
  const [selected, setSelected] = useState<Schemas["Payment"] | null>(null);

  if (isPending) {
    return (
      <>
        <TopBar title={t("title")} />
        <LoadingRows rows={6} label={tc("loading")} />
      </>
    );
  }

  const empty = !data || data.actionable_count === 0;

  return (
    <>
      <TopBar title={t("title")} />
      {empty ? (
        <EmptyState>{t("empty")}</EmptyState>
      ) : (
        <div className="space-y-8">
          <Group title={t("failed")} items={data.failed_invoices}>
            {(inv) => (
              <Row
                lead={t("failedLead", { reason: inv.last_error ?? "—" })}
                trailing={<Money cents={inv.total_cents} />}
                action={
                  <Link href={`/invoices/${inv.id}`}>
                    <Button size="sm" variant="danger">
                      {t("fix")}
                    </Button>
                  </Link>
                }
              />
            )}
          </Group>
          <Group title={t("unmatched")} items={data.unmatched_payments}>
            {(p) => (
              <Row
                lead={t("unmatchedLead", { amount: formatKES(p.amount_cents), msisdn: p.payer_msisdn_masked })}
                trailing={<Money cents={p.amount_cents} />}
                action={
                  <Button size="sm" onClick={() => setSelected(p)}>
                    {t("convert")}
                  </Button>
                }
              />
            )}
          </Group>
          <Group title={t("unverified")} items={data.unverified_shortcodes}>
            {(sc) => (
              <Row
                lead={t("unverifiedLead", { shortcode: sc.shortcode })}
                action={
                  <Button
                    size="sm"
                    variant="secondary"
                    loading={verify.isPending && verify.variables === sc.id}
                    onClick={async () => {
                      try {
                        await verify.mutateAsync(sc.id);
                      } catch (e) {
                        toast.push(e instanceof ApiRequestError ? tc("errorGeneric", { message: e.message }) : tc("noConnection"), "error");
                      }
                    }}
                  >
                    {t("verify")}
                  </Button>
                }
              />
            )}
          </Group>
          <Group title={t("pendingLong")} items={data.pending_long}>
            {(inv) => (
              <Row
                lead={t("pendingLongLead")}
                trailing={<Money cents={inv.total_cents} />}
                action={
                  <Link href={`/invoices/${inv.id}`} className="text-sm underline">
                    {inv.receipt_code}
                  </Link>
                }
              />
            )}
          </Group>
        </div>
      )}
      <ConvertPaymentSheet payment={selected} onClose={() => setSelected(null)} />
    </>
  );
}

function Group<T extends { id: string }>({ title, items, children }: { title: string; items: T[]; children: (item: T) => ReactNode }) {
  if (items.length === 0) return null;
  return (
    <section>
      <h2 className="flex items-baseline gap-2">
        {title}
        <span className="font-mono text-sm text-muted">{items.length}</span>
      </h2>
      <ul className="ruled mt-2">
        {items.map((it) => (
          <li key={it.id}>{children(it)}</li>
        ))}
      </ul>
    </section>
  );
}

function Row({ lead, trailing, action }: { lead: string; trailing?: ReactNode; action: ReactNode }) {
  return (
    <div className="flex min-h-[var(--row)] flex-wrap items-center gap-3 py-3">
      <p className="min-w-0 flex-1 basis-60 text-sm">{lead}</p>
      {trailing}
      <div className="shrink-0">{action}</div>
    </div>
  );
}
