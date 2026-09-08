# ADR-0003: Zero custody — CiftPay never holds, routes or settles funds

## Status

Accepted — 2026-09-02

## Context

CiftPay sits between a merchant's M-Pesa shortcode (Till, Paybill or Pochi la Biashara) and KRA eTIMS. The product could in principle be built in two ways: as a payment intermediary that receives buyer funds and forwards them to the merchant, or as an observer that is notified of payments the merchant already received directly from Safaricom.

In Kenya, holding, routing or settling funds on behalf of others is regulated by the Central Bank of Kenya under the National Payment System Act 2011 and the National Payment System Regulations 2014. Operating as a Payment Service Provider requires CBK authorisation, capital requirements, trust-account arrangements and ongoing reporting. That licensing effort is measured in many months and would dominate a small team's runway before a single invoice is issued.

The product's actual value — turning a received payment into a KRA-acknowledged invoice, a reconciled ledger line and a buyer receipt — does not require touching the money. Safaricom's Daraja API already delivers C2B validation/confirmation and STK callbacks describing each payment that lands on the merchant's own shortcode.

## Decision

CiftPay is a zero-custody system. This is non-negotiable N1 in `plan.md`.

- CiftPay never holds, routes, settles, refunds or disburses funds. Money moves from buyer to merchant entirely within M-Pesa; CiftPay only receives Daraja callbacks (`POST /webhooks/daraja/c2b/validation/{token}`, `POST /webhooks/daraja/c2b/confirmation/{token}`, `POST /webhooks/daraja/stk/{token}`) and fiscalises what it observes.
- Shortcodes are always the merchant's own, registered and verified by the merchant. CiftPay does not operate aggregator shortcodes on behalf of merchants.
- STK push initiated by CiftPay (request-to-pay, shortcode verification) always targets the merchant's own shortcode as the receiving party; CiftPay is never the payee for merchant sales.
- Reversals are observed via Daraja callbacks and mirrored as credit notes; CiftPay does not initiate reversals of merchant funds.
- Subscription fees are paid by the merchant via STK push to CiftPay's own Paybill. These are CiftPay's revenue for a software service and are not custody of merchant or buyer funds. They are accounted for in `internal/billing`, not in the merchant ledger.
- Phase 3 financing is structured so that a CBK-licensed Digital Credit Provider lends and collects; CiftPay shares consented data and earns an origination fee. CiftPay never lends and never collects.
- Any future feature that would move funds — B2C or B2B disbursement, refunds, wallets, escrow, split payments, payouts to accountants — requires a new ADR, external legal review of the NPS Act position, and explicit sign-off before any endpoint is merged. Code review rejects disbursement endpoints that lack a referenced ADR.

## Consequences

### Positive

- CiftPay operates outside CBK Payment Service Provider licensing, which removes the largest regulatory barrier to launch and lets the team focus on eTIMS compliance and the Data Protection Act 2019.
- No trust accounts, float management, settlement reconciliation or chargeback liability.
- Merchants keep their existing shortcodes and settlement; onboarding is additive, with nothing to migrate.
- The security surface is smaller: a compromised CiftPay cannot redirect money, only misreport it, and misreporting is detectable against M-Pesa statements.
- Buyers and merchants can independently verify every receipt at `/r/<code>` against a payment they already see in M-Pesa.

### Negative

- CiftPay cannot offer payment features that competitors with PSP licences can (instant refunds, split settlement, wallet balances), which caps some monetisation paths.
- The product depends entirely on Daraja callback delivery; missed callbacks must be recovered by polling `TransactionStatus`, not by inspecting a balance CiftPay controls.
- Future disbursement-adjacent features carry a heavy process cost (ADR plus legal review) by design.
- The regulatory position must be re-validated if CBK or Parliament changes the NPS framework; `docs/compliance.md` owns that watch item.

## Alternatives considered

### Escrow or wallet model

Receive buyer payments into a CiftPay-controlled shortcode or wallet, fiscalise, then settle to the merchant. Rejected: this is squarely PSP activity under the NPS Act 2011 and NPS Regulations 2014, requires CBK authorisation and trust accounts, and adds settlement risk, float accounting and fraud liability without improving the core compliance outcome.

### Aggregator shortcode operated by CiftPay

Merchants sell through a CiftPay Paybill with per-merchant account numbers. Rejected for the same reasons as escrow; it also removes the merchant's direct relationship with Safaricom and makes CiftPay the counterparty on every transaction.
