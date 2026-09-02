# CiftPay — Product

## 1. One-liner

> Get paid on M-Pesa like you always have. CiftPay issues the KRA invoice, sends the receipt to your customer, and keeps your books — before you've put your phone down.

**C.I.F.T.** = *Compliant Invoicing & Fund Tracking*. "Cift" also reads as *sift*: it sifts raw M-Pesa noise into compliant books.

CiftPay is **not** a wallet and **not** a payment gateway. Money moves Safaricom → merchant exactly as today. CiftPay is the compliance and intelligence layer on top of that rail.

## 2. The Pain Hunt Tournament

Every candidate had to pass four filters: (1) not a wallet / not a crypto gateway; (2) CiftPay never holds customer funds (no CBK PSP licence on day one); (3) the pain is *forced* on the customer by an external actor — regulator, buyer or platform — not "nice to have"; (4) a data or integration moat a copycat cannot clone in a weekend.

Scoring 1–5 on: Pain intensity · Willingness to pay · Regulatory tailwind · Technical viability via third-party rails · Uniqueness · Data moat.

| # | Candidate | Pain | WTP | Reg. | Viab. | Uniq. | Moat | Total | Verdict |
|---|---|---|---|---|---|---|---|---|---|
| A | **eTIMS Compliance Rail** — every M-Pesa Till/Paybill/Pochi payment becomes a KRA eTIMS invoice + reconciled ledger | 5 | 5 | 5 | 4 | 4 | 5 | **28** | **Winner** |
| C | Micro-employer statutory payroll (PAYE/NSSF/SHIF/Housing Levy) via M-Pesa | 4 | 2 | 4 | 4 | 3 | 3 | 20 | Runner-up → Phase 4 module |
| F | Gig-worker income passport from M-Pesa statements | 3 | 2 | 3 | 4 | 4 | 4 | 20 | Absorbed → Phase 3 cash-flow score |
| B | Social-commerce escrow for IG/WhatsApp/TikTok sellers | 4 | 3 | 2 | 3 | 3 | 3 | 18 | Eliminated: custody ⇒ PSP licence |
| G | Landlord / estate Paybill reconciliation | 3 | 3 | 2 | 5 | 2 | 2 | 17 | Absorbed → vertical template |
| D | Matatu SACCO cashless fares | 4 | 2 | 2 | 2 | 3 | 3 | 16 | Eliminated: distribution graveyard (BebaPay) |
| E | Chama / SACCO treasury OS | 3 | 2 | 2 | 5 | 1 | 2 | 15 | Eliminated: crowded, low WTP |

### Why A wins — the forcing function

- **Finance Act 2023 → from 1 January 2024 any business expense not supported by a KRA eTIMS invoice is non-deductible.** Corporate and SME buyers now refuse suppliers who cannot issue eTIMS invoices. Millions of M-Pesa-first micro-businesses (mama mboga, hardware, salons, transporters, freelancers) are being pushed out of B2B supply chains.
- KRA's own tools (eTIMS Lite web/app, USSD `*222#`) are manual, one invoice at a time, and disconnected from where money arrives.
- The pain is **double-sided**: the seller must issue, the buyer must receive. Every invoice is an acquisition event.
- **Zero custody**: CiftPay listens to Daraja callbacks; it never touches the money.
- **Moat**: the reconciled payment ↔ invoice ↔ KRA-ack graph is the cleanest cash-flow dataset on Kenyan SMEs that exists.

## 3. Value proposition by persona

| Persona | Pain today | With CiftPay |
|---|---|---|
| **Wanjiru — duka / mama mboga owner** (Till, 50–300 payments/day, Android Go phone) | B2B customers (restaurants, offices) ask for eTIMS invoices; she cannot re-type each payment into eTIMS Lite | Every Till payment auto-fiscalised; she sees a "Today" strip and a short Needs-attention list |
| **Otieno — B2B buyer / restaurant manager** | Pays suppliers on M-Pesa, gets no deductible invoice; his accountant rejects the expense | Receives an SMS with an eTIMS receipt link seconds after paying; PIN attached on request |
| **Amina — freelance accountant with 20 SME clients** | Collects M-Pesa statements and retypes invoices at month end | One portal; every client's income already reconciled and fiscalised; VAT3 pack in one export |
| **Kamau — hardware wholesaler (Paybill, several staff)** | Staff issue paper receipts; KRA compliance depends on one person | Paybill BillRef matches open sales; staff roles; multi-outlet in Phase 4 |

## 4. Core transaction loop

1. Customer pays the merchant's Till / Paybill / Pochi (or a CiftPay request-to-pay link / QR from Phase 2).
2. Daraja C2B confirmation → CiftPay ingests idempotently (`TransID`), normalises, and matches: BillRef → open sale; amount + phone within a 10-minute window → pending STK request; otherwise a **cash sale** with the shortcode's default item.
3. Worker builds the fiscal invoice (items, tax category, buyer PIN if known) and submits through the `fiscal.Provider` port (KRA-approved third-party integrator at MVP).
4. KRA acknowledgement (invoice number, signature, QR) stored; buyer gets SMS/WhatsApp with `ciftpay.co.ke/r/<code>`.
5. Ledger, VAT position and monthly return draft update in real time; failures land in **Needs attention** with retry/backoff.

## 5. Business model

| Tier | Price (KES / month) | Includes |
|---|---|---|
| **Hustler** (free) | 0 | 1 shortcode, ≤ 30 invoices/month, SMS receipts, receipt verify page |
| **Duka** | 1,500 | ≤ 500 invoices, WhatsApp receipts, CSV/XLSX exports, request-to-pay links |
| **Biashara** | 4,500 | Unlimited invoices, multi-outlet, accountant seat, API keys |
| **Accountant** | 9,000 | Up to 25 client orgs, bulk exports, client health dashboard |

- Overage: KES 5 per invoice above the tier cap.
- Phase 3: origination fee (2–4 %) on revenue-based financing distributed through a CBK-licensed Digital Credit Provider partner. CiftPay never lends.
- Subscription collection: STK push to **CiftPay's** Paybill — CiftPay revenue, not custody of merchant funds.

Unit economics targets: SMS cost ≈ KES 0.8/receipt, integrator fee ≈ KES 1–2/invoice at volume → gross margin > 75 % on Duka from ~120 invoices/month.

## 6. Go-to-market

1. **Design partners (Phase 1):** 10 merchants in one Nairobi supply chain (e.g. one restaurant group and its fresh-produce and hardware suppliers) so the buyer side pulls the seller side.
2. **Receipt as channel (Phase 1+):** every `/r/<code>` page carries "Issue your own eTIMS receipts — CiftPay".
3. **Accountants as resellers (Phase 2):** accountant tier + referral credit.
4. **Trade associations & wholesalers (Phase 2–3):** wholesalers onboard their retailer base to keep them deductible.

## 7. KPIs and gates

| Gate | Criteria |
|---|---|
| **G0** (Phase 0) | Clean checkout runs the stack; replayed webhook → `ACKED` mock invoice + receipt |
| **G1** (MVP) | 10 design-partner merchants live; ≥ 500 real eTIMS invoices; ≥ 95 % auto-match on Tills; p95 payment → KRA ack < 60 s |
| **G2** (Growth) | 300 paying merchants; < 2 % terminal failed submissions; monthly logo churn < 4 % |
| **G3** (Moat) | CAC payback < 4 months; KRA integrator certification obtained; first financing origination |

Operating dashboard: merchants active (7d), payments ingested, auto-match %, invoices ACKED, terminal failure %, p95 payment → ack, receipts delivered %, CTA clicks on receipt pages, MRR, churn.

## 8. What CiftPay deliberately is not

- Not a wallet, not an escrow, not a lender.
- Not a POS replacement (it integrates with POS in Phase 3).
- Not an accounting suite; it produces clean, fiscalised books that export to one.
- Not a KRA agent; it is a software integrator operating through approved channels.

See [`compliance.md`](compliance.md) for the regulatory reasoning and [`../plan.md`](../plan.md) for the phased build.
