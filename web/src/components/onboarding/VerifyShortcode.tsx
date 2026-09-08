"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { StatusChip } from "@/components/ui/StatusChip";
import { ApiRequestError, type Schemas } from "@/lib/api/client";
import { useShortcode, useVerifyShortcode } from "@/lib/api/queries";
import { useSession } from "@/lib/auth";
import { normaliseMsisdn } from "@/lib/format";
import { formatCountdown, payInstructions, remainingSeconds, verifyPhase, type VerifyPhase } from "./verify";

type Shortcode = Schemas["Shortcode"];
type Challenge = Schemas["ShortcodeVerification"];

export type VerifyShortcodeProps = {
  shortcode: Shortcode;
  /** Called once, when the api reports the shortcode verified. */
  onVerified: (sc: Shortcode) => void;
  /** Shown as a ghost action while waiting; omit to hide "Do this later". */
  onLater?: () => void;
  /** Primary action after success; omit to let the parent render its own. */
  onDone?: () => void;
  /** Label for `onDone`; defaults to "Go to Today". */
  doneLabel?: string;
  /** `true` renders the h1 + lead (onboarding), `"compact"` only the lead (inside a Sheet). */
  heading?: boolean | "compact";
};

/**
 * The control check: the merchant pays exactly KES 1 from their own phone to
 * the shortcode; the api's C2B webhook settles the challenge. Opens the
 * challenge on mount, shows the M-Pesa menu path, counts down and polls
 * `GET /shortcodes/{id}` every 3 s. States: starting → pending → verified,
 * or expired ("Start again"), or claimed (another business verified first).
 */
export function VerifyShortcode({ shortcode, onVerified, onLater, onDone, doneLabel, heading = true }: VerifyShortcodeProps) {
  const t = useTranslations("onboarding.verify");
  const tk = useTranslations("onboarding.verify.kindNames");
  const tc = useTranslations("common");
  const ts = useTranslations("status");
  const { data: session } = useSession();
  const verify = useVerifyShortcode();

  const [challenge, setChallenge] = useState<Challenge | null>(null);
  const [challengeAt, setChallengeAt] = useState(0);
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const [secondsLeft, setSecondsLeft] = useState(0);
  const [otherPhone, setOtherPhone] = useState<string | null>(null);
  const [phoneInput, setPhoneInput] = useState("");
  const [phoneError, setPhoneError] = useState(false);

  const start = useCallback(
    async (msisdn?: string) => {
      setErrorCode(null);
      setChallenge(null);
      setChallengeAt(Date.now());
      try {
        const res = await verify.mutateAsync({ id: shortcode.id, msisdn });
        setChallenge(res);
        if (res.expires_at) setSecondsLeft(remainingSeconds(res.expires_at));
      } catch (e) {
        setErrorCode(e instanceof ApiRequestError ? e.code : "network");
      }
    },
    [shortcode.id, verify],
  );

  // Open the challenge as soon as the step is shown.
  const started = useRef(false);
  useEffect(() => {
    if (started.current) return;
    started.current = true;
    void start();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const pendingChallenge = challenge?.status === "pending";
  const { data: polledRaw, dataUpdatedAt } = useShortcode(shortcode.id, { poll: pendingChallenge && secondsLeft > 0 });
  // A poll answered before the current challenge opened may still say "expired".
  const polled = polledRaw && dataUpdatedAt >= challengeAt ? polledRaw : undefined;

  const phase: VerifyPhase = verifyPhase({
    challenge: challenge ? { status: challenge.status } : null,
    polled: polled ?? null,
    secondsLeft,
    errorCode,
  });

  useEffect(() => {
    if (phase !== "pending" || !challenge?.expires_at) return;
    const exp = challenge.expires_at;
    const id = window.setInterval(() => setSecondsLeft(remainingSeconds(exp)), 1000);
    return () => window.clearInterval(id);
  }, [phase, challenge?.expires_at]);

  const reported = useRef(false);
  useEffect(() => {
    if (phase !== "verified" || reported.current) return;
    reported.current = true;
    onVerified(polled?.verified ? polled : challenge?.shortcode ?? { ...shortcode, verified: true });
  }, [phase, polled, challenge, shortcode, onVerified]);

  const kindName = tk(shortcode.kind);
  const msisdn = challenge?.msisdn_masked ?? session?.msisdn_masked ?? "—";
  const steps = payInstructions(shortcode.kind, shortcode.shortcode, challenge?.pay?.account_ref);

  return (
    <div className="space-y-5">
      {heading === true && (
        <div>
          <h1>{t("title", { kind: kindName })}</h1>
          <p className="mt-2 text-ink-2">{t("lead", { msisdn, shortcode: shortcode.shortcode })}</p>
        </div>
      )}
      {heading === "compact" && <p className="text-ink-2">{t("lead", { msisdn, shortcode: shortcode.shortcode })}</p>}

      {phase === "starting" && (
        <p role="status" className="font-mono text-sm text-muted">
          {t("starting")}
        </p>
      )}

      {phase === "pending" && (
        <>
          <section aria-labelledby="pay-how" className="rounded-r2 border border-hairline bg-paper-2 p-4">
            <h2 id="pay-how" className="text-sm font-medium text-ink-2">
              {t("how")}
            </h2>
            <ol className="mt-3 space-y-2">
              {steps.map((s, i) => (
                <li key={s.key} className="flex gap-3">
                  <span aria-hidden className="font-mono text-sm text-muted">
                    {i + 1}.
                  </span>
                  <span className={s.value ? "font-mono" : undefined}>{t(`steps.${s.key}`, { value: s.value ?? "" })}</span>
                </li>
              ))}
            </ol>
          </section>
          <div className="flex items-center justify-between gap-3">
            <p role="status" className="text-sm text-ink-2">
              {t("waiting")}
            </p>
            <span className="font-mono text-sm text-muted" aria-live="off">
              {t("timeLeft", { time: formatCountdown(secondsLeft) })}
            </span>
          </div>
          {otherPhone === null ? (
            <Button type="button" variant="ghost" size="sm" onClick={() => setOtherPhone("")}>
              {t("useOtherPhone")}
            </Button>
          ) : (
            <form
              noValidate
              className="space-y-3"
              onSubmit={(e) => {
                e.preventDefault();
                const n = normaliseMsisdn(phoneInput);
                if (!n) {
                  setPhoneError(true);
                  return;
                }
                setPhoneError(false);
                setOtherPhone(null);
                void start(n);
              }}
            >
              <Field
                label={t("otherPhone")}
                error={phoneError ? t("otherPhoneInvalid") : undefined}
                type="tel"
                inputMode="tel"
                placeholder="0712 345 678"
                mono
                value={phoneInput}
                onChange={(e) => setPhoneInput(e.target.value)}
              />
              <div className="flex gap-2">
                <Button type="submit" variant="secondary" size="sm">
                  {t("otherPhoneSubmit")}
                </Button>
                <Button type="button" variant="ghost" size="sm" onClick={() => setOtherPhone(null)}>
                  {tc("cancel")}
                </Button>
              </div>
            </form>
          )}
        </>
      )}

      {phase === "verified" && (
        <div className="rounded-r2 border border-hairline bg-ok-bg p-4">
          <StatusChip tone="acked">{ts("verified")}</StatusChip>
          <p className="mt-2">{t("verified", { shortcode: shortcode.shortcode })}</p>
        </div>
      )}

      {phase === "expired" && (
        <div className="rounded-r2 border border-ochre bg-warn-bg p-4">
          <p className="font-medium">{t("expired", { msisdn })}</p>
          <p className="mt-1 text-sm text-ink-2">{t("expiredHint")}</p>
        </div>
      )}

      {phase === "claimed" && (
        <div role="alert" className="rounded-r2 border border-red bg-bad-bg p-4">
          <p className="font-medium">{t("claimed")}</p>
          <p className="mt-1 text-sm text-ink-2">{t("claimedHint")}</p>
        </div>
      )}

      {phase === "error" && (
        <p role="alert" className="text-sm text-red">
          {errorCode === "network" ? tc("noConnection") : tc("errorGeneric", { message: verify.error instanceof Error ? verify.error.message : "" })}
        </p>
      )}

      <div className="space-y-2">
        {(phase === "expired" || phase === "error") && (
          <Button type="button" block loading={verify.isPending} onClick={() => void start()}>
            {phase === "expired" ? t("startAgain") : tc("retry")}
          </Button>
        )}
        {phase === "verified" && onDone && (
          <Button type="button" block onClick={onDone}>
            {doneLabel ?? t("done")}
          </Button>
        )}
        {phase !== "verified" && onLater && (
          <Button type="button" variant="ghost" block onClick={onLater}>
            {t("later")}
          </Button>
        )}
      </div>
    </div>
  );
}
