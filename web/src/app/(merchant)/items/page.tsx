"use client";

import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslations } from "next-intl";
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
import { useCreateItem, useItems } from "@/lib/api/queries";
import { ApiRequestError } from "@/lib/api/client";

const schema = z.object({
  name: z.string().min(1),
  etims_class_code: z.string().min(4),
  tax_category: z.enum(["A", "B", "C", "D", "E"]),
  unit: z.string().min(1),
  price: z.string().optional(),
});
type Form = z.infer<typeof schema>;

const CATS = ["A", "B", "C", "D", "E"] as const;

export default function ItemsPage() {
  const t = useTranslations("items");
  const tc = useTranslations("common");
  const toast = useToast();
  const { data, isPending } = useItems();
  const create = useCreateItem();
  const [open, setOpen] = useState(false);
  const form = useForm<Form>({ resolver: zodResolver(schema), defaultValues: { name: "", etims_class_code: "", tax_category: "B", unit: "PCS", price: "" } });
  const rows = data?.data ?? [];

  async function submit(v: Form) {
    try {
      await create.mutateAsync({
        name: v.name,
        etims_class_code: v.etims_class_code,
        tax_category: v.tax_category,
        unit: v.unit,
        price_cents: v.price ? Math.round(Number(v.price) * 100) : null,
      });
      toast.push(t("saved"));
      form.reset();
      setOpen(false);
    } catch (e) {
      toast.push(e instanceof ApiRequestError ? tc("errorGeneric", { message: e.message }) : tc("noConnection"), "error");
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
        <EmptyState
          action={<Button onClick={() => setOpen(true)}>{t("add")}</Button>}
        >
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
              {i.etims_class_code} · {i.tax_category}
            </span>
          )}
          trailing={(i) => (i.price_cents != null ? <Money cents={i.price_cents} /> : <span className="text-muted">—</span>)}
          columns={[
            { key: "name", header: t("name"), cell: (i) => i.name },
            { key: "code", header: t("code"), cell: (i) => <span className="font-mono">{i.etims_class_code}</span> },
            { key: "cat", header: t("taxCategory"), cell: (i) => <span className="font-mono">{i.tax_category}</span> },
            { key: "unit", header: t("unit"), cell: (i) => <span className="font-mono">{i.unit}</span> },
            { key: "price", header: "KES", numeric: true, cell: (i) => (i.price_cents != null ? <Money cents={i.price_cents} bare /> : "—") },
          ]}
        />
      )}

      <Sheet
        open={open}
        onClose={() => setOpen(false)}
        title={t("add")}
        closeLabel={tc("close")}
        footer={
          <Button block loading={create.isPending} onClick={form.handleSubmit(submit)}>
            {tc("save")}
          </Button>
        }
      >
        <form className="space-y-4" noValidate onSubmit={form.handleSubmit(submit)}>
          <Field label={t("name")} error={form.formState.errors.name?.message} {...form.register("name")} />
          <Field label={t("code")} hint={t("codeHint")} mono error={form.formState.errors.etims_class_code?.message} {...form.register("etims_class_code")} />
          <SelectField label={t("taxCategory")} {...form.register("tax_category")}>
            {CATS.map((c) => (
              <option key={c} value={c}>
                {t(`tax${c}`)}
              </option>
            ))}
          </SelectField>
          <div className="grid grid-cols-2 gap-3">
            <Field label={t("unit")} mono {...form.register("unit")} />
            <Field label={t("price")} hint={t("priceHint")} inputMode="decimal" mono {...form.register("price")} />
          </div>
        </form>
      </Sheet>
    </>
  );
}
