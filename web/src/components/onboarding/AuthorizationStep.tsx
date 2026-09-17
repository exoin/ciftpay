"use client";

import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { ApiRequestError, type Schemas } from "@/lib/api/client";
import { useSubmitShortcodeAuthorization } from "@/lib/api/queries";
import { cn } from "@/lib/cn";

const ACCEPTED_TYPES = ["image/jpeg", "image/png", "image/webp", "application/pdf"];
const MAX_BYTES = 10 * 1024 * 1024; // 10 MB, mirrors backend LetterMaxBytes

/**
 * The Administrative Gate's one merchant-facing step (ADR-0008): print the
 * pre-filled authorization letter, sign and stamp it, then upload a photo or
 * PDF of it here. CiftPay never touches Safaricom credentials — an operator
 * forwards the letter and later marks the shortcode `verified`.
 *
 * Shared between onboarding (full heading, offers "do this later") and the
 * Settings sheet for a shortcode that still needs a letter (`heading="compact"`).
 */
export function AuthorizationStep({
  shortcode,
  onSubmitted,
  onLater,
  heading = true,
}: {
  shortcode: Schemas["Shortcode"];
  onSubmitted: () => void;
  onLater?: () => void;
  heading?: boolean | "compact";
}) {
  const t = useTranslations("onboarding.authorization");
  const inputRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [fileError, setFileError] = useState<string | null>(null);
  const submit = useSubmitShortcodeAuthorization();

  if (shortcode.status === "verified") {
    return (
      <div className="space-y-5">
        <p className="text-ink-2">{t("alreadyVerified")}</p>
        <Button block onClick={onSubmitted}>
          {t("continue")}
        </Button>
      </div>
    );
  }

  if (submit.isSuccess) {
    return (
      <div className="space-y-5">
        <p className="text-ink-2">{t("submitted")}</p>
        <Button block onClick={onSubmitted}>
          {t("continue")}
        </Button>
      </div>
    );
  }

  function validate(f: File): string | null {
    if (!ACCEPTED_TYPES.includes(f.type)) return t("fileTypeInvalid");
    if (f.size > MAX_BYTES) return t("fileTooLarge");
    return null;
  }

  function pick(f: File | null) {
    if (!f) {
      setFile(null);
      setFileError(null);
      return;
    }
    const err = validate(f);
    setFile(err ? null : f);
    setFileError(err);
  }

  const apiError = submit.error;
  let apiMessage: string | null = null;
  if (apiError instanceof ApiRequestError) {
    apiMessage = apiError.code === "shortcode_claimed" ? t("claimed") : apiError.code === "validation" ? t("fileRejected") : apiError.message;
  } else if (apiError) {
    apiMessage = t("fileRejected");
  }

  return (
    <div className="space-y-5">
      {heading && (
        <div>
          <h1 className={heading === "compact" ? "text-lg" : undefined}>{t("title")}</h1>
          <p className="mt-2 text-ink-2">{t("lead", { shortcode: shortcode.shortcode })}</p>
        </div>
      )}

      <section className="space-y-2 rounded-r2 border border-hairline bg-paper-2 p-4">
        <h2 className="text-sm font-medium text-ink-2">{t("how")}</h2>
        <ol className="list-decimal space-y-1.5 pl-5 text-sm text-ink">
          <li>{t("steps.print")}</li>
          <li>{t("steps.sign")}</li>
          <li>{t("steps.stamp")}</li>
          <li>{t("steps.upload")}</li>
        </ol>
        <a
          href={`/onboarding/letter?shortcode=${encodeURIComponent(shortcode.shortcode)}&label=${encodeURIComponent(shortcode.label ?? "")}`}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-block text-sm font-medium text-green underline underline-offset-2"
        >
          {t("printLink")}
        </a>
      </section>

      <p className="text-sm text-muted">{t("note")}</p>

      <div className="space-y-1.5">
        <span className="text-sm font-medium text-ink-2">{t("file")}</span>
        <div
          className={cn(
            "flex min-h-[var(--touch)] items-center justify-between gap-3 rounded-r2 border border-dashed border-hairline bg-paper-2 px-3 py-3",
            fileError && "border-red",
          )}
        >
          <span className="min-w-0 truncate text-sm text-ink-2">{file ? file.name : t("choose")}</span>
          <Button type="button" size="sm" variant="secondary" onClick={() => inputRef.current?.click()}>
            {file ? t("change") : t("browse")}
          </Button>
        </div>
        <input
          ref={inputRef}
          type="file"
          accept={ACCEPTED_TYPES.join(",")}
          className="sr-only"
          onChange={(e) => pick(e.target.files?.[0] ?? null)}
        />
        {fileError ? (
          <p role="alert" className="text-xs text-red">
            {fileError}
          </p>
        ) : (
          <p className="text-xs text-muted">{t("fileHint")}</p>
        )}
      </div>

      {apiMessage && (
        <p role="alert" className="text-sm text-red">
          {apiMessage}
        </p>
      )}

      <div className="space-y-2">
        <Button
          block
          disabled={!file}
          loading={submit.isPending}
          onClick={() => {
            if (!file) return;
            submit.mutate({ id: shortcode.id, file });
          }}
        >
          {t("submit")}
        </Button>
        {onLater && (
          <Button type="button" variant="ghost" block onClick={onLater}>
            {t("later")}
          </Button>
        )}
      </div>
    </div>
  );
}
