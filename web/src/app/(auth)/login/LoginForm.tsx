"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useLocale, useTranslations } from "next-intl";
import { RotateCw } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { useToast } from "@/components/ui/Toast";
import { readSession, useRequestOtp, useVerifyOtp } from "@/lib/auth";
import { ApiRequestError } from "@/lib/api/client";
import { maskMsisdn, normaliseMsisdn } from "@/lib/format";

const phoneSchema = z.object({ phone: z.string().refine((v) => normaliseMsisdn(v) !== null) });
const codeSchema = z.object({ code: z.string().regex(/^\d{6}$/) });

type AuthMode = "signin" | "signup";

export function LoginForm() {
  const t = useTranslations("login");
  const tc = useTranslations("common");
  const toast = useToast();
  const locale = useLocale() as "en" | "sw";
  const router = useRouter();
  const params = useSearchParams();
  const next = params.get("next") ?? "/today";
  const initialMode = params.get("mode") === "signup" ? "signup" : "signin";

  const [mode, setMode] = useState<AuthMode>(initialMode);
  const [msisdn, setMsisdn] = useState<string | null>(null);
  const [expires, setExpires] = useState(300);
  const [cooldown, setCooldown] = useState(30);

  const request = useRequestOtp();
  const verify = useVerifyOtp();

  // Cooldown countdown timer when OTP is sent or resent
  useEffect(() => {
    if (!msisdn || cooldown <= 0) return;
    const timer = setInterval(() => {
      setCooldown((prev) => Math.max(0, prev - 1));
    }, 1000);
    return () => clearInterval(timer);
  }, [msisdn, cooldown]);

  const routePostLogin = useCallback(
    (orgs: { role?: string }[] | undefined) => {
      if (!orgs || orgs.length === 0) {
        router.replace("/onboarding");
      } else if (orgs.some((o) => o.role === "merchant" || o.role === "owner" || o.role === "admin" || o.role === "staff")) {
        router.replace(next && next !== "/clients" ? next : "/today");
      } else if (orgs.some((o) => o.role === "accountant")) {
        router.replace("/clients");
      } else {
        router.replace("/onboarding");
      }
    },
    [router, next],
  );

  useEffect(() => {
    const s = readSession();
    if (s) {
      routePostLogin(s.orgs);
    }
  }, [routePostLogin]);

  const phoneForm = useForm<z.infer<typeof phoneSchema>>({ resolver: zodResolver(phoneSchema), defaultValues: { phone: "" } });
  const codeForm = useForm<z.infer<typeof codeSchema>>({ resolver: zodResolver(codeSchema), defaultValues: { code: "" } });

  async function handleResendCode() {
    if (!msisdn || cooldown > 0 || request.isPending) return;
    try {
      const res = await request.mutateAsync({ msisdn, locale });
      setExpires(res.expires_in_seconds);
      setCooldown(30);
      toast.push(t("otpResent"));
    } catch (err: unknown) {
      if (err instanceof ApiRequestError) {
        toast.push(tc("errorGeneric", { message: err.message }), "error");
      } else {
        toast.push(tc("noConnection"), "error");
      }
    }
  }

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
            setCooldown(30);
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
        routePostLogin(s.orgs);
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

      {/* Primary Action Button */}
      <Button type="submit" block loading={verify.isPending}>
        {mode === "signup" ? t("verifySignUp") : t("verify")}
      </Button>

      {/* Resend OTP button with rate-limiting countdown */}
      <div className="flex items-center justify-between pt-1">
        <button
          type="button"
          disabled={cooldown > 0 || request.isPending}
          onClick={handleResendCode}
          className="inline-flex items-center gap-1.5 text-xs font-medium text-ink-2 hover:text-ink disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          <RotateCw className={`size-3.5 ${request.isPending ? "animate-spin" : ""}`} />
          {cooldown > 0
            ? t("resendCooldown", { seconds: cooldown })
            : t("resendOtp")}
        </button>

        <button
          type="button"
          onClick={() => setMsisdn(null)}
          className="text-xs text-muted hover:text-ink transition-colors"
        >
          {t("changeNumber")}
        </button>
      </div>
    </form>
  );
}

function errorText(err: unknown, tc: ReturnType<typeof useTranslations<"common">>): string {
  if (err instanceof ApiRequestError) return tc("errorGeneric", { message: err.message });
  return tc("noConnection");
}
