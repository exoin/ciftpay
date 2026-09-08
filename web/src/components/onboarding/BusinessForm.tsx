"use client";

import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { ApiRequestError, type Schemas } from "@/lib/api/client";
import { useCreateOrg } from "@/lib/auth";

/** Mirrors org.PINRe on the server: A/P + nine digits + a check letter. */
export const KRA_PIN_RE = /^[AP]\d{9}[A-Z]$/;

const schema = z.object({
  name: z.string().trim().min(2).max(80),
  kra_pin: z
    .string()
    .transform((v) => v.replace(/\s+/g, "").toUpperCase())
    .pipe(z.string().regex(KRA_PIN_RE)),
  vat_registered: z.boolean(),
});

type Values = z.infer<typeof schema>;

export function BusinessForm({ onCreated }: { onCreated: (org: Schemas["Org"]) => void }) {
  const t = useTranslations("onboarding.business");
  const tc = useTranslations("common");
  const locale = useLocale() as Schemas["Locale"];
  const create = useCreateOrg();
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: { name: "", kra_pin: "", vat_registered: false } });

  const apiError = create.error;
  let apiMessage: string | null = null;
  if (apiError instanceof ApiRequestError) {
    if (apiError.code === "pin_unknown") apiMessage = t("pinUnknown");
    else if (apiError.status === 409) apiMessage = t("pinTaken");
    else apiMessage = tc("errorGeneric", { message: apiError.message });
  } else if (apiError) {
    apiMessage = tc("noConnection");
  }

  return (
    <form
      noValidate
      className="space-y-5"
      onSubmit={form.handleSubmit(async (v) => {
        const org = await create.mutateAsync({ name: v.name, kra_pin: v.kra_pin, vat_registered: v.vat_registered, locale });
        onCreated(org);
      })}
    >
      <div>
        <h1>{t("title")}</h1>
        <p className="mt-2 text-ink-2">{t("lead")}</p>
      </div>
      <Field label={t("name")} hint={t("nameHint")} error={form.formState.errors.name ? t("nameInvalid") : undefined} autoComplete="organization" maxLength={80} {...form.register("name")} />
      <Field
        label={t("pin")}
        hint={t("pinHint")}
        error={form.formState.errors.kra_pin ? t("pinInvalid") : undefined}
        placeholder="P051234567X"
        autoCapitalize="characters"
        autoCorrect="off"
        spellCheck={false}
        maxLength={11}
        mono
        {...form.register("kra_pin")}
      />
      <label className="flex min-h-[var(--touch)] items-start gap-3">
        <input type="checkbox" className="mt-1 size-5 accent-[var(--color-green)]" {...form.register("vat_registered")} />
        <span>
          <span className="block text-sm font-medium text-ink-2">{t("vat")}</span>
          <span className="block text-xs text-muted">{t("vatHint")}</span>
        </span>
      </label>
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
