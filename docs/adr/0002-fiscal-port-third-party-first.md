# ADR-0002: `fiscal.Provider` port, third-party integrator first, direct OSCU later

## Status

Accepted — 2026-09-02

## Context

Every invoice CiftPay produces must be registered with KRA eTIMS. KRA offers two integration modes for software vendors: direct system-to-system integration as an OSCU/VSCU, which requires a certification process with KRA, or integration through an already KRA-approved third-party integrator that exposes its own API and carries the certification.

Direct certification takes months and requires a working product to certify. Design partners need real eTIMS invoices within Phase 1 (weeks 2–8). At the same time, the fiscal call is the riskiest external dependency in the system: it times out, rate-limits, rejects malformed item codes and PINs, and its failure semantics drive the invoice state machine (`plan.md` section 9.5). Phase 4 also anticipates other East African fiscalisation regimes (Uganda EFRIS, Tanzania VFD) with the same shape: register device, submit invoice, submit credit note, look up item codes.

## Decision

Define a single port in `internal/fiscal` and keep every KRA-specific detail behind it:

```go
type Provider interface {
    RegisterDevice(ctx context.Context, org OrgFiscalProfile) (DeviceRef, error)
    SubmitInvoice(ctx context.Context, inv Invoice) (Ack, error)   // idempotent by inv.ID
    SubmitCreditNote(ctx context.Context, cn CreditNote) (Ack, error)
    LookupItemCodes(ctx context.Context, q string) ([]ItemCode, error)
    Health(ctx context.Context) error
}
```

- `SubmitInvoice` must be idempotent by `inv.ID`: resubmitting an already-acknowledged invoice returns the original `Ack` rather than creating a duplicate at KRA.
- Adapters return typed errors classified as retryable or terminal; the state machine in `internal/fiscal` never inspects vendor payloads.
- Ship adapters in this order:
  1. `mock` (Phase 0) — in-memory, deterministic, with failure injection (retryable / terminal / none) so the state machine, backoff and `NEEDS_REVIEW` flow are tested before any external account exists.
  2. `vendor` (Phase 1) — HTTP client for a KRA-approved eTIMS third-party integrator, configured by `FISCAL_VENDOR_BASE_URL` and `FISCAL_VENDOR_API_KEY`. This is the go-live path for the MVP.
  3. `oscu` (Phase 3) — direct KRA system-to-system implementation, followed by KRA certification; selectable per org via `fiscal_adapter` once certified.
- The active adapter is chosen by `FISCAL_ADAPTER` (`mock` | `vendor` | `oscu`) in the worker.
- A shared contract suite, `internal/fiscal/providertest`, runs against every adapter and asserts idempotency, error classification, credit-note linkage and health reporting. An adapter is not mergeable until it passes the suite.

## Consequences

### Positive

- Real eTIMS invoices for design partners in Phase 1 without waiting for KRA certification.
- The state machine, retries and merchant-facing failure handling are exercised end to end from Phase 0 using `mock`.
- Switching or adding a vendor is a new adapter plus `providertest`, not a rewrite of the invoice flow.
- The same port extends to Uganda EFRIS and Tanzania VFD in Phase 4 as further `Provider` implementations.
- Per-org adapter selection allows a gradual migration from `vendor` to `oscu` without a big-bang cutover.

### Negative

- The integrator's fee per invoice and its uptime sit on the MVP critical path; margin and SLA are outside CiftPay's control until `oscu` exists.
- Two production adapters (`vendor`, `oscu`) will coexist for a period and must both be kept green in `providertest`.
- The port must be the lowest common denominator of several fiscal regimes; vendor-specific capabilities (for example in-place invoice amendment) are exposed only as optional features, not on the core interface.
- Item-code catalogues differ between the integrator and KRA's own lists; `LookupItemCodes` results are cached per adapter and must be invalidated when switching.

## Alternatives considered

### Direct OSCU integration first

Build against KRA's system-to-system spec immediately. Rejected for the MVP: certification cannot begin without a functioning product and takes months, blocking Gate G1 (500 real ACKED invoices). It remains the Phase 3 target because it removes per-invoice vendor fees and a dependency on a third party's uptime.

### Hard-wired vendor client with no abstraction

Call the integrator's API directly from the invoice service. Rejected: it would make the mock-driven Phase 0 impossible, tie error handling to one vendor's payloads, and turn the Phase 3 move to OSCU (and any Phase 4 EAC adapter) into a rewrite of the money path rather than an additive change.
