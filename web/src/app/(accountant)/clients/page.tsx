"use client";

import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import {
  Building2,
  Mail,
  CheckCircle2,
  XCircle,
  ArrowUpRight,
  AlertCircle,
  RefreshCw,
  Clock,
} from "lucide-react";
import { Money } from "@/components/ui/Money";
import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/Toast";
import {
  useAccountantInvites,
  useAcceptAccountantInvite,
  useRejectAccountantInvite,
  useAccountantClients,
} from "@/lib/api/queries";
import { switchOrg } from "@/lib/auth";

export default function AccountantClientsPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();

  // Queries
  const { data: invitesData } = useAccountantInvites();
  const { data: clientsData, isPending: isClientsPending, refetch: refetchClients } = useAccountantClients();

  // Mutations
  const acceptMutation = useAcceptAccountantInvite();
  const rejectMutation = useRejectAccountantInvite();

  const pendingInvites = invitesData?.data ?? [];
  const clients = clientsData?.data ?? [];

  async function handleAccept(id: string, orgName: string) {
    try {
      await acceptMutation.mutateAsync(id);
      toast.push(`Joined ${orgName} workspace successfully.`);
      void refetchClients();
    } catch (err: unknown) {
      toast.push(err instanceof Error ? err.message : "Failed to accept invite", "error");
    }
  }

  async function handleReject(id: string, orgName: string) {
    try {
      await rejectMutation.mutateAsync(id);
      toast.push(`Declined invitation from ${orgName}.`);
    } catch (err: unknown) {
      toast.push(err instanceof Error ? err.message : "Failed to decline invite", "error");
    }
  }

  function handleOpenWorkspace(orgId: string) {
    switchOrg(qc, orgId);
    router.push(`/client/${orgId}`);
  }

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="font-display text-2xl font-bold tracking-tight text-ink">
            Client Organizations
          </h1>
          <p className="text-sm text-ink-2">
            Multi-tenant accounting portal. Reconcile ledgers and download KRA iTax VAT return CSVs.
          </p>
        </div>
      </div>

      {/* Pending Invitations Banner / Inbox */}
      {pendingInvites.length > 0 && (
        <section id="invites" className="rounded-r2 border border-ochre/40 bg-ochre/5 p-4 sm:p-5">
          <div className="flex items-center gap-2 mb-3">
            <Mail className="size-5 text-ochre" />
            <h2 className="text-base font-semibold text-ink">
              Pending Client Invitations ({pendingInvites.length})
            </h2>
          </div>
          <p className="text-xs text-ink-2 mb-4">
            The following businesses have invited you as their authorized tax agent or accountant.
          </p>

          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {pendingInvites.map((inv) => (
              <div
                key={inv.id}
                className="flex flex-col justify-between rounded-r2 border border-hairline bg-paper p-4 shadow-sm"
              >
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="font-semibold text-ink truncate">{inv.org_name}</span>
                    <span className="rounded bg-paper-2 px-1.5 py-0.5 font-mono text-[11px] font-semibold text-muted uppercase">
                      {inv.role}
                    </span>
                  </div>
                  <div className="text-xs text-muted">
                    KRA PIN: <span className="font-mono font-medium text-ink">{inv.kra_pin || "Not configured"}</span>
                  </div>
                  <div className="text-[11px] text-muted flex items-center gap-1">
                    <Clock className="size-3" />
                    <span>Received {new Date(inv.created_at).toLocaleDateString()}</span>
                  </div>
                </div>

                <div className="mt-4 flex items-center gap-2 pt-2 border-t border-hairline">
                  <Button
                    size="sm"
                    className="flex-1 text-xs"
                    loading={acceptMutation.isPending}
                    onClick={() => handleAccept(inv.id, inv.org_name)}
                  >
                    <CheckCircle2 className="mr-1 size-3.5" />
                    Accept
                  </Button>
                  <Button
                    size="sm"
                    variant="secondary"
                    className="text-xs text-danger hover:bg-danger/10"
                    loading={rejectMutation.isPending}
                    onClick={() => handleReject(inv.id, inv.org_name)}
                  >
                    <XCircle className="mr-1 size-3.5" />
                    Decline
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Client Table Section */}
      <section className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold text-ink">
            Active Clients ({clients.length})
          </h2>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => void refetchClients()}
            title="Refresh clients"
          >
            <RefreshCw className="size-3.5 text-muted" />
          </Button>
        </div>

        {isClientsPending ? (
          <div className="rounded-r2 border border-hairline bg-paper p-8 text-center text-sm text-muted">
            Loading client accounts and calculating real-time VAT liability...
          </div>
        ) : clients.length === 0 ? (
          <div className="rounded-r2 border border-dashed border-hairline bg-paper p-12 text-center">
            <Building2 className="mx-auto size-10 text-muted mb-3" />
            <h3 className="font-semibold text-ink">No client businesses yet</h3>
            <p className="mt-1 text-sm text-ink-2 max-w-sm mx-auto">
              Ask merchants to invite your registered phone number or email from their CiftPay business settings.
            </p>
          </div>
        ) : (
          <div className="overflow-hidden rounded-r2 border border-hairline bg-paper shadow-sm">
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="border-b border-hairline bg-paper-2 text-xs font-semibold uppercase text-muted">
                  <tr>
                    <th scope="col" className="px-4 py-3 sm:px-6">Business Name</th>
                    <th scope="col" className="px-4 py-3">KRA PIN</th>
                    <th scope="col" className="px-4 py-3 text-right">Current Month Gross</th>
                    <th scope="col" className="px-4 py-3 text-right">Est. VAT Liability</th>
                    <th scope="col" className="px-4 py-3 text-center">eTIMS Sync Health</th>
                    <th scope="col" className="px-4 py-3 text-right sm:px-6">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-hairline font-sans">
                  {clients.map((client) => {
                    const healthTone =
                      client.etims_sync_health === "healthy"
                        ? "bg-green/10 text-green border-green/20"
                        : client.etims_sync_health === "action_required"
                        ? "bg-danger/10 text-danger border-danger/20"
                        : client.etims_sync_health === "pending_sync"
                        ? "bg-ochre/10 text-ochre border-ochre/20"
                        : "bg-paper-2 text-muted border-hairline";

                    const healthLabel =
                      client.etims_sync_health === "healthy"
                        ? "Healthy"
                        : client.etims_sync_health === "action_required"
                        ? "Needs Review"
                        : client.etims_sync_health === "pending_sync"
                        ? "Pending Sync"
                        : "Unconfigured";

                    return (
                      <tr key={client.org_id} className="hover:bg-paper-2/40 transition-colors">
                        <td className="px-4 py-4 sm:px-6">
                          <button
                            type="button"
                            onClick={() => handleOpenWorkspace(client.org_id)}
                            className="font-semibold text-ink hover:text-green text-left flex items-center gap-1.5"
                          >
                            <span>{client.name}</span>
                          </button>
                        </td>
                        <td className="px-4 py-4 font-mono text-xs text-ink-2">
                          {client.kra_pin || "—"}
                        </td>
                        <td className="px-4 py-4 text-right font-mono">
                          <Money cents={client.current_month_gross_cents} />
                        </td>
                        <td className="px-4 py-4 text-right font-mono font-medium text-ink">
                          <Money cents={client.estimated_vat_cents} />
                        </td>
                        <td className="px-4 py-4 text-center">
                          <span
                            className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-semibold ${healthTone}`}
                          >
                            {client.etims_sync_health === "healthy" ? (
                              <CheckCircle2 className="size-3" />
                            ) : client.etims_sync_health === "action_required" ? (
                              <AlertCircle className="size-3" />
                            ) : (
                              <Clock className="size-3" />
                            )}
                            {healthLabel}
                          </span>
                        </td>
                        <td className="px-4 py-4 text-right sm:px-6">
                          <Button
                            size="sm"
                            variant="secondary"
                            onClick={() => handleOpenWorkspace(client.org_id)}
                            className="text-xs"
                          >
                            Open Workspace
                            <ArrowUpRight className="ml-1 size-3.5" />
                          </Button>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}
