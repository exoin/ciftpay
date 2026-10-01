"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { Field, SelectField } from "@/components/ui/Field";
import { Leader } from "@/components/ui/Leader";
import { Money } from "@/components/ui/Money";
import { Sheet } from "@/components/ui/Sheet";
import { useToast } from "@/components/ui/Toast";
import { useConvertPayment, useItems, useOpenSales } from "@/lib/api/queries";
import { ApiRequestError, type Schemas } from "@/lib/api/client";

/** Unmatched payment → sale + queued invoice or linked to an existing counter sale */
export function ConvertPaymentSheet({ payment, onClose }: { payment: Schemas["Payment"] | null; onClose: () => void }) {
  const t = useTranslations("payments");
  const tc = useTranslations("common");
  const toast = useToast();
  const { data: items } = useItems();
  const { data: openSales } = useOpenSales();
  const convert = useConvertPayment();

  const [mode, setMode] = useState<"items" | "order">("items");
  const [selectedSaleId, setSelectedSaleId] = useState("");
  const [itemId, setItemId] = useState("");
  const [buyerPin, setBuyerPin] = useState("");

  const hasOpenOrders = (openSales?.data?.length ?? 0) > 0;

  async function submit() {
    if (!payment) return;
    try {
      if (mode === "order") {
        if (!selectedSaleId) return;
        await convert.mutateAsync({
          id: payment.id,
          body: {
            sale_id: selectedSaleId,
          },
        });
      } else {
        if (!itemId) return;
        await convert.mutateAsync({
          id: payment.id,
          body: {
            lines: [{ item_id: itemId, qty: "1", unit_price_cents: payment.amount_cents }],
            ...(buyerPin ? { buyer_pin: buyerPin.toUpperCase() } : {}),
          },
        });
      }
      toast.push(t("converted"));
      setItemId("");
      setBuyerPin("");
      setSelectedSaleId("");
      onClose();
    } catch (e) {
      toast.push(e instanceof ApiRequestError ? tc("errorGeneric", { message: e.message }) : tc("noConnection"), "error");
    }
  }

  const isSubmitDisabled = mode === "order" ? !selectedSaleId : !itemId;

  return (
    <Sheet
      open={payment !== null}
      onClose={onClose}
      title={t("convertTitle")}
      closeLabel={tc("close")}
      footer={
        <Button block disabled={isSubmitDisabled} loading={convert.isPending} onClick={submit}>
          {mode === "order" ? t("linkOrderSubmit") : t("convertSubmit")}
        </Button>
      }
    >
      {payment && (
        <>
          <p className="text-sm text-ink-2">{t("convertLead")}</p>
          <div className="mt-4 space-y-1 font-mono text-sm">
            <Leader label={t("from", { msisdn: payment.payer_msisdn_masked })} amount={<Money cents={payment.amount_cents} />} />
            {payment.bill_ref && <div className="text-muted">{t("ref", { ref: payment.bill_ref })}</div>}
          </div>

          {/* Mode Switcher */}
          <div className="mt-5 grid grid-cols-2 gap-2 rounded-lg bg-surface-2 p-1 text-xs font-medium">
            <button
              type="button"
              className={`rounded-md py-1.5 transition-colors ${
                mode === "items" ? "bg-surface text-ink shadow-sm font-semibold" : "text-ink-2 hover:text-ink"
              }`}
              onClick={() => setMode("items")}
            >
              {t("tabItems")}
            </button>
            <button
              type="button"
              className={`rounded-md py-1.5 transition-colors ${
                mode === "order" ? "bg-surface text-ink shadow-sm font-semibold" : "text-ink-2 hover:text-ink"
              }`}
              onClick={() => setMode("order")}
            >
              {t("tabLinkOrder")}
              {hasOpenOrders && (
                <span className="ml-1.5 rounded-full bg-accent/20 px-1.5 py-0.5 text-[10px] text-accent font-bold">
                  {openSales?.data?.length}
                </span>
              )}
            </button>
          </div>

          {mode === "order" ? (
            <div className="mt-5 space-y-3">
              <p className="text-xs text-ink-2 font-medium">{t("selectOrderPrompt")}</p>
              {!hasOpenOrders ? (
                <div className="rounded-lg border border-dashed border-line p-4 text-center text-xs text-muted">
                  {t("openOrdersEmpty")}
                </div>
              ) : (
                <div className="max-h-60 space-y-2 overflow-y-auto">
                  {openSales?.data?.map((s) => {
                    const isSelected = selectedSaleId === s.id;
                    const diff = payment.amount_cents - s.total_cents;
                    return (
                      <div
                        key={s.id}
                        role="button"
                        tabIndex={0}
                        onClick={() => setSelectedSaleId(s.id)}
                        onKeyDown={(e) => e.key === "Enter" && setSelectedSaleId(s.id)}
                        className={`cursor-pointer rounded-lg border p-3 text-left transition-all ${
                          isSelected ? "border-accent bg-accent/5 ring-1 ring-accent" : "border-line bg-surface hover:border-ink-2/30"
                        }`}
                      >
                        <div className="flex items-center justify-between text-xs font-semibold">
                          <span className="font-mono text-ink">{s.ref}</span>
                          <span className="font-mono">
                            <Money cents={s.total_cents} />
                          </span>
                        </div>
                        {s.buyer_name && <div className="mt-1 text-xs text-ink-2 truncate">{s.buyer_name}</div>}
                        <div className="mt-1 flex items-center justify-between text-[11px] text-muted">
                          <span>{new Date(s.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</span>
                          {diff === 0 ? (
                            <span className="text-accent font-medium">{tc("exactMatch")}</span>
                          ) : (
                            <span className="font-mono">
                              {diff > 0 ? "+" : ""}
                              <Money cents={diff} />
                            </span>
                          )}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          ) : (
            <div className="mt-5 space-y-4">
              <SelectField label={t("item")} value={itemId} onChange={(e) => setItemId(e.target.value)}>
                <option value="" />
                {items?.data
                  .filter((it) => it.is_active)
                  .map((it) => (
                    <option key={it.id} value={it.id}>
                      {it.name}
                    </option>
                  ))}
              </SelectField>
              <Field
                label={t("buyerPin")}
                hint={t("buyerPinHint")}
                mono
                placeholder="A123456789B"
                value={buyerPin}
                onChange={(e) => setBuyerPin(e.target.value)}
              />
            </div>
          )}
        </>
      )}
    </Sheet>
  );
}
