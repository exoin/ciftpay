"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { Field, SelectField } from "@/components/ui/Field";
import { Leader } from "@/components/ui/Leader";
import { Money } from "@/components/ui/Money";
import { Sheet } from "@/components/ui/Sheet";
import { useToast } from "@/components/ui/Toast";
import { useCreateSale, useItems } from "@/lib/api/queries";
import { ApiRequestError } from "@/lib/api/client";

type LineDraft = { item_id: string; qty: string; unit_price: string };

/** "Record a sale": cash sale for a payment CiftPay didn't see. Files with KRA immediately. */
export function RecordSaleSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useTranslations("sale");
  const tp = useTranslations("payments");
  const tc = useTranslations("common");
  const toast = useToast();
  const { data: items } = useItems();
  const create = useCreateSale();
  const [lines, setLines] = useState<LineDraft[]>([{ item_id: "", qty: "1", unit_price: "" }]);
  const [buyerPin, setBuyerPin] = useState("");

  const totalCents = useMemo(() => {
    return lines.reduce((sum, l) => {
      const item = items?.data.find((i) => i.id === l.item_id);
      const price = l.unit_price ? Math.round(Number(l.unit_price) * 100) : (item?.price_cents ?? 0);
      const qty = Number(l.qty) || 0;
      return sum + Math.round(price * qty);
    }, 0);
  }, [lines, items]);

  const canSubmit = lines.every((l) => l.item_id && Number(l.qty) > 0) && !create.isPending;

  async function submit() {
    try {
      await create.mutateAsync({
        kind: "cash",
        lines: lines.map((l) => ({
          item_id: l.item_id,
          // Quantity is a decimal string on the wire (OpenAPI `Quantity`).
          qty: l.qty.trim(),
          ...(l.unit_price ? { unit_price_cents: Math.round(Number(l.unit_price) * 100) } : {}),
        })),
        ...(buyerPin ? { buyer_pin: buyerPin.toUpperCase() } : {}),
        client_ref: crypto.randomUUID(),
      });
      toast.push(t("queued"));
      setLines([{ item_id: "", qty: "1", unit_price: "" }]);
      setBuyerPin("");
      onClose();
    } catch (e) {
      toast.push(e instanceof ApiRequestError ? tc("errorGeneric", { message: e.message }) : tc("noConnection"), "error");
    }
  }

  return (
    <Sheet
      open={open}
      onClose={onClose}
      title={t("title")}
      closeLabel={tc("close")}
      footer={
        <div className="space-y-3">
          <Leader strong label={t("total")} amount={<Money cents={totalCents} size="lg" />} />
          <Button block disabled={!canSubmit} loading={create.isPending} onClick={submit}>
            {t("submit")}
          </Button>
        </div>
      }
    >
      <p className="text-sm text-ink-2">{t("lead")}</p>
      <div className="mt-4 space-y-5">
        {lines.map((l, i) => (
          <fieldset key={i} className="grid grid-cols-[1fr_88px] gap-3 border-b border-dashed border-hairline pb-4">
            <SelectField
              label={tp("item")}
              value={l.item_id}
              onChange={(e) => setLines((cur) => cur.map((x, j) => (j === i ? { ...x, item_id: e.target.value } : x)))}
            >
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
              label={tp("qty")}
              inputMode="decimal"
              mono
              value={l.qty}
              onChange={(e) => setLines((cur) => cur.map((x, j) => (j === i ? { ...x, qty: e.target.value } : x)))}
            />
          </fieldset>
        ))}
        <Button variant="secondary" size="sm" onClick={() => setLines((cur) => [...cur, { item_id: "", qty: "1", unit_price: "" }])}>
          {t("addLine")}
        </Button>
        <Field label={tp("buyerPin")} hint={tp("buyerPinHint")} mono placeholder="A123456789B" value={buyerPin} onChange={(e) => setBuyerPin(e.target.value)} />
      </div>
    </Sheet>
  );
}
