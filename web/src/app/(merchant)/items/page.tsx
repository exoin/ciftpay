"use client";

import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslations, useLocale } from "next-intl";
import { TopBar } from "@/components/shell/TopBar";
import { Button } from "@/components/ui/Button";
import { DataTable } from "@/components/ui/DataTable";
import { EmptyState } from "@/components/ui/EmptyState";
import { Field, SelectField } from "@/components/ui/Field";
import { LoadingRows } from "@/components/ui/LoadingRows";
import { Money } from "@/components/ui/Money";
import { Sheet } from "@/components/ui/Sheet";
import { StatusChip } from "@/components/ui/StatusChip";
import { useToast } from "@/components/ui/Toast";
import { useCreateItem, useItems, useCurrentOrg } from "@/lib/api/queries";
import { ApiRequestError } from "@/lib/api/client";
import {
  CATEGORY_PRESETS,
  ETIMS_CLASS_CODES,
  KRA_UNITS_OF_MEASURE,
} from "@/lib/etims-codes";

const schema = z.object({
  name: z.string().min(1),
  etims_class_code: z.string().min(4),
  tax_category: z.enum(["A", "B", "C", "D", "E"]),
  unit: z.string().min(1),
  price: z.string().optional(),
  category_preset: z.string().optional(),
  is_vat_applicable: z.boolean(),
});
type Form = z.infer<typeof schema>;

const CATS = ["A", "B", "C", "D", "E"] as const;

export default function ItemsPage() {
  const t = useTranslations("items");
  const tc = useTranslations("common");
  const locale = useLocale();
  const toast = useToast();
  const { data, isPending } = useItems();
  const { data: org } = useCurrentOrg();
  const create = useCreateItem();
  const [open, setOpen] = useState(false);
  const [codeSearch, setCodeSearch] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const orgVatRegistered = Boolean(org?.vat_registered);
  const defaultTaxCat = orgVatRegistered ? "B" : "D";

  const form = useForm<Form>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: "",
      etims_class_code: "50000000",
      tax_category: defaultTaxCat,
      unit: "PCS",
      price: "",
      category_preset: "",
      is_vat_applicable: orgVatRegistered,
    },
  });

  const rows = data?.data ?? [];

  function handleCategoryPresetChange(presetId: string) {
    form.setValue("category_preset", presetId);
    if (!presetId) return;

    const preset = CATEGORY_PRESETS.find((p) => p.id === presetId);
    if (preset) {
      form.setValue("etims_class_code", preset.code, { shouldValidate: true });
      if (preset.defaultUnit) {
        form.setValue("unit", preset.defaultUnit);
      }
      if (orgVatRegistered) {
        const targetTax = preset.defaultTaxCategory;
        form.setValue("tax_category", targetTax);
        form.setValue("is_vat_applicable", targetTax === "B");
      } else {
        form.setValue("tax_category", "D");
        form.setValue("is_vat_applicable", false);
      }
    }
  }

  function handleVatCheckboxChange(checked: boolean) {
    form.setValue("is_vat_applicable", checked);
    if (checked) {
      form.setValue("tax_category", "B");
    } else {
      form.setValue("tax_category", orgVatRegistered ? "A" : "D");
    }
  }

  function handleTaxCategoryChange(cat: "A" | "B" | "C" | "D" | "E") {
    form.setValue("tax_category", cat);
    form.setValue("is_vat_applicable", cat === "B");
  }

  const filteredCodes = ETIMS_CLASS_CODES.filter((c) => {
    if (!codeSearch.trim()) return true;
    const q = codeSearch.toLowerCase();
    return (
      c.code.includes(q) ||
      c.nameEn.toLowerCase().includes(q) ||
      c.nameSw.toLowerCase().includes(q) ||
      c.category.toLowerCase().includes(q)
    );
  });

  async function submit(v: Form) {
    setFormError(null);
    try {
      await create.mutateAsync({
        name: v.name,
        etims_class_code: v.etims_class_code,
        tax_category: v.tax_category,
        unit: v.unit,
        price_cents: v.price ? Math.round(Number(v.price) * 100) : null,
      });
      toast.push(t("saved"));
      form.reset({
        name: "",
        etims_class_code: "50000000",
        tax_category: orgVatRegistered ? "B" : "D",
        unit: "PCS",
        price: "",
        category_preset: "",
        is_vat_applicable: orgVatRegistered,
      });
      setCodeSearch("");
      setFormError(null);
      setOpen(false);
    } catch (e) {
      if (e instanceof ApiRequestError) {
        if (e.status >= 500) {
          toast.push(tc("errorGeneric", { message: e.message }), "error");
        } else {
          setFormError(e.message || "Failed to save item. Please verify the entered details.");
        }
      } else if (e instanceof Error) {
        setFormError(e.message);
      } else {
        toast.push(tc("noConnection"), "error");
      }
    }
  }

  return (
    <>
      <TopBar
        title={t("title")}
        action={
          <Button size="sm" onClick={() => setOpen(true)}>
            {t("add")}
          </Button>
        }
      />
      {isPending ? (
        <LoadingRows rows={5} label={tc("loading")} />
      ) : rows.length === 0 ? (
        <EmptyState action={<Button onClick={() => setOpen(true)}>{t("add")}</Button>}>
          {t("empty")}
        </EmptyState>
      ) : (
        <DataTable
          rows={rows}
          rowKey={(i) => i.id}
          primary={(i) => (
            <span className="flex items-center gap-2">
              {i.name}
              {!i.is_active && <StatusChip tone="neutral">{t("inactive")}</StatusChip>}
            </span>
          )}
          secondary={(i) => (
            <span className="font-mono">
              {i.etims_class_code} · {i.tax_category} · {i.unit}
            </span>
          )}
          trailing={(i) =>
            i.price_cents != null ? <Money cents={i.price_cents} /> : <span className="text-muted">—</span>
          }
          columns={[
            { key: "name", header: t("name"), cell: (i) => i.name },
            { key: "code", header: t("code"), cell: (i) => <span className="font-mono">{i.etims_class_code}</span> },
            { key: "cat", header: t("taxCategory"), cell: (i) => <span className="font-mono">{i.tax_category}</span> },
            { key: "unit", header: t("unit"), cell: (i) => <span className="font-mono">{i.unit}</span> },
            {
              key: "price",
              header: "KES",
              numeric: true,
              cell: (i) => (i.price_cents != null ? <Money cents={i.price_cents} bare /> : "—"),
            },
          ]}
        />
      )}

      <Sheet
        open={open}
        onClose={() => {
          setFormError(null);
          setOpen(false);
        }}
        title={t("add")}
        closeLabel={tc("close")}
        footer={
          <div className="space-y-3">
            {formError && (
              <div
                role="alert"
                className="rounded-r2 border border-danger/30 bg-danger/10 px-3 py-2 text-xs font-medium text-danger"
              >
                {formError}
              </div>
            )}
            <Button block disabled={create.isPending} loading={create.isPending} onClick={form.handleSubmit(submit)}>
              {tc("save")}
            </Button>
          </div>
        }
      >
        <form className="space-y-4" noValidate onSubmit={form.handleSubmit(submit)}>
          <Field
            label={t("name")}
            placeholder="e.g. Fresh Milk 500ml"
            error={form.formState.errors.name?.message}
            {...form.register("name")}
          />

          {/* Preset Category Mapping */}
          <SelectField
            label={t("categoryPreset")}
            hint={t("categoryPresetHint")}
            value={form.watch("category_preset") || ""}
            onChange={(e) => handleCategoryPresetChange(e.target.value)}
          >
            <option value="">{t("categoryPresetCustom")}</option>
            {CATEGORY_PRESETS.map((p) => (
              <option key={p.id} value={p.id}>
                {locale === "sw" ? p.nameSw : p.nameEn}
              </option>
            ))}
          </SelectField>

          {/* Descriptive eTIMS Classification Selection */}
          <div className="space-y-1.5">
            <SelectField
              label={t("code")}
              hint={t("codeHint")}
              error={form.formState.errors.etims_class_code?.message}
              value={form.watch("etims_class_code")}
              onChange={(e) => {
                const code = e.target.value;
                form.setValue("etims_class_code", code, { shouldValidate: true });
                const matched = ETIMS_CLASS_CODES.find((c) => c.code === code);
                if (matched && orgVatRegistered) {
                  form.setValue("tax_category", matched.defaultTaxCategory);
                  form.setValue("is_vat_applicable", matched.defaultTaxCategory === "B");
                }
              }}
            >
              {filteredCodes.map((c) => (
                <option key={c.code} value={c.code}>
                  {c.code} · {locale === "sw" ? c.nameSw : c.nameEn}
                </option>
              ))}
            </SelectField>

            <input
              type="text"
              placeholder="Search eTIMS codes or keywords..."
              value={codeSearch}
              onChange={(e) => setCodeSearch(e.target.value)}
              className="block w-full min-h-[36px] rounded-r2 border border-hairline bg-paper-2 px-3 text-xs text-ink placeholder:text-muted focus:border-ink focus:outline-none"
            />
          </div>

          {/* Distinct VAT Checkbox & Tax Category */}
          <div className="rounded-r2 border border-hairline bg-paper-2 p-3 space-y-3">
            <label className="flex items-start gap-2.5 cursor-pointer">
              <input
                type="checkbox"
                checked={form.watch("is_vat_applicable")}
                onChange={(e) => handleVatCheckboxChange(e.target.checked)}
                className="mt-0.5 h-4 w-4 rounded border-hairline text-ink accent-ink focus:ring-ochre"
              />
              <div className="flex flex-col">
                <span className="text-sm font-medium text-ink">{t("isVatApplicable")}</span>
                <span className="text-xs text-muted">
                  {orgVatRegistered ? t("vatRegisteredNotice") : t("isVatApplicableHint")}
                </span>
              </div>
            </label>

            <SelectField
              label={t("taxCategory")}
              value={form.watch("tax_category")}
              onChange={(e) => handleTaxCategoryChange(e.target.value as "A" | "B" | "C" | "D" | "E")}
            >
              {CATS.map((c) => (
                <option key={c} value={c}>
                  {t(`tax${c}`)}
                </option>
              ))}
            </SelectField>
          </div>

          {/* Unit of Measure Dropdown & Price */}
          <div className="grid grid-cols-2 gap-3">
            <SelectField
              label={t("unit")}
              value={form.watch("unit")}
              onChange={(e) => form.setValue("unit", e.target.value)}
            >
              {KRA_UNITS_OF_MEASURE.map((u) => (
                <option key={u.code} value={u.code}>
                  {u.label}
                </option>
              ))}
            </SelectField>

            <Field
              label={t("price")}
              hint={t("priceHint")}
              inputMode="decimal"
              mono
              placeholder="0.00"
              {...form.register("price")}
            />
          </div>
        </form>
      </Sheet>
    </>
  );
}
