"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { Field, SelectField } from "@/components/ui/Field";
import { ApiRequestError, type Schemas } from "@/lib/api/client";
import { useCreateShortcode } from "@/lib/api/queries";
import { normaliseMsisdn } from "@/lib/format";

const KINDS: Schemas["ShortcodeKind"][] = ["till", "paybill", "pochi"];

/** Server rule: 5–12 digits. Pochi numbers are phones, so local `07…` input is normalised first. */
const schema = z
  .object({
    kind: z.enum(["till", "paybill", "pochi"]),
    shortcode: z.string().trim(),
    label: z.string().trim().max(60),
  })
  .transform((v) => {
    if (v.kind === "pochi") {
      const n = normaliseMsisdn(v.shortcode);
      return { ...v, shortcode: n ?? v.shortcode.replace(/\D/g, "") };
    }
    return { ...v, shortcode: v.shortcode.replace(/\s+/g, "") };
  })
  .refine((v) => /^\d{5,12}$/.test(v.shortcode), { path: ["shortcode"] });

type Values = z.input<typeof schema>;

export function ShortcodeForm({ onCreated, heading = true }: { onCreated: (sc: Schemas["Shortcode"]) => void; heading?: boolean }) {
  const t = useTranslations("onboarding.shortcode");
  const tc = useTranslations("common");
  const create = useCreateShortcode();
  const form = useForm<Values, unknown, z.output<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { kind: "till", shortcode: "", label: "" } });
  const kind = form.watch("kind");

  const apiError = create.error;
  let apiMessage: string | null = null;
  if (apiError instanceof ApiRequestError) {
    apiMessage = apiError.code === "shortcode_claimed" ? t("claimed") : tc("errorGeneric", { message: apiError.message });
  } else if (apiError) {
    apiMessage = tc("noConnection");
  }

  const numberHint = kind === "paybill" ? t("numberHintPaybill") : kind === "pochi" ? t("numberHintPochi") : t("numberHintTill");

  return (
    <form
      noValidate
      className="space-y-5"
      onSubmit={form.handleSubmit(async (v) => {
        const sc = await create.mutateAsync({ kind: v.kind, shortcode: v.shortcode, label: v.label || undefined, auto_invoice: true });
        onCreated(sc);
      })}
    >
      {heading && (
        <div>
          <h1>{t("title")}</h1>
          <p className="mt-2 text-ink-2">{t("lead")}</p>
        </div>
      )}
      <SelectField label={t("kind")} {...form.register("kind")}>
        {KINDS.map((k) => (
          <option key={k} value={k}>
            {t(`kinds.${k}`)}
          </option>
        ))}
      </SelectField>
      <Field
        label={t("number")}
        hint={numberHint}
        error={form.formState.errors.shortcode ? t("numberInvalid") : undefined}
        inputMode={kind === "pochi" ? "tel" : "numeric"}
        autoComplete="off"
        placeholder={kind === "pochi" ? "0712 345 678" : "600123"}
        mono
        {...form.register("shortcode")}
      />
      <Field label={t("label")} hint={t("labelHint")} maxLength={60} {...form.register("label")} />
      {apiMessage && (
        <p role="alert" className="text-sm text-red">
          {apiMessage}
        </p>
      )}
      <Button type="submit" block loading={create.isPending}>
        {t("submit")}
      </Button>
    </form>
  );
}
