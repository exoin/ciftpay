"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { X } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Field, SelectField } from "@/components/ui/Field";
import { Leader } from "@/components/ui/Leader";
import { Money } from "@/components/ui/Money";
import { Sheet } from "@/components/ui/Sheet";
import { useToast } from "@/components/ui/Toast";
import { useCreateSale, useItems } from "@/lib/api/queries";
import { ApiRequestError } from "@/lib/api/client";
import { normaliseMsisdn } from "@/lib/format";

type LineDraft = { item_id: string; qty: string; unit_price: string };

/** "Record a sale": cash sale for a payment CiftPay didn't see. Files with KRA immediately. */
export function RecordSaleSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const t = useTranslations("sale");
  const tp = useTranslations("payments");
  const tc = useTranslations("common");
  const ti = useTranslations("items");
  const toast = useToast();
  const { data: items } = useItems();
  const create = useCreateSale();
  const [lines, setLines] = useState<LineDraft[]>([{ item_id: "", qty: "1", unit_price: "" }]);
  const [buyerPhone, setBuyerPhone] = useState("");
  const [buyerName, setBuyerName] = useState("");
  const [buyerPin, setBuyerPin] = useState("");
  const [phoneError, setPhoneError] = useState<string | null>(null);

  const totalCents = useMemo(() => {
    return lines.reduce((sum, l) => {
      const item = items?.data.find((i) => i.id === l.item_id);
      const price = l.unit_price ? Math.round(Number(l.unit_price) * 100) : (item?.price_cents ?? 0);
      const qty = Number(l.qty) || 0;
      return sum + Math.round(price * qty);
    }, 0);
  }, [lines, items]);

  const canSubmit =
    lines.length > 0 &&
    lines.every((l) => {
      if (!l.item_id || Number(l.qty) <= 0) return false;
      const item = items?.data.find((i) => i.id === l.item_id);
      const price = l.unit_price ? Number(l.unit_price) : (item?.price_cents ?? 0) / 100;
      return price > 0;
    }) &&
    !create.isPending;

  function handleClose() {
    setPhoneError(null);
    onClose();
  }

  async function submit() {
    const trimmedPhone = buyerPhone.trim();
    let normPhone: string | undefined = undefined;
    if (trimmedPhone) {
      const n = normaliseMsisdn(trimmedPhone);
      if (!n) {
        setPhoneError(t("invalidPhone"));
        return;
      }
      normPhone = n;
    }

    try {
      await create.mutateAsync({
        kind: "cash",
        lines: lines.map((l) => {
          const item = items?.data.find((i) => i.id === l.item_id);
          const priceCents = l.unit_price ? Math.round(Number(l.unit_price) * 100) : (item?.price_cents ?? 0);
          return {
            item_id: l.item_id,
            // Quantity is a decimal string on the wire (OpenAPI `Quantity`).
            qty: l.qty.trim() || "1",
            unit_price_cents: priceCents,
          };
        }),
        ...(normPhone ? { buyer_msisdn: normPhone } : {}),
        ...(buyerName.trim() ? { buyer_name: buyerName.trim() } : {}),
        ...(buyerPin.trim() ? { buyer_pin: buyerPin.trim().toUpperCase() } : {}),
        client_ref: crypto.randomUUID(),
      });
      toast.push(t("queued"));
      setLines([{ item_id: "", qty: "1", unit_price: "" }]);
      setBuyerPhone("");
      setBuyerName("");
      setBuyerPin("");
      setPhoneError(null);
      onClose();
    } catch (e) {
      toast.push(e instanceof ApiRequestError ? tc("errorGeneric", { message: e.message }) : tc("noConnection"), "error");
    }
  }

  function handleItemChange(index: number, selectedId: string) {
    const item = items?.data.find((it) => it.id === selectedId);
    setLines((cur) =>
      cur.map((x, j) => {
        if (j !== index) return x;
        const priceCents = item?.price_cents ?? 0;
        const defaultPrice = priceCents > 0 ? (priceCents / 100).toString() : "";
        return { ...x, item_id: selectedId, unit_price: defaultPrice };
      })
    );
  }

  function removeLine(index: number) {
    setLines((cur) => cur.filter((_, j) => j !== index));
  }

  return (
    <Sheet
      open={open}
      onClose={handleClose}
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
          <fieldset key={i} className="grid grid-cols-[1fr_72px_100px_auto] items-end gap-2 border-b border-dashed border-hairline pb-4 sm:gap-3">
            <SelectField
              label={tp("item")}
              value={l.item_id}
              onChange={(e) => handleItemChange(i, e.target.value)}
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
            <Field
              label={ti("price")}
              inputMode="decimal"
              mono
              placeholder="0"
              value={l.unit_price}
              onChange={(e) => setLines((cur) => cur.map((x, j) => (j === i ? { ...x, unit_price: e.target.value } : x)))}
            />
            {lines.length > 1 ? (
              <button
                type="button"
                className="mb-1 rounded p-1.5 text-ink-3 hover:text-ink transition-colors"
                onClick={() => removeLine(i)}
                aria-label={tc("close")}
                title={tc("close")}
              >
                <X className="size-4" aria-hidden="true" />
              </button>
            ) : (
              <div className="w-0" />
            )}
          </fieldset>
        ))}
        <Button variant="secondary" size="sm" onClick={() => setLines((cur) => [...cur, { item_id: "", qty: "1", unit_price: "" }])}>
          {t("addLine")}
        </Button>

        <div className="pt-2 border-t border-hairline space-y-3">
          <div className="text-xs font-semibold uppercase tracking-wider text-ink-3">
            {t("customerSection")}
          </div>

          <Field
            label={t("buyerPhone")}
            hint={t("buyerPhoneHint")}
            error={phoneError ?? undefined}
            type="tel"
            inputMode="tel"
            autoComplete="tel"
            placeholder="0712 345 678"
            mono
            value={buyerPhone}
            onChange={(e) => {
              setBuyerPhone(e.target.value);
              if (phoneError) setPhoneError(null);
            }}
          />

          <Field
            label={t("buyerName")}
            placeholder="Jane Doe"
            value={buyerName}
            onChange={(e) => setBuyerName(e.target.value)}
          />

          <Field
            label={tp("buyerPin")}
            hint={tp("buyerPinHint")}
            mono
            placeholder="A123456789B"
            value={buyerPin}
            onChange={(e) => setBuyerPin(e.target.value)}
          />
        </div>
      </div>
    </Sheet>
  );
}
