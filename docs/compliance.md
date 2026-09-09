# CiftPay — Compliance & Regulatory Posture

This document explains *why CiftPay can operate without a payments licence*, *how it stays inside KRA's eTIMS rules*, and *what it owes under data-protection law*. It is written for engineers and for counsel; the engineering consequences of each point are called out explicitly. It is not legal advice; every item marked **[counsel]** needs sign-off from a Kenyan advocate before Phase 1 go-live.

## 1. Zero custody — outside the payments perimeter

**Law:** National Payment System Act 2011 (NPS Act) and NPS Regulations 2014 require a Central Bank of Kenya (CBK) authorisation for *payment service providers* — entities that issue payment instruments, operate e-money, or hold/transfer funds on behalf of others.

**CiftPay's position:** CiftPay never *holds*, *routes*, *settles* or *instructs* movement of customer funds.

| What CiftPay does | What it does not do |
|---|---|
| Receives Daraja **C2B confirmation callbacks** (read-only notifications that Safaricom has already settled funds to the merchant) | Receive funds into any CiftPay-controlled account on behalf of merchants |
| Initiates **STK push requests** *to the merchant's own shortcode* (the merchant is payee; CiftPay only triggers the prompt) | Initiate B2C / B2B disbursements from merchant funds |
| Reads reversal notifications | Perform or approve reversals |
| Collects its **own** subscription fees to **CiftPay's own** Paybill | Hold a float, e-money or trust account |

The STK push point deserves care: in Daraja "Lipa na M-Pesa Online" the `BusinessShortCode`/`PartyB` is the **merchant's** shortcode registered under the merchant's Daraja app *or* under CiftPay's app with the merchant's shortcode onboarded as a sub-merchant. In the second case Safaricom settles to the merchant; CiftPay is a technology provider (analogous to how POS vendors and e-commerce platforms integrate). **[counsel]** confirm the sub-merchant model does not amount to "payment gateway" services requiring PSP authorisation; the fallback is to require each merchant to own their Daraja app and share credentials (heavier onboarding, zero doubt).

**Engineering consequences (ADR-0003):**
- No code path may call Daraja B2C, B2B, or Reversal *initiation* APIs. `internal/mpesa` exposes only OAuth, C2B RegisterURL, STK Push, Transaction Status, and callback parsing.
- No table stores a "balance" for a merchant. `payments` are facts, not ledger balances CiftPay is liable for.
- Adding any disbursement feature requires a new ADR and this section updated.

## 2. KYC: inherited identity, verified control

CiftPay does **not** perform primary KYC. Both counterparties have already been identified by regulated actors:

| Fact | Who verified it | How CiftPay relies on it |
|---|---|---|
| Owner of the M-Pesa shortcode | Safaricom (CBK-regulated; KYC + business registration for Till/Paybill) | **Administrative Gate** ([ADR-0008](adr/0008-administrative-gate.md)): the merchant uploads a signed and stamped authorization letter; Safaricom checks the signatory against the shortcode's KYC before mapping it to CiftPay's Daraja app; a CiftPay operator then records the outcome (`mpesa_shortcodes.status = 'verified'`). CiftPay performs no payment-based check and holds no merchant Daraja credentials |
| Holder of the KRA PIN | KRA (PIN issuance) | Format check `^[AP]\d{9}[A-Z]$` + iTax PIN checker lookup returning taxpayer name; stored `kra_pin_verified_at` |
| Phone ownership | Safaricom SIM registration | OTP to the MSISDN |
| Business identity for eTIMS | KRA, during eTIMS onboarding through the integrator | `RegisterDevice` result stored in `orgs.fiscal_profile` |

Rules:
- A shortcode may be **verified by one org only** (partial unique index `WHERE status = 'verified'`). A second claim on a verified number returns `409 shortcode_claimed`; several orgs may be *pending* on one number until Safaricom's answer decides.
- No C2B confirmation reaches a tax ledger unless the shortcode is `verified`; other callbacks are parked in `webhook_events` and acknowledged.
- Verification expires if RegisterURL callbacks stop arriving for 90 days (Phase 2).
- CiftPay keeps the evidence of ownership — the uploaded letter (`authorization_letter_path`, admin-only access) and the `shortcode.verified` audit row naming the operator — for the life of the account.

**AML note [counsel]:** CiftPay is not a "reporting institution" under the Proceeds of Crime and Anti-Money Laundering Act because it does not handle funds; nonetheless the admin back-office flags anomalous patterns (e.g. one MSISDN paying > 50 merchants/day) for the merchant's information only.

## 3. eTIMS — operating through approved channels

**Law:** Tax Procedures Act (Electronic Tax Invoice) Regulations 2023–2024; Finance Act 2023 s. 23 (non-deductibility of expenses without eTIMS invoices from 1 Jan 2024). KRA offers: eTIMS Lite (web/USSD), eTIMS Client software, **OSCU** (Online Sales Control Unit — system-to-system, real-time), **VSCU** (Virtual SCU — for offline/batch), and integration through **KRA-approved third-party integrators**.

**Phase 1–2 (vendor adapter):** CiftPay submits invoices through a KRA-approved third-party integrator's API. The integrator holds the OSCU/VSCU certification; CiftPay is a software client. Each merchant is onboarded with the integrator under **the merchant's own PIN** (`RegisterDevice`), so the KRA invoice is legally issued *by the merchant*, not by CiftPay.

Selection criteria for the integrator (checklist for §9.2 of `plan.md`):
1. Listed on KRA's published register of approved eTIMS third-party integrators.
2. REST API with sandbox; per-invoice idempotency (client reference) and a "fetch by client reference" endpoint.
3. Item classification code search endpoint (needed for the guided picker).
4. Supports credit notes and buyer PIN on invoices.
5. Pricing: per-invoice fee ≤ KES 2 at 100k invoices/month or flat.
6. Data-processing agreement compatible with §4 below.

**Phase 3 (OSCU adapter):** CiftPay applies for its own system-to-system integrator certification with KRA and implements `internal/fiscal/oscu`. The product does not change; `FISCAL_ADAPTER` is switched per org after migration.

**Engineering consequences (ADR-0002):**
- Invoice content follows KRA's mandatory fields: seller PIN, seller name, invoice number (KRA-assigned), date/time, item description, item classification code, quantity, unit price, tax category (A/B/C/D/E), tax amount, total, buyer PIN (optional), QR/signature.
- Every submission attempt is recorded in `fiscal_submissions` with request/response for 7 years.
- Invoice numbers displayed to users are **KRA's**, never CiftPay's internal IDs.
- Credit notes reference the original KRA invoice number.
- Time on the invoice is East Africa Time; CiftPay must not back-date beyond what the integrator/KRA allows (typically same day) — late payments create a same-day invoice with the payment reference in the description.

## 4. Data protection — Data Protection Act 2019 (DPA)

**Roles:** CiftPay is a **data controller** for merchant users (accounts, sessions) and a **data processor** for merchants regarding their customers' data (payer MSISDN, buyer PIN). The eTIMS integrator and Africa's Talking are sub-processors. **[counsel]** confirm the controller/processor split in the Terms and the DPA schedule.

Obligations and how they are met:

| Obligation | Implementation |
|---|---|
| Registration with the ODPC (s. 18) | File before Phase 1 go-live; certificate number shown in the privacy notice |
| Lawful basis | Merchant: contract. Buyer MSISDN/PIN: legitimate interest + legal obligation (tax invoice) — documented in the LIA |
| Data minimisation | Only MSISDN, name (as sent by Daraja) and optional PIN are stored about buyers; no address, no ID number |
| Security (s. 41) | Envelope encryption of MSISDN/PIN (`internal/platform/crypto`), TLS everywhere, RLS tenancy, audit log, secrets in env |
| Retention | Table in [`data-model.md §6`](data-model.md#6-retention); tax records 7 years, buyer MSISDN on notifications 90 days |
| Data subject rights (ss. 26–40) | `DSAR` endpoint (Phase 1): lookup by hashed MSISDN, export or erase non-tax-record data; tax records are exempt from erasure (legal obligation) but access is honoured |
| Privacy notice | Linked from every `/r/<code>` page footer and the login screen, EN/SW |
| Breach notification (s. 43) | Runbook: notify ODPC within 72 h; template in `docs/runbooks/` (Phase 1) |
| Cross-border transfer (s. 48) | Fly.io region for Postgres chosen in Africa/EU with adequate safeguards; **[counsel]** confirm SCC-equivalent terms |

Engineering rules: never log MSISDN/PIN; list endpoints return masked values; buyer PIN is shown on the receipt page only as `A•••••••••B` unless the buyer-authenticated view (Phase 2).

## 5. Consumer protection & messaging

- SMS receipts are transactional (not marketing) under Communications Authority rules; the sender ID is registered; each message includes the merchant name and a link only to `ciftpay.co.ke`.
- The CTA on the receipt page is a link, not an SMS opt-in; no marketing SMS without explicit opt-in.
- Swahili and English available for every notification template.

## 6. Lending (Phase 3) — never on CiftPay's balance sheet

**Law:** CBK (Digital Credit Providers) Regulations 2022 require licensing to provide digital credit.

**Model:** CiftPay computes a cash-flow score from reconciled, merchant-consented data and lists offers from **CBK-licensed DCP partners**. The partner underwrites, disburses to the merchant's M-Pesa, and collects. CiftPay earns a referral/origination fee and never touches loan funds or repayments.

Guardrails: explicit merchant opt-in per data share; score explainability shown to the merchant; no adverse-action decisions taken by CiftPay; partner contract forbids re-use of data beyond the application.

## 7. Tax status of CiftPay itself

CiftPay is a Kenyan company, VAT-registered; subscription invoices to merchants are themselves issued through eTIMS (dog-fooding via the same pipeline: the subscription payment is a payment on CiftPay's own org). Digital Service Tax considerations apply if the entity is non-resident — the plan assumes a Kenyan entity.

## 8. Checklist before Phase 1 go-live

- [ ] ODPC registration filed; certificate number recorded
- [ ] Privacy notice (EN/SW) live on `/r/<code>` and login
- [ ] Terms of service + Data Processing Agreement for merchants **[counsel]**
- [ ] Zero-custody memo signed off **[counsel]** (this doc §1 + ADR-0003)
- [ ] Integrator contract with sub-processor terms
- [ ] Safaricom Daraja Go-Live for the design partners' shortcodes
- [ ] Breach-notification runbook and on-call rota
- [ ] Sender ID registered with Africa's Talking / Communications Authority
