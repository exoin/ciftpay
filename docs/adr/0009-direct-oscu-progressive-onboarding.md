# ADR-0009: Direct KRA OSCU adapter, per-merchant device provisioning, and progressive eTIMS onboarding

## Status

Accepted — 2026-09-17.

## Context

ADR-0002 planned three `fiscal.Provider` adapters in order: `mock` (Phase 0), `vendor` — a KRA-approved third-party integrator (Phase 1 go-live), and `oscu` — direct system-to-system integration with KRA (Phase 3), "selectable per org via `fiscal_adapter` once certified". CiftPay has now obtained its own direct KRA OSCU developer credentials, ahead of the original Phase 3 timeline, and needs the `oscu` adapter to exist for real.

KRA's OSCU protocol (KRA "Online Sales Control Unit Requirements & Communication Protocols" and the taxpayer sign-up guide) splits credentials into two tiers that do not fit the same shape as the `vendor` adapter's single API key:

- **CiftPay's own OSCU developer credentials** (`KRA_OSCU_CONSUMER_KEY` / `KRA_OSCU_CONSUMER_SECRET`) are global: one pair for the whole deployment, used as HTTP Basic auth on KRA's OAuth `client_credentials` grant (`GET {base}/v1/token/generate?grant_type=client_credentials`). CiftPay already holds a working pair in the sandbox.
- **Device identity is per merchant.** KRA's device-initialisation call (`selectInitOsdcInfo`) takes the taxpayer's own PIN (`tin`), a branch id (`bhfId`, `"00"` for a single-location business) and a **device serial** (`dvcSrlNo`). KRA checks that exact `(tin, bhfId, dvcSrlNo)` triple against its own registry and, only if it matches, returns a `cmcKey` that signs every later call for that device. Reusing another merchant's serial, or changing a merchant's serial later, does not work: it is either rejected outright or silently invalidates the previous `cmcKey`.

**The device serial is not something CiftPay can generate on a merchant's behalf.** Per KRA's own OSCU/VSCU sign-up guide, a taxpayer must sign up on the eTIMS taxpayer portal (sandbox: `https://etims-sbx.kra.go.ke`; production: KRA's equivalent production host) with their own KRA PIN, submit a **Service Request** selecting **OSCU**, and upload the eTIMS Commitment Form. KRA reviews the request and, on approval, issues the device serial tied to that taxpayer. This is a one-time, manual step on KRA's side that happens outside CiftPay entirely; CiftPay's job is to make it easy to find and then use once the merchant has it — not to skip it.

This creates two product problems if handled naively:

1. **Onboarding friction.** Asking for KRA OSCU details during the M-Pesa onboarding flow would block every merchant on a KRA portal round-trip they have not necessarily done yet, before they can even see a receipt.
2. **A live payment with nowhere fiscal to go.** A verified shortcode (ADR-0008) can start receiving M-Pesa confirmations before its org has a `cmcKey`. Silently dropping those, or blocking C2B ingestion entirely, both lose money-in-the-ledger accuracy CiftPay exists to provide.

## Decision

### Adapter

`internal/fiscal/oscu` implements `fiscal.Provider`:

- `Client.Token` — OAuth client-credentials GET, HTTP Basic `Base64(consumerKey:secret)`, cached until shortly before `expires_in`.
- `Client.Initialize` — `selectInitOsdcInfo`; returns `Info{TIN, TaxpayerName, BranchID, BranchName, DeviceID, SDCID, MRCNo, CmcKey}`.
- `Provider.RegisterDevice` wraps `Initialize` and returns a `fiscal.DeviceRef` whose `Raw` JSON (`device_id`, `branch_id`, `device_serial`, `cmc_key`, `sdc_id`, `mrc_no`) is the same opaque blob every adapter stores on `orgs.fiscal_profile` (`internal/fiscal/submitter.go`).
- `Provider.SubmitInvoice` / `SubmitCreditNote` need that `cmcKey` back at submission time, so `fiscal.Invoice`/`fiscal.CreditNote` gained a `DeviceProfile json.RawMessage` field, populated by `Submitter.buildDocument` from `org.FiscalProfile`. Other adapters ignore it. An org with no `cmcKey` yet gets `ConfigError{Code: "device_not_registered"}` (terminal), never a silent no-op.
- Exact sales-submission and item-classification field names are a best-effort mapping to KRA's published OSCU spec, like `internal/fiscal/vendor`'s own documented caveat; `RegisterDevice`'s shape is the one part checked against the terminology used consistently by KRA's own docs (`tin`, `bhfId`, `dvcSrlNo`, `cmcKey`, `resultCd`/`resultMsg`/`data`).
- A shared in-memory idempotency cache (`Provider.acks`, keyed by `inv.ID`) satisfies `internal/fiscal/providertest`'s idempotency requirement, since KRA's own API has no such concept. It does not survive a process restart; `fiscal_submissions` (already durable) is what actually prevents a duplicate submission to KRA in that case, same as every other adapter.

### Networking: the sandbox DNS override

`sbx.kra.go.ke` fails to resolve through some validating local resolvers (a DNSSEC misconfiguration on KRA's side, observed as SERVFAIL). `oscu.Client` accepts a `DNSResolver` UDP address (`KRA_OSCU_DNS_RESOLVER`, default `8.8.8.8:53` in the sandbox) and, only when it is non-empty, builds its own `*http.Transport` with a `net.Dialer` whose `Resolver.Dial` always connects to that address. This is deliberately narrow:

- It touches only this one `http.Client`'s transport — never the process's global resolver, never any other outbound call (Daraja, Africa's Talking).
- It is fully opt-out-able: an empty `KRA_OSCU_DNS_RESOLVER` uses the normal system resolver.
- It is a workaround for a specific sandbox host's DNS quirk, not a general design pattern. Production infrastructure should still run a resolver that answers `sbx`/production `kra.go.ke` hosts correctly; this override exists so that fact does not block a working integration today.

### Per-org configuration: Tax Settings

`orgs` gains `etims_status ∈ {unconfigured, initialized, failed}` (default `unconfigured`), `kra_bhf_id`, `kra_device_serial` (plain — a KRA-assigned identifier, not a secret) and `kra_cmc_key_enc` (envelope-encrypted like `kra_pin_enc`, since it is a signing credential). The org's own taxpayer PIN is **not** duplicated into a new column: `kra_pin_enc` already holds it (`orgs`, migration `0001`) and `ConfigureEtims` reads it back through the same decrypt path `Submitter.buildDocument` uses.

`POST /org/etims {kra_bhf_id?, kra_device_serial}` (`org.Service.ConfigureEtims`, owner/admin only):

1. Decrypts the org's own PIN, defaults `kra_bhf_id` to `"00"`.
2. Calls `fiscal.Provider.RegisterDevice`. On failure, `etims_status → failed` with `etims_failed_reason` (truncated, no `cmcKey` fragment) so the merchant can see what KRA said and fix their input.
3. On success, persists the branch id, device serial, encrypted `cmcKey` and the raw profile blob, sets `etims_status = initialized`, and the HTTP handler (not `org.Service`, to keep `org` free of a `ledger` dependency) calls `ledger.Service.ActivateTaxPending` to release anything held back per below.

The Tax Settings screen (`web`) is where a merchant enters `kra_bhf_id`/`kra_device_serial`; its copy explains, in plain language, that the device serial comes from KRA's own eTIMS taxpayer portal (Service Request → OSCU), not from CiftPay, with a link to that portal — the actual bottleneck is a manual KRA approval CiftPay cannot shortcut, so the product's job is to make the CiftPay-side step (typing the result in) take ten seconds once the merchant has it, and to be honest that the KRA-side step exists at all.

### Progressive onboarding

`invoices.state` gains `TAX_PENDING`, inserted between `DRAFT` and `QUEUED` in the state machine (`TAX_PENDING → QUEUED` only; nothing automatic leaves it, same as `NEEDS_REVIEW`). `ledger.Service.CreateInvoiceForSale` reads the org's `etims_status` and creates the invoice `QUEUED` (fiscal job enqueued) when `initialized`, or `TAX_PENDING` (no job, no KRA call) otherwise. The sale, payment and invoice rows all exist and are visible to the merchant throughout — only the KRA submission is withheld. `ledger.Service.ActivateTaxPending` moves every `TAX_PENDING` invoice for an org to `QUEUED` and enqueues its job in one transaction, called once by `ConfigureEtims`'s success path.

This decouples the two onboarding tracks completely: a merchant can connect M-Pesa (ADR-0008) and start seeing sales the same day, and configure KRA whenever their portal registration comes through, with no data loss or reconciliation step in between.

### Testing before any merchant is configured

`KRA_OSCU_TEST_PIN`, `KRA_OSCU_TEST_BHF_ID` (default `00`) and `KRA_OSCU_TEST_DEVICE_SERIAL` are a throwaway KRA sandbox developer-portal identity (obtained the same way any test taxpayer would, or KRA's own published sandbox test PIN if they publish one) for exercising the *live* sandbox — `ciftctl kra-init --token-only` first (validates `KRA_OSCU_CONSUMER_KEY`/`SECRET` and the DNS override alone), then `ciftctl kra-init --pin $KRA_OSCU_TEST_PIN --branch $KRA_OSCU_TEST_BHF_ID --serial $KRA_OSCU_TEST_DEVICE_SERIAL` once a real test device serial exists. `internal/fiscal/oscu`'s own test suite (`providertest.Run` plus classification/DNS tests) never touches the network at all, so it does not need any of this; the `_TEST_` variables exist purely for a human running a real connectivity check against KRA's sandbox before the first real merchant does it live.

## Consequences

### Positive

- Real direct-KRA invoices become possible without waiting on either the `vendor` integrator relationship or a big-bang cutover: `FISCAL_ADAPTER` still selects the adapter process-wide, and per-org `etims_status` is an orthogonal, additive gate.
- No merchant is blocked from seeing M-Pesa activity by KRA's own manual approval queue.
- The `cmcKey` gets the same encryption treatment as every other secret in the system; nothing new is stored in clear.

### Negative

- `TAX_PENDING` invoices need their own visibility in the merchant UI (Today/Invoices should read as "recorded, not yet filed with KRA", not as an error) — tracked as web follow-up, not a backend gap.
- The sales-submission and item-lookup wire shapes are provisional until reconciled against a real device (`kra_device_serial`) and a real submitted receipt; `RegisterDevice`/`Token` are the parts already exercised.
- Two credential tiers (global consumer key/secret, per-org device identity) is one more shape than `vendor`'s single API key, and the DNS override is one more piece of environment-specific configuration to carry into production deployment.

## Alternatives considered

**Ask CiftPay to mint its own device serial per org (e.g. a UUID) instead of a KRA-issued one.** Rejected: KRA's device initialisation checks the triple against its own registry; an unregistered serial is refused outright, it is not a bearer token CiftPay is free to issue.

**Block C2B ingestion for unconfigured orgs instead of `TAX_PENDING`.** Rejected: money already moved on M-Pesa; not recording it in the ledger because of a KRA registration step still pending would make CiftPay's core "see every shilling" promise depend on a KRA approval queue outside anyone's control.
