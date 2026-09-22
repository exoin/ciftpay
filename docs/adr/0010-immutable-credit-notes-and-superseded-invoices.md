# ADR-0010: Immutable Credit Note & Superseded Invoice Pointer Chain

## Status

Accepted — 2026-09-21.

## Context

In early architectural planning (`plan.md` and `docs/data-model.md`), updating an invoice (such as when a customer retroactively adds their KRA PIN to an anonymous receipt, or corrects buyer details) was loosely conceptualized as updating or re-submitting the existing invoice record.

However, KRA eTIMS / OSCU regulatory specifications strictly prohibit in-place mutation or duplicate resubmission of any invoice once acknowledged (`ACKED` with a KRA invoice number). Under Kenyan tax law:
1. Every fiscal receipt registered with KRA is legally final.
2. Any reduction, amendment, or cancellation of a registered invoice must be executed via an official Credit Note (`rcptTyCd = "R"` or `"C"`), which explicitly cites the original KRA invoice number (`orgInvcNo`) and specifies a valid refund reason code (`rfdRsnCd`, e.g. `"01"` for Incorrect Details / Cancellation).
3. The credit note lines must mathematically mirror the original sale lines.
4. If a replacement invoice is needed (e.g. re-issuing the sale with the buyer's PIN attached), a brand new invoice must be created and fiscalized independently.

Allowing in-place mutation or multiple re-amendments would lead to severe regulatory non-compliance, ledger desynchronization, and failed KRA audits.

## Decision

1. **Strict Immutability & Forward Pointer Chain**:
   - Invoices are strictly immutable once created.
   - Added schema columns to `invoices`:
     - `parent_invoice_id UUID REFERENCES invoices(id)`: Points to the prior invoice being amended or canceled.
     - `superseded_by_id UUID REFERENCES invoices(id)`: Points forward to the newly active replacement invoice.
     - `refund_reason_code TEXT`: Stores the mandatory KRA reason code (`"01"` default).
   - Migration `0005_superseded_invoices.sql` established these relationships and foreign keys.

2. **Terminal Non-Re-amendable States**:
   - Once an invoice has been amended or canceled, its `superseded_by_id` is set, or its kind is `CREDIT_NOTE`.
   - An amended invoice can **never** be amended again.
   - Both customer-facing (`/r/[code]`) and merchant-facing (`/invoices/[id]`) UIs lock down all amendment buttons for superseded invoices or credit notes. Instead, a prominent banner informs the user that a newer active version exists, with a direct link to view and manage the current active invoice.

3. **River Queue Un-parking & Recursive Submission**:
   - When a reissue or cancellation is triggered:
     - The original invoice is marked superseded.
     - A Credit Note is generated (`kind = 'CREDIT_NOTE'`) and queued for eTIMS submission (`state = 'QUEUED'`).
     - If reissuing with buyer details, a new replacement invoice is generated (`kind = 'INVOICE'`) and queued.
     - Any parked River queue jobs for the sale/invoice are unparked and re-enqueued, ensuring automatic fiscalization and customer SMS delivery without manual operator intervention.

## Consequences

### Positive
- 100% compliant with KRA eTIMS Tax Procedures Act Regulations.
- Complete, tamper-evident fiscal audit trail: the original invoice, the canceling credit note, and the active replacement invoice are permanently linked and inspectable.
- Eliminates race conditions and invalid duplicate tax filings.

### Negative
- A single amended transaction produces three database records (Original Invoice, Credit Note, Replacement Invoice), requiring UI views to clearly group or identify related documents.
