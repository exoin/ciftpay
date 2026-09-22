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

type AuthMode = "signin" | "signup";

export function LoginForm() {
  const t = useTranslations("login");
  const tc = useTranslations("common");
  const locale = useLocale() as "en" | "sw";
  const router = useRouter();
  const params = useSearchParams();
  const next = params.get("next") ?? "/today";
  const initialMode = params.get("mode") === "signup" ? "signup" : "signin";

  const [mode, setMode] = useState<AuthMode>(initialMode);
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
      <div className="space-y-6">
        {/* Clear Tab Navigation for Sign In vs Create Account */}
        <div className="flex rounded-r2 border border-hairline bg-paper-2 p-1" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={mode === "signin"}
            onClick={() => setMode("signin")}
            className={`flex-1 rounded py-2 text-center text-sm font-medium transition-all ${
              mode === "signin"
                ? "bg-paper text-ink shadow-sm"
                : "text-muted hover:text-ink"
            }`}
          >
            {t("signInTab")}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === "signup"}
            onClick={() => setMode("signup")}
            className={`flex-1 rounded py-2 text-center text-sm font-medium transition-all ${
              mode === "signup"
                ? "bg-paper text-ink shadow-sm"
                : "text-muted hover:text-ink"
            }`}
          >
            {t("signUpTab")}
          </button>
        </div>

        <div>
          <h1 className="text-xl font-semibold text-ink">
            {mode === "signup" ? t("signUpTitle") : t("signInTitle")}
          </h1>
          <p className="mt-1 text-sm text-ink-2">
            {mode === "signup" ? t("signUpLead") : t("signInLead")}
          </p>
        </div>

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

          <div className="pt-2 text-center">
            <button
              type="button"
              onClick={() => setMode(mode === "signin" ? "signup" : "signin")}
              className="text-xs text-ink-2 underline underline-offset-2 hover:text-ink"
            >
              {mode === "signin" ? t("switchModeSignUp") : t("switchModeSignIn")}
            </button>
          </div>
        </form>
      </div>
    );
  }

  return (
    <form
      noValidate
      className="space-y-4"
      onSubmit={codeForm.handleSubmit(async ({ code }) => {
        const s = await verify.mutateAsync({ msisdn, code });
        // If user is solely an accountant or their primary membership is accountant,
        // take them directly to the Organizations portal (/clients).
        const isSolelyAccountant = (s.orgs ?? []).length > 0 && !(s.orgs ?? []).some(m => m.role === "owner" || m.role === "admin" || m.role === "staff");
        if (isSolelyAccountant) {
          router.replace("/clients");
        } else if (hasNoOrg(s)) {
          if (mode === "signup") {
            router.replace("/onboarding");
          } else {
            router.replace("/clients");
          }
        } else {
          router.replace(next);
        }
      })}
    >
      <div>
        <h1 className="text-xl font-semibold text-ink">
          {mode === "signup" ? t("signUpTab") : t("title")}
        </h1>
        <p className="mt-1 text-sm text-ink-2">
          {t("codeSentTo", { msisdn: maskMsisdn(msisdn), minutes: Math.max(1, Math.round(expires / 60)) })}
        </p>
      </div>

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
        {mode === "signup" ? t("verifySignUp") : t("verify")}
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
