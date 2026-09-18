"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { StatusChip, type ChipTone } from "@/components/ui/StatusChip";
import { ApiRequestError } from "@/lib/api/client";
import { useConfigureEtims, useEtimsSettings } from "@/lib/api/queries";

const PORTAL_URL = "https://etims-sbx.kra.go.ke";
const BRANCH_RE = /^[0-9]{2}$/;

const STATUS_TONE: Record<string, ChipTone> = {
  unconfigured: "neutral",
  initialized: "acked",
  failed: "failed",
};

/**
 * Tax Settings (ADR-0009): lets a merchant connect CiftPay directly to KRA
 * eTIMS by entering their branch id and the device serial KRA issued them.
 * Deliberately decoupled from M-Pesa onboarding (ADR-0008) — a merchant can
 * ignore this entirely and their sales still record; only KRA filing waits.
 */
export function EtimsSection() {
  const t = useTranslations("settings.etims");
  const tc = useTranslations("common");
  const { data, isPending } = useEtimsSettings();
  const configure = useConfigureEtims();
  const [editing, setEditing] = useState(false);
  const [branchId, setBranchId] = useState("00");
  const [deviceSerial, setDeviceSerial] = useState("");
  const [fieldErrors, setFieldErrors] = useState<{ branchId?: string; deviceSerial?: string }>({});
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  if (isPending || !data) {
    return null;
  }

  const showForm = editing || data.status !== "initialized";
  const apiError = configure.error;
  const apiMessage = apiError instanceof ApiRequestError ? apiError.message : apiError ? tc("noConnection") : null;

  function submit() {
    const errs: typeof fieldErrors = {};
    if (branchId && !BRANCH_RE.test(branchId)) errs.branchId = t("branchIdInvalid");
    if (!deviceSerial.trim()) errs.deviceSerial = t("deviceSerialInvalid");
    setFieldErrors(errs);
    if (Object.keys(errs).length > 0) return;
    setSuccessMsg(null);
    configure.mutate(
      { kra_bhf_id: branchId || "00", kra_device_serial: deviceSerial.trim() },
      {
        onSuccess: () => {
          setEditing(false);
          setSuccessMsg(t("connectSuccess"));
        },
      }
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-ink-2">{t("lead")}</p>
        <StatusChip tone={STATUS_TONE[data.status] ?? "neutral"}>{t(`status.${data.status}`)}</StatusChip>
      </div>

      {data.status === "unconfigured" && <p className="text-xs text-muted">{t("pendingNote")}</p>}
      {data.status === "failed" && data.failed_reason && (
        <p role="alert" className="text-xs text-red">
          {data.failed_reason}
        </p>
      )}

      {successMsg && (
        <div role="status" className="rounded bg-green/10 p-2.5 text-xs font-medium text-green">
          {successMsg}
        </div>
      )}

      {!showForm ? (
        <>
          <dl className="font-mono text-sm">
            <div className="flex justify-between gap-4 py-1">
              <dt className="text-muted">{t("branchId")}</dt>
              <dd>{data.kra_bhf_id}</dd>
            </div>
            <div className="flex justify-between gap-4 py-1">
              <dt className="text-muted">{t("deviceSerial")}</dt>
              <dd>{data.kra_device_serial}</dd>
            </div>
          </dl>
          <Button size="sm" variant="secondary" onClick={() => setEditing(true)}>
            {t("reconfigure")}
          </Button>
        </>
      ) : (
        <div className="space-y-4">
          <div className="space-y-2 rounded-r2 border border-hairline bg-paper-2 p-4 text-sm">
            <p className="font-medium text-ink-2">{t("howTitle")}</p>
            <ol className="list-decimal space-y-1 pl-5 text-ink-2">
              <li>{t("how1")}</li>
              <li>{t("how2")}</li>
              <li>{t("how3")}</li>
            </ol>
            <a href={PORTAL_URL} target="_blank" rel="noopener noreferrer" className="inline-block font-medium text-green underline underline-offset-2">
              {t("portalLink")}
            </a>
          </div>

          <Field
            label={t("branchId")}
            hint={t("branchIdHint")}
            error={fieldErrors.branchId}
            inputMode="numeric"
            maxLength={2}
            mono
            value={branchId}
            onChange={(e) => setBranchId(e.target.value.replace(/\D/g, ""))}
          />
          <Field
            label={t("deviceSerial")}
            hint={t("deviceSerialHint")}
            error={fieldErrors.deviceSerial}
            autoComplete="off"
            mono
            value={deviceSerial}
            onChange={(e) => setDeviceSerial(e.target.value)}
          />

          {configure.isPending && (
            <div className="flex items-center gap-2 rounded bg-paper-2 p-3 text-xs text-ink-2">
              <span className="inline-block size-3.5 animate-spin rounded-full border-2 border-ochre border-t-transparent" />
              <span>{t("connectingMsg")}</span>
            </div>
          )}

          {apiMessage && (
            <p role="alert" className="text-sm text-red">
              {apiMessage}
            </p>
          )}

          <div className="flex gap-2">
            <Button onClick={submit} loading={configure.isPending}>
              {t("connect")}
            </Button>
            {editing && data.status === "initialized" && (
              <Button variant="ghost" onClick={() => setEditing(false)}>
                {tc("cancel")}
              </Button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
