"use client";

import { useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { hasNoOrg, readSession, useRequestOtp, useVerifyOtp } from "@/lib/auth";
import { ApiRequestError } from "@/lib/api/client";
import { maskMsisdn, normaliseMsisdn } from "@/lib/format";

const phoneSchema = z.object({ phone: z.string().refine((v) => normaliseMsisdn(v) !== null) });
const codeSchema = z.object({ code: z.string().regex(/^\d{6}$/) });

export function LoginForm() {
  const t = useTranslations("login");
  const tc = useTranslations("common");
  const locale = useLocale() as "en" | "sw";
  const router = useRouter();
  const params = useSearchParams();
  const next = params.get("next") ?? "/today";

  const [msisdn, setMsisdn] = useState<string | null>(null);
  const [expires, setExpires] = useState(300);
  const request = useRequestOtp();
  const verify = useVerifyOtp();

  useEffect(() => {
    const s = readSession();
    if (s) router.replace(hasNoOrg(s) ? "/onboarding" : next);
  }, [router, next]);

  const phoneForm = useForm<z.infer<typeof phoneSchema>>({ resolver: zodResolver(phoneSchema), defaultValues: { phone: "" } });
  const codeForm = useForm<z.infer<typeof codeSchema>>({ resolver: zodResolver(codeSchema), defaultValues: { code: "" } });

  if (!msisdn) {
    return (
      <form
        noValidate
        className="space-y-4"
        onSubmit={phoneForm.handleSubmit(async ({ phone }) => {
          const n = normaliseMsisdn(phone)!;
          const res = await request.mutateAsync({ msisdn: n, locale });
          setExpires(res.expires_in_seconds);
          setMsisdn(n);
        })}
      >
        <Field
          label={t("phone")}
          hint={t("phoneHint")}
          error={phoneForm.formState.errors.phone ? t("phoneInvalid") : undefined}
          type="tel"
          inputMode="tel"
          autoComplete="tel"
          placeholder="0712 345 678"
          mono
          {...phoneForm.register("phone")}
        />
        {request.error && (
          <p role="alert" className="text-sm text-red">
            {errorText(request.error, tc)}
          </p>
        )}
        <Button type="submit" block loading={request.isPending}>
          {t("sendCode")}
        </Button>
      </form>
    );
  }

  return (
    <form
      noValidate
      className="space-y-4"
      onSubmit={codeForm.handleSubmit(async ({ code }) => {
        const s = await verify.mutateAsync({ msisdn, code });
        // A brand-new user has no business yet: onboarding comes before anything else.
        router.replace(hasNoOrg(s) ? "/onboarding" : next);
      })}
    >
      <p className="text-sm text-ink-2">{t("codeSentTo", { msisdn: maskMsisdn(msisdn), minutes: Math.max(1, Math.round(expires / 60)) })}</p>
      <Field
        label={t("code")}
        error={codeForm.formState.errors.code ? t("codeInvalid") : undefined}
        inputMode="numeric"
        autoComplete="one-time-code"
        maxLength={6}
        mono
        autoFocus
        {...codeForm.register("code")}
      />
      {verify.error && (
        <p role="alert" className="text-sm text-red">
          {verify.error instanceof ApiRequestError && verify.error.status === 401 ? t("wrongCode") : errorText(verify.error, tc)}
        </p>
      )}
      <Button type="submit" block loading={verify.isPending}>
        {t("verify")}
      </Button>
      <Button type="button" variant="ghost" block onClick={() => setMsisdn(null)}>
        {t("changeNumber")}
      </Button>
    </form>
  );
}

function errorText(err: unknown, tc: ReturnType<typeof useTranslations<"common">>): string {
  if (err instanceof ApiRequestError) return tc("errorGeneric", { message: err.message });
  return tc("noConnection");
}
