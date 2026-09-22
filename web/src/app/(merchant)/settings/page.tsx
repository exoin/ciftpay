"use client";

import { useState, type ReactNode } from "react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { UserPlus, Trash2, User } from "lucide-react";
import { TopBar } from "@/components/shell/TopBar";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Sheet } from "@/components/ui/Sheet";
import { StatusChip } from "@/components/ui/StatusChip";
import { useToast } from "@/components/ui/Toast";
import { OrgSwitcher } from "@/components/shell/OrgSwitcher";
import { AuthorizationStep } from "@/components/onboarding/AuthorizationStep";
import { ShortcodeForm } from "@/components/onboarding/ShortcodeForm";
import { EtimsSection } from "@/components/settings/EtimsSection";
import {
  qk,
  useCurrentOrg,
  useShortcodes,
  useOrgMembers,
  useOrgInvites,
  useCreateInvite,
  useRevokeInvite,
  useRemoveMember,
} from "@/lib/api/queries";
import { rawGet, type Schemas } from "@/lib/api/client";
import { useLogout, useActiveMembership } from "@/lib/auth";
import { setLocale } from "@/lib/i18n/actions";
import { cn } from "@/lib/cn";
import { shortcodeAction, shortcodeChip } from "@/lib/status";

export default function SettingsPage() {
  const t = useTranslations("settings");
  const tc = useTranslations("common");
  const ts = useTranslations("status");
  const locale = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();

  const { data: org } = useCurrentOrg();
  const { data: shortcodes } = useShortcodes();
  const { data: membersData } = useOrgMembers();
  const { data: invitesData } = useOrgInvites();

  const createInvite = useCreateInvite();
  const revokeInvite = useRevokeInvite();
  const removeMember = useRemoveMember();

  const membership = useActiveMembership();
  const isAccountant = membership?.role === "accountant";

  // The add/letter sheet for shortcodes
  const [sheet, setSheet] = useState<{ mode: "add" } | { mode: "letter"; shortcode: Schemas["Shortcode"] } | null>(null);
  const closeSheet = () => {
    setSheet(null);
    void qc.invalidateQueries({ queryKey: qk.shortcodes });
  };

  // Accountant / Team invite modal state
  const [inviteOpen, setInviteOpen] = useState(false);
  const [invitePhone, setInvitePhone] = useState("");
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteRole, setInviteRole] = useState<"accountant" | "staff" | "admin">("accountant");

  const { data: ent } = useQuery({
    queryKey: qk.entitlement,
    queryFn: () => rawGet<{ plan_code: string; plan_name: string; invoices_acked: number; invoice_cap: number | null }>("/billing/entitlement"),
  });
  const logout = useLogout();

  async function handleSendInvite(e: React.FormEvent) {
    e.preventDefault();
    if (!invitePhone && !inviteEmail) return;
    try {
      await createInvite.mutateAsync({
        phone: invitePhone.trim() || undefined,
        email: inviteEmail.trim() || undefined,
        role: inviteRole,
      });
      toast.push(t("inviteSuccess"));
      setInviteOpen(false);
      setInvitePhone("");
      setInviteEmail("");
    } catch (err: unknown) {
      toast.push(err instanceof Error ? err.message : "Failed to send invitation", "error");
    }
  }

  async function handleRevokeInvite(id: string) {
    try {
      await revokeInvite.mutateAsync(id);
      toast.push(t("revokeSuccess"));
    } catch (err: unknown) {
      toast.push(err instanceof Error ? err.message : "Failed to revoke invitation", "error");
    }
  }

  async function handleRemoveMember(userId: string) {
    try {
      await removeMember.mutateAsync(userId);
      toast.push(t("removeSuccess"));
    } catch (err: unknown) {
      toast.push(err instanceof Error ? err.message : "Failed to remove member", "error");
    }
  }

  const members = membersData?.data ?? [];
  const invites = invitesData?.data ?? [];

  return (
    <>
      <TopBar title={t("title")} />
      <div className="space-y-10">
        <Section title={t("business")}>
          <dl className="font-mono text-sm">
            <div className="flex justify-between gap-4 py-2">
              <dt className="text-muted">{t("business")}</dt>
              <dd>{org?.name ?? "—"}</dd>
            </div>
            <div className="flex justify-between gap-4 py-2">
              <dt className="text-muted">{t("kraPin")}</dt>
              <dd>{org?.kra_pin_masked ?? "—"}</dd>
            </div>
            {isAccountant && (
              <div className="flex justify-between gap-4 py-2">
                <dt className="text-muted">Role</dt>
                <dd>
                  <StatusChip tone="neutral">{t("readOnlyBadge")}</StatusChip>
                </dd>
              </div>
            )}
          </dl>
          <div className="lg:hidden">
            <OrgSwitcher />
          </div>
        </Section>

        {/* Team & Accountant Invites Section (Only owners & admins can invite) */}
        <Section
          title={t("team")}
          action={
            !isAccountant ? (
              <Button size="sm" variant="secondary" onClick={() => setInviteOpen(true)}>
                <span className="flex items-center gap-1.5">
                  <UserPlus className="size-4" />
                  {t("inviteAccountant")}
                </span>
              </Button>
            ) : undefined
          }
        >
          <div className="space-y-4">
            {/* Active Members List */}
            <div>
              <h3 className="text-xs font-semibold uppercase text-muted mb-2">{t("membersTitle")}</h3>
              {members.length === 0 ? (
                <EmptyState>{t("noMembers")}</EmptyState>
              ) : (
                <ul className="ruled">
                  {members.map((m) => (
                    <li key={m.id} className="flex min-h-[var(--row)] items-center justify-between gap-3 py-2">
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-2">
                          <User className="size-4 text-muted shrink-0" />
                          <span className="font-medium text-ink">{m.name || m.phone_masked || "Member"}</span>
                          <StatusChip tone={m.role === "accountant" ? "neutral" : "acked"}>
                            {m.role === "accountant" ? t("roleAccountant") : m.role}
                          </StatusChip>
                        </div>
                        {m.phone_masked && <div className="font-mono text-xs text-muted pl-6">{m.phone_masked}</div>}
                      </div>
                      {!isAccountant && m.role !== "owner" && (
                        <button
                          type="button"
                          onClick={() => handleRemoveMember(m.user_id)}
                          className="p-1 text-muted hover:text-red transition-colors"
                          aria-label={t("removeMember")}
                        >
                          <Trash2 className="size-4" />
                        </button>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </div>

            {/* Pending Invites List */}
            {!isAccountant && invites.length > 0 && (
              <div className="pt-3 border-t border-hairline">
                <h3 className="text-xs font-semibold uppercase text-muted mb-2">{t("pendingInvites")}</h3>
                <ul className="ruled">
                  {invites.map((inv) => (
                    <li key={inv.id} className="flex min-h-[var(--row)] items-center justify-between gap-3 py-2">
                      <div className="min-w-0 flex-1">
                        <span className="font-mono text-sm text-ink">{inv.phone || inv.email}</span>
                        <div className="flex items-center gap-2 mt-0.5">
                          <StatusChip tone="pending">{inv.status}</StatusChip>
                          <span className="text-xs text-muted">Role: {inv.role}</span>
                        </div>
                      </div>
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() => handleRevokeInvite(inv.id)}
                      >
                        {t("revokeInvite")}
                      </Button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        </Section>

        {/* Shortcodes section: read-only for accountants */}
        <Section
          title={t("shortcodes")}
          action={
            !isAccountant ? (
              <Button size="sm" variant="secondary" onClick={() => setSheet({ mode: "add" })}>
                {t("addAction")}
              </Button>
            ) : undefined
          }
        >
          {!shortcodes || shortcodes.data.length === 0 ? (
            <EmptyState>{t("addShortcode")}</EmptyState>
          ) : (
            <ul className="ruled">
              {shortcodes.data.map((s) => {
                const chip = shortcodeChip(s.status);
                const action = shortcodeAction(s);
                return (
                  <li key={s.id} className="flex min-h-[var(--row)] flex-wrap items-center justify-between gap-3 py-2">
                    <span className="min-w-0 flex-1 basis-40">
                      <span className="block font-mono">{s.shortcode}</span>
                      <span className="block text-sm text-muted">
                        {s.kind}
                        {s.label ? ` · ${s.label}` : ""}
                      </span>
                      {s.status === "rejected" && s.rejection_reason && <span className="block text-xs text-red">{s.rejection_reason}</span>}
                      {s.c2b_urls_registered_at && <span className="block text-xs text-muted">{t("c2bConnected")}</span>}
                      {action === "waiting" && <span className="block text-xs text-muted">{t("letterWaiting")}</span>}
                    </span>
                    <span className="flex shrink-0 items-center gap-2">
                      <StatusChip tone={chip.tone}>{ts(chip.key)}</StatusChip>
                      {!isAccountant && action === "upload" && (
                        <Button size="sm" variant="secondary" onClick={() => setSheet({ mode: "letter", shortcode: s })}>
                          {t("uploadLetter")}
                        </Button>
                      )}
                    </span>
                  </li>
                );
              })}
            </ul>
          )}
        </Section>

        {/* KRA Connection section: read-only note for accountants */}
        <Section title={t("etims.title")}>
          {isAccountant ? (
            <p className="text-sm text-muted">
              Accountants have read-only access to view tax status. KRA device settings can only be altered by business owners or admins.
            </p>
          ) : (
            <EtimsSection />
          )}
        </Section>

        <Section title={t("language")}>
          <div className="flex gap-2" role="radiogroup" aria-label={t("language")}>
            {(["en", "sw"] as const).map((l) => (
              <button
                key={l}
                type="button"
                role="radio"
                aria-checked={locale === l}
                onClick={async () => {
                  await setLocale(l);
                  router.refresh();
                }}
                className={cn("min-h-[var(--touch)] rounded-r2 border px-4 text-sm", locale === l ? "border-ink bg-paper-3" : "border-hairline bg-paper-2 hover:bg-paper-3")}
              >
                {l === "en" ? t("english") : t("swahili")}
              </button>
            ))}
          </div>
        </Section>

        <Section title={t("plan")}>
          <p className="font-mono text-sm">
            {!ent
              ? "—"
              : ent.invoice_cap == null
                ? t("planUnlimited", { plan: ent.plan_name })
                : t("planLead", { plan: ent.plan_name, used: ent.invoices_acked, limit: ent.invoice_cap })}
          </p>
        </Section>

        <Section title={t("receiptTemplate")}>
          <p className="text-sm text-ink-2">{t("receiptLead")}</p>
        </Section>

        <Section title={t("account")}>
          <Button variant="danger" onClick={() => logout.mutate()} loading={logout.isPending}>
            {t("signOut")}
          </Button>
        </Section>
      </div>

      {/* Shortcode sheet */}
      <Sheet open={sheet !== null} onClose={closeSheet} title={sheet?.mode === "letter" ? t("letterSheetTitle") : t("addSheetTitle")} closeLabel={tc("close")}>
        {sheet?.mode === "add" && <ShortcodeForm heading={false} onCreated={(sc) => setSheet({ mode: "letter", shortcode: sc })} />}
        {sheet?.mode === "letter" && <AuthorizationStep key={sheet.shortcode.id} shortcode={sheet.shortcode} heading="compact" onSubmitted={closeSheet} />}
      </Sheet>

      {/* Invite Accountant / Team Member Sheet */}
      <Sheet
        open={inviteOpen}
        onClose={() => setInviteOpen(false)}
        title={t("inviteDialogTitle")}
        closeLabel={tc("close")}
      >
        <form onSubmit={handleSendInvite} className="space-y-4">
          <p className="text-sm text-ink-2 leading-relaxed">
            {t("inviteDialogLead")}
          </p>

          <div>
            <label className="block text-xs font-semibold uppercase text-muted mb-1.5">
              {t("inviteRole")}
            </label>
            <select
              value={inviteRole}
              onChange={(e) => setInviteRole(e.target.value as "accountant" | "staff" | "admin")}
              className="w-full rounded-r2 border border-hairline bg-paper px-3 py-2 text-sm text-ink focus:border-ochre focus:outline-none"
            >
              <option value="accountant">{t("roleAccountant")}</option>
              <option value="staff">{t("roleStaff")}</option>
              <option value="admin">{t("roleAdmin")}</option>
            </select>
          </div>

          <div>
            <label className="block text-xs font-semibold uppercase text-muted mb-1.5">
              {t("invitePhone")}
            </label>
            <input
              type="tel"
              value={invitePhone}
              onChange={(e) => setInvitePhone(e.target.value)}
              placeholder="0712 345 678"
              className="w-full rounded-r2 border border-hairline bg-paper px-3 py-2 font-mono text-sm text-ink placeholder:text-muted focus:border-ochre focus:outline-none"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold uppercase text-muted mb-1.5">
              {t("inviteEmail")}
            </label>
            <input
              type="email"
              value={inviteEmail}
              onChange={(e) => setInviteEmail(e.target.value)}
              placeholder="accountant@example.com"
              className="w-full rounded-r2 border border-hairline bg-paper px-3 py-2 font-mono text-sm text-ink placeholder:text-muted focus:border-ochre focus:outline-none"
            />
          </div>

          <div className="pt-2 flex justify-end gap-2">
            <Button
              type="button"
              variant="secondary"
              onClick={() => setInviteOpen(false)}
            >
              {tc("cancel")}
            </Button>
            <Button
              type="submit"
              loading={createInvite.isPending}
            >
              {t("sendInvite")}
            </Button>
          </div>
        </form>
      </Sheet>
    </>
  );
}

function Section({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section>
      <div className="flex items-end justify-between gap-3 border-b border-hairline pb-2">
        <h2>{title}</h2>
        {action}
      </div>
      <div className="mt-3">{children}</div>
    </section>
  );
}
