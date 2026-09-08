# CiftPay — UX Flows & Screen Inventory

Companion to [`design-system.md`](design-system.md) (how it looks) and [`api.md`](api.md) (what each screen calls). Screens are listed with route, purpose, data, primary action, and empty/error states. Mobile-first; desktop adds a left rail.

## 1. Information architecture

```
/login                         (auth)         phone → OTP
/onboarding                    (auth)         business → shortcode → pay KES 1 to your own Till (one screen, 3 steps)
/today                         (merchant)     ▸ default after login
/payments                      (merchant)
/payments/[id]                 (merchant)     sheet on mobile, drawer on desktop
/invoices                      (merchant)
/invoices/[id]                 (merchant)     receipt facsimile
/attention                     (merchant)
/items                         (merchant)     ("More" tab)
/settings/*                    (merchant)     ("More" tab)
/clients                       (accountant)   org list → switches into merchant routes with X-Org-Id
/ops                           (admin)
/r/[code]                      public         no shell, no auth, no JS
/p/[ref]                       public         request-to-pay page (Phase 2)
```

Bottom navigation (mobile): **Today · Payments · Invoices · Attention · More**. "More" opens Items, Settings, Reports (Phase 2), Help, Language, Sign out.

## 2. Onboarding (target ≤ 3 minutes; shipped in §4.1)

One client screen, `/onboarding` (`OnboardingFlow.tsx`), with a three-segment `StepIndicator` ("Step 2 of 3 · Till or Paybill"). Login and OTP stay on `/login`; a session whose `orgs` is empty is sent here by `LoginForm` and by `AppShell` (any merchant route). The flow skips step 1 when an org exists and leaves for `/today` when a verified shortcode exists; an unverified one reopens step 3.

| Step | Component | User does | System does | Failure copy |
|---|---|---|---|---|
| — | `/login` | Enters phone; then the 6-digit code | `POST /auth/otp/request`, `POST /auth/otp/verify` → cookie + `orgs: []` → `/onboarding` | "That code didn't match." |
| 1 | `BusinessForm` | Business name, KRA PIN (uppercased, 11 chars), VAT registered? checkbox | Format check client-side (`^[AP]\d{9}[A-Z]$`); `POST /orgs` → fiscal provider PIN lookup; session copy gets the new membership, `X-Org-Id` switches to it | `422 pin_unknown`: "KRA doesn't recognise this PIN. Check it on iTax and try again." · `409`: "A business with this PIN is already on CiftPay…" |
| 2 | `ShortcodeForm` | Picks Till / Paybill / Pochi, enters number (Pochi accepts `07…`), optional label | `POST /shortcodes` (`auto_invoice: true`) | `409 shortcode_claimed`: "This number is already verified by another business. If it's yours, contact support." |
| 3 | `VerifyShortcode` | Opens M-Pesa on their own phone and sends **exactly KES 1** to the number just added (Buy Goods / Pay Bill with account `CIFTPAY` / Send Money for Pochi) | `POST /shortcodes/{id}/verify` → `202 pending` with masked payer MSISDN, `pay` instructions and `expires_at` (10 min); best-effort Daraja RegisterURL; the PWA counts down and polls `GET /shortcodes/{id}` every 3 s until `verified`. The C2B confirmation settles the challenge server-side and **creates no payment/sale/invoice** | expired: "Time's up. Nothing was received from 2547•••••513." → **Start again** (new challenge) · claimed (another org won the race): copy as above with a support address · no network: "No connection…" → Retry |
| done | `/today` | Taps **Go to Today** | — | — |

Also on step 3: **Pay from a different phone** (posts `{msisdn}` and reopens the challenge for that payer) and **Do this later** (to `/today`; Attention nags about the unverified shortcode). Without Daraja credentials (`make up` default) the api answers `200 verified` immediately and step 3 collapses to the success card. The default-item step from the original design is deferred to §4.5 Items; cash sales fall back to the shortcode's `default_item_id` once set.

The same `ShortcodeForm` and `VerifyShortcode` power **Settings › Shortcodes**: an **Add** button opens a `Sheet` (form → KES 1 check → **Done**), every unverified row has a **Verify** button, and each row shows whether M-Pesa confirmations are connected (`c2b_urls_registered_at`).

## 3. Daily loop (merchant)

### 3.1 Today — `/today`
- **Header:** "Today" + date in mono.
- **Hero:** total received today (`Money`, `--t-2xl/3xl`), under it three `StatusChip` counts: `N sent to KRA`, `N pending`, `N need attention` (last one links to `/attention`).
- **Live strip:** a `ReceiptCard` that appends a line per payment as it lands (polling every 15 s in Phase 0/1; SSE in Phase 2). Each line: time · masked phone · amount.
- **Last 5 payments** list → `/payments`.
- **Primary action:** `Record a sale` (ochre-emphasised primary button, fixed above BottomNav) → opens `Sheet` with item picker + quantity + optional buyer phone/PIN → `POST /sales` (Phase 2 STK; in Phase 1 it records a cash sale to be fiscalised without a payment link).
- **Empty:** "No M-Pesa payments yet today. The next one shows up here on its own."

### 3.2 Payments — `/payments`
- Filter tabs: All · Unmatched · Cash sales · Matched · Reversed.
- Row: time, masked phone + payer name, BillRef (mono, muted), `Money`, `StatusChip`.
- Tap → `Sheet` detail: raw Daraja facts (TransID, shortcode, time), link to invoice if any.
- **Unmatched row action:** `Convert to invoice` → item picker (multi-line), buyer PIN optional → `POST /payments/{id}/convert` → invoice `QUEUED` → toast "Invoice going to KRA".
- Also: `Mark as not a sale` (Phase 2: refunds, own transfers).
- **Empty:** per tab; Unmatched: "Nothing needs matching. Payments on verified tills are invoiced automatically."

### 3.3 Invoices — `/invoices`
- Filter tabs: All · Sent to KRA · Pending · Failed · Credit notes.
- Row: KRA invoice no. (mono) or "—" while pending, buyer (masked phone / PIN / name), `Money`, `StatusChip`, time.
- Detail `/invoices/[id]`: `ReceiptCard` facsimile identical to the public page, `Stamp` per state, actions: `Resend to buyer` (`POST /invoices/{id}/resend`), `Share link`, `Download PDF` (Phase 2), `Issue credit note` (Phase 2). Failed: shows KRA/vendor message in plain words and one fix action.
- **Print reveal** animation when a pending invoice becomes ACKED while open.

### 3.4 Attention — `/attention`
A single list, grouped, each item has exactly one button:

| Group | Item | Button |
|---|---|---|
| Failed invoices | "KRA rejected KES 2,400 — item code missing" | `Fix item` → picker → `POST /invoices/{id}/retry` |
| Unmatched payments | "KES 1,250 from 0712•••345, no BillRef" | `Convert` |
| Unverified shortcodes | "Till 512345 not verified" | `Verify now` |
| Pending > 10 min | "KES 800 waiting for KRA for 14 min" | `View` (no action; informational, uses ochre) |
| Plan limit (Phase 2) | "27 of 30 free invoices used" | `Upgrade` |

Badge on the BottomNav shows the count of *actionable* items only.
**Empty:** "Nothing needs your attention." (ledger-green stamp "ALL CLEAR" on a small receipt).

### 3.5 Items — `/items`
- List: name, KRA class code (mono), tax category chip (A/B/C/D/E), price.
- `Add item` → `Sheet`: name, search KRA classification (`GET /items/codes?q=` → `LookupItemCodes`), tax category (explained in one line each: "B — 16 % VAT, most goods"), unit, default price (optional).
- Set as default for a shortcode from the item row menu.

### 3.6 Settings — `/settings`
Sections (each its own route under `/settings/*`): Business (name, PIN masked, VAT status), Shortcodes (list with verify state, add, set default item, toggle auto-invoice), Receipt (footer text, show phone? language default), Language (EN/SW), Plan (Phase 2), Team (Phase 2), Sign out.

## 4. Buyer receipt — `/r/[code]` (public)

Server-rendered HTML only, ≤ 30 KB, works on Opera Mini.

1. `ReceiptCard` centred at 360 px: merchant name (Fraunces), KRA PIN, "KRA INVOICE No." + number (mono), date/time EAT, lines (`Leader`), subtotal / VAT by category / total, buyer PIN if present (masked), QR (SVG) bottom-left, `Stamp` top-right: **KRA VERIFIED** (green) / **PENDING KRA** (ochre, with "Refresh in a minute") / **CANCELLED** (red, credit note reference).
2. Under the card: `Save to phone` (uses Web Share where available, else a plain `<a download>` to `/r/[code].pdf` in Phase 2), `Add my KRA PIN` (Phase 2).
3. Footer: "Issued through CiftPay. Issue your own eTIMS receipts — ciftpay.co.ke" + privacy notice link. UTM on the link.
4. Not found: a receipt with "NO SUCH RECEIPT" stamp and "Check the code in your SMS."

## 5. Accountant portal — `/clients`

- List of client orgs (memberships with role `accountant`): name, health chips (unmatched N · failed N · VAT due KES X), last payment time.
- Tap → sets `X-Org-Id` in the API client and routes to that org's `/today`; `TopBar` shows the org name in mono and a back-to-clients link; `OrgSwitcher` in the rail.
- `Export all` (Phase 2) → one ZIP of VAT packs.

## 6. Admin — `/ops`

- Search org by name/PIN hash/shortcode; org detail with feature flags and adapter.
- Dead-letter queue: invoices in `NEEDS_REVIEW` across orgs (admin role bypasses nothing — the UI iterates orgs the admin has memberships for; the platform admin role gets a membership on every org via a periodic job).
- Re-queue, view `fiscal_submissions` request/response, mark resolved.

## 7. Notifications the buyer receives

| Event | Channel | EN | SW |
|---|---|---|---|
| Invoice ACKED | SMS | "Receipt: KES 2,400 to WANJIRU GROCERIES. KRA invoice 0012345678. View/save: ciftpay.co.ke/r/7KQ2M9" | "Risiti: KES 2,400 kwa WANJIRU GROCERIES. Invoice ya KRA 0012345678. Tazama: ciftpay.co.ke/r/7KQ2M9" |
| Pending > 5 min | SMS | "Your receipt for KES 2,400 to WANJIRU GROCERIES is being registered with KRA. Link: ciftpay.co.ke/r/7KQ2M9" | "Risiti yako ya KES 2,400 kwa WANJIRU GROCERIES inasajiliwa KRA. Kiungo: ciftpay.co.ke/r/7KQ2M9" |
| Credit note | SMS | "Receipt KRA 0012345678 was cancelled by WANJIRU GROCERIES. Details: ciftpay.co.ke/r/7KQ2M9" | … |

One SMS ≤ 160 GSM-7 characters; merchant names are truncated at 24 chars.

## 8. Offline behaviour (PWA)

- App shell, fonts, icons precached. Opening offline shows the last cached Today with a hairline banner "Offline — showing what we had at 14:02".
- `Record a sale` works offline: stored in IndexedDB queue, shown in the live strip with a `pending sync` chip; synced on reconnect; conflicts are impossible because the client generates the sale `ref`.
- Payments and invoices lists are read-only offline.

## 9. Edge cases and how the UI handles them

| Case | Behaviour |
|---|---|
| Duplicate Daraja callback | Nothing visible; `webhook_duplicates_total` metric increments |
| Reversal after ACKED (Phase 2) | Invoice shows `CANCELLED` stamp with linked credit note; Attention item "Confirm credit note sent" |
| Buyer PIN invalid format | Inline error "KRA PINs look like A123456789B"; never submitted |
| Shortcode claimed by another org | `409` → copy in §2; support link |
| KRA down for > 1 h | Attention shows one grouped item "KRA is unavailable — 14 invoices waiting. We'll keep trying."; buyers already got the pending SMS |
| Swahili overflow | Receipt lines wrap at 32 mono chars; merchant name truncates with "…" |
| Month boundary in reports (Phase 2) | Period picker uses Africa/Nairobi; a note shows "Includes payments until 23:59 EAT on the 31st" |
