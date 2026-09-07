"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { Field, SelectField } from "@/components/ui/Field";
import { Leader } from "@/components/ui/Leader";
import { Money } from "@/components/ui/Money";
import { Sheet } from "@/components/ui/Sheet";
import { useToast } from "@/components/ui/Toast";
import { useConvertPayment, useItems } from "@/lib/api/queries";
import { ApiRequestError, type Schemas } from "@/lib/api/client";

/** Unmatched payment → sale + queued invoice. One item, qty 1, price = amount paid. */
export function ConvertPaymentSheet({ payment, onClose }: { payment: Schemas["Payment"] | null; onClose: () => void }) {
  const t = useTranslations("payments");
  const tc = useTranslations("common");
  const toast = useToast();
  const { data: items } = useItems();
  const convert = useConvertPayment();
  const [itemId, setItemId] = useState("");
  const [buyerPin, setBuyerPin] = useState("");

  async function submit() {
    if (!payment || !itemId) return;
    try {
      await convert.mutateAsync({
        id: payment.id,
        body: {
          lines: [{ item_id: itemId, qty: "1", unit_price_cents: payment.amount_cents }],
          ...(buyerPin ? { buyer_pin: buyerPin.toUpperCase() } : {}),
        },
      });
      toast.push(t("converted"));
      setItemId("");
      setBuyerPin("");
      onClose();
    } catch (e) {
      toast.push(e instanceof ApiRequestError ? tc("errorGeneric", { message: e.message }) : tc("noConnection"), "error");
    }
  }

  return (
    <Sheet
      open={payment !== null}
      onClose={onClose}
      title={t("convertTitle")}
      closeLabel={tc("close")}
      footer={
        <Button block disabled={!itemId} loading={convert.isPending} onClick={submit}>
          {t("convertSubmit")}
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
          <div className="mt-6 space-y-4">
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
            <Field label={t("buyerPin")} hint={t("buyerPinHint")} mono placeholder="A123456789B" value={buyerPin} onChange={(e) => setBuyerPin(e.target.value)} />
          </div>
        </>
      )}
    </Sheet>
  );
}
