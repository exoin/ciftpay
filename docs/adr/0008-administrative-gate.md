# ADR-0008: The Administrative Gate — shortcode ownership is proven by Safaricom's paperwork, not by a payment

## Status

Accepted — 2026-09-09. Supersedes the own-Till KES 1 control check shipped under `plan.md` §4.1 on 2026-09-08 (migration `0002`, `POST /shortcodes/{id}/verify`).

## Context

A merchant connects a Till or Paybill to CiftPay (Pochi la Biashara is excluded: Daraja C2B RegisterURL does not cover it, so no confirmation could ever arrive) so that Daraja C2B confirmations for that number become sales, KRA invoices and receipts in *their* ledger. Before any payment is fiscalised CiftPay must know, with certainty, which organisation owns the number: attributing a payment to the wrong org means issuing a KRA invoice under the wrong PIN.

The first implementation asked the merchant to pay **KES 1 to their own shortcode** from their phone and matched the C2B confirmation on `{shortcode, msisdn, amount}`. Two problems became clear once it was run against the sandbox and thought through for production:

1. **It proves the wrong thing.** Anyone can send KES 1 to a Buy Goods Till. Paying Naivas proves you are a *customer* of Naivas, not its owner. A fraudster could register, type Naivas's Till, pay KES 1 in the shop, and CiftPay would hand them Naivas's tax ledger.
2. **It cannot even succeed in production.** Safaricom only delivers C2B callbacks for shortcodes that are *mapped to the calling Daraja app*. For an unmapped Till the KES 1 reaches the merchant and no callback ever reaches CiftPay, so the challenge expires — for the honest merchant too. The sandbox hid this because `600000` is pre-mapped to every sandbox app.

Alternatives considered and rejected:

- **Reverse micro-payment (CiftPay pays the Till via B2B, merchant types the receipt code from Safaricom's SMS).** Sound in principle, but CiftPay has no production shortcode with B2B enabled, it costs money per attempt, and it adds an outbound-payment code path to a system whose whole regulatory posture is "never initiates transactions" (ADR-0003).
- **Merchant-owned Daraja app / passkey / portal login.** Would give day-one connectivity for technical merchants, but requires storing merchant Safaricom credentials. Rejected: zero-credentials is a hard rule.
- **Statement screenshot or CSV upload reviewed by ops.** Forgeable, no Safaricom involvement, and ops would be doing KYC we are not equipped for.

What Safaricom actually does when a merchant asks them to map a shortcode to a third party's Daraja app: the merchant signs an **authorization letter** (business name, KRA PIN, shortcode, the Daraja app to map to, the signatory's ID) and Safaricom **verifies the signatory against the shortcode's KYC records** before mapping it and turning the callbacks on. That check is exactly the ownership proof CiftPay needs, performed by the only party who can perform it.

## Decision

Ownership and connectivity are settled together by Safaricom's Go-Live process; CiftPay performs **no** API-based ownership proof and holds **no** merchant credentials. A CiftPay operator records Safaricom's answer. This is the Administrative Gate.

- `mpesa_shortcodes.status ∈ {pending_authorization, verified, rejected}`, default `pending_authorization`. `verified_at` is set iff `status = 'verified'` (CHECK constraint). At most one `verified` row per number (partial unique index); several orgs may hold `pending_authorization` rows for the same number until Safaricom's answer decides.
- The merchant's only step: **`POST /shortcodes/{id}/authorization`** uploads a photo or PDF of the signed and stamped *CiftPay Safaricom Authorization Letter* (pre-filled and printable from the PWA at `/onboarding/letter`). The file lives under `UPLOAD_DIR` (`internal/platform/storage`), is never served publicly, and is readable only via `GET /admin/shortcodes/{id}/authorization`.
- Operators forward letters to Safaricom and, when Safaricom confirms the mapping, call **`PATCH /admin/shortcodes/{id}/verify`** (or `ciftctl verify-shortcode`). That writes `shortcode.verified` to the audit log and only then calls Daraja `RegisterURL` for the number. `PATCH /admin/shortcodes/{id}/reject {reason}` records a refusal; the merchant sees the reason and a corrected upload returns the row to the queue.
- **Only `verified` shortcodes resolve in the C2B ingest path** (`ResolveShortcode … AND status = 'verified'`). A confirmation for any other number is stored in `webhook_events`, marked processed with `no verified shortcode <n>`, acknowledged with 200 so Daraja does not retry, and creates no payment, sale or invoice. This is the single gate between Safaricom's callbacks and a tax ledger.
- Removed: `shortcode_verifications`, `POST /shortcodes/{id}/verify`, the `verify` STK purpose, the unconfigured-Daraja auto-verify shortcut, and `DARAJA_PASSKEY`. `mpesa.Client.STKPush` returns `ErrSTKNotConfigured` until Phase 2 request-to-pay (§5.1) brings a passkey for CiftPay's own Paybill. In `APP_ENV=production` the api refuses to start without Daraja consumer credentials.
- The merchant is not blocked while pending: manual sales, items, staff, reports all work; the shell shows the shortcode as *Pending authorization* and Settings lets them re-upload.

## Consequences

- **Correctness.** No payment reaches a ledger unless Safaricom has confirmed the owner. The impostor scenario is impossible by construction: their row can never become `verified` while the real owner's is, and until it is verified their row is invisible to ingest.
- **Zero custody, zero credentials preserved** (ADR-0003): CiftPay initiates no transactions and stores nothing about the merchant's M-Pesa account beyond the number and a photo of a letter.
- **Latency.** Connecting a Till now takes Safaricom's 1–3 business days instead of thirty seconds. This is the true cost of the product and was always going to be paid at Go-Live; the KES 1 flow only hid it. Onboarding copy says so honestly.
- **Ops load.** Each design partner is a letter batched to Safaricom and one `verify` call. `GET /admin/shortcodes` is the queue; a `/ops` UI is Phase 1 §4.6 work. CiftPay must also secure its own production Go-Live and a technology-partner arrangement with Safaricom so multi-shortcode mapping by letter is routine (`plan.md` §9.2).
- **Security of the letter.** It contains a KRA PIN and an ID number. It is stored on local disk (compose volume) behind an admin-only endpoint; moving to object storage means implementing `storage.Store` once. Retention policy is a follow-up.
- **Sandbox testing** no longer exercises "verification" at all: `make verify-shortcode SHORTCODE=600000` flips the row, `make sandbox-c2b` proves the C2B pipe.
