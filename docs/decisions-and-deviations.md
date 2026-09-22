# Architecture Decisions & Deviations from Original Plan

This document serves as the master registry of all architectural, product, and technical decisions made during the development of CiftPay that evolved, accelerated, or intentionally departed from the initial project phases and baseline plan (`plan.md`).

---

## Summary Matrix of Deviations

| Area | Initial Plan / Phase Baseline | Decision Taken / Realized Architecture | Motivation / Trigger | Reference |
|---|---|---|---|---|
| **eTIMS Integration Tier** | Phase 1 planned a third-party aggregator (`fiscal.Provider` vendor adapter). Direct KRA OSCU was slated for Phase 3. | Transitioned directly to **Direct KRA OSCU system-to-system** ahead of schedule in Phase 1. | Secured direct KRA OSCU developer credentials early; avoided per-invoice aggregator fees (KES 1–2/invoice) and third-party data custody. | [ADR-0009](adr/0009-direct-oscu-progressive-onboarding.md) |
| **Invoice Amendments & Buyer PINs** | In-place modification or re-filing of invoices when buyer details or PINs are added retroactively. | **Strict Immutability with Credit Notes & Forward Pointer Chain** (`parent_invoice_id`, `superseded_by_id`). Superseded invoices are permanently locked from further amendment. | KRA Tax Procedures Act regulations strictly forbid modifying or resubmitting an acknowledged invoice. Requires credit note cancellation (`rcptTyCd: "R"`, `rfdRsnCd: "01"`). | [ADR-0010](adr/0010-immutable-credit-notes-and-superseded-invoices.md) |
| **KRA Host & Endpoint Architecture** | Single base URL `https://sbx.kra.go.ke` assumed for all KRA OSCU requests. | **Dual-Origin Architecture**: OAuth token generator at `https://sbx.kra.go.ke`, while eTIMS OSCU endpoints reside at `https://etims-api-sbx.kra.go.ke`. | KRA splits authentication gateway from transaction RPC endpoints; requests to the wrong origin return 404 or HTML errors. | [ADR-0011](adr/0011-dual-origin-kra-gateway-and-live-verification.md) |
| **Testing & Mock Strategy** | Local development and testing frequently relied on mock fiscal adapters or optimistic UI simulation. | **Strict Zero-Mock Mandate in Production**: Real KRA handshake on device registration, surfacing actual KRA error codes (e.g. 901 `"It is not valid device"`). | Prevents dangerous false-compliance states where merchants believe their fiscal device is active when KRA has rejected it. | [ADR-0011](adr/0011-dual-origin-kra-gateway-and-live-verification.md) |
| **Manual Sales & Cash Notifications** | Manual sales ("Record Sale") were initially an offline bookkeeping fallback without SMS notifications. | **Unified 4-Stage Transaction Pipeline** (Ingest -> Ledger -> eTIMS -> SMS) with optional customer phone field on manual sales. | Customers paying cash or via bank transfers need immediate SMS receipt delivery with live public KRA verification links (`/r/[code]`). | [ADR-0012](adr/0012-unified-transaction-pipeline-and-customer-sms.md) |
| **Onboarding PIN Uniqueness** | Strict global database unique constraint on taxpayer PIN during initial sign-up step. | **Self-Healing Onboarding**: Differentiates between active/verified orgs and abandoned unverified sessions, allowing genuine owners to reclaim PINs without support intervention. | Prevented permanent account lockouts caused by users abandoning registration mid-stream. | [ADR-0013](adr/0013-self-healing-onboarding-pin-lockout.md) |
| **Payments Ledger Navigation** | Payments page was a read-only list with item conversion restricted only to unmatched rows. | **Full-Lifecycle Payment Inspection & Universal Search**: Search by code, name, phone hash, or account; `PaymentDetailSheet` with 1-click receipt copy, rule badges, and direct links to generated KRA invoices. | Merchants need instant access to customer payment verification, proof of tax compliance, and quick invoice navigation. | Commit `0ca71aa` |

---

## Detailed Context by Decision

### 1. Direct KRA OSCU in Phase 1 (ADR-0009)
* **What was planned:** In `docs/compliance.md` and `plan.md §9.2`, Phase 1 was scoped around an approved third-party eTIMS aggregator (the `vendor` adapter), with direct OSCU deferred to Phase 3.
* **What changed:** When direct KRA developer credentials and sandbox access were acquired, building on an aggregator introduced redundant vendor integration debt. CiftPay built the direct `oscu` adapter immediately, supporting global OAuth developer credentials alongside merchant-specific device provisioning (`tin`, `bhfId`, `dvcSrlNo`, `cmcKey`).

### 2. Credit Note & Re-issue Pointer Chain (ADR-0010)
* **What was planned:** Retroactive buyer PIN claims were initially modeled as an update to the invoice.
* **What changed:** KRA eTIMS strictly treats every acknowledged invoice (`ACKED`) as legally immutable. To add a buyer PIN or cancel an invoice, the system creates an immutable Credit Note citing the original invoice number and exact refund reason (`"01"`), and generates a new replacement invoice with a pointer chain (`superseded_by_id`, `parent_invoice_id`). Furthermore, once superseded, neither buyer nor merchant can amend the invoice again; the UI routes them to the active replacement.

### 3. Dual-Origin KRA Gateway & Live Handshake Verification (ADR-0011)
* **What was planned:** Single configuration variable `KRA_OSCU_BASE_URL` and optimistic UI states.
* **What changed:** 
  1. Token generation origin (`sbx.kra.go.ke`) was separated from the eTIMS API origin (`etims-api-sbx.kra.go.ke`), with dynamic handling for KRA's string-encoded `"expires_in": "3599"`.
  2. The tax settings connection was hardened against mock approvals: invalid device serials hit KRA's live endpoint, return real error code 901 (`"It is not valid device"`), and fail cleanly with HTTP 422, preserving ledger accuracy.

### 4. Unified Manual & Webhook Transaction Pipeline (ADR-0012)
* **What was planned:** Focus primarily on Lipa na M-Pesa C2B callbacks; manual sales did not deliver SMS receipts.
* **What changed:** Merged both into an identical pipeline: line-item arithmetic validation (`totAmt = taxblAmt + taxAmt`), ledger write, eTIMS queue, and SMS receipt queue. Added optional customer phone input to the "Record a Sale" sheet.

### 5. Self-Healing Onboarding & PIN Lockout Prevention (ADR-0013)
* **What was planned:** Hard uniqueness on KRA PIN on registration.
* **What changed:** Abandoned unverified attempts no longer permanently lock out genuine business owners. The onboarding flow allows authenticated owners to resume their organization or clear stale unverified attempts.
