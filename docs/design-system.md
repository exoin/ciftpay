# CiftPay — Design System: "The Honest Receipt"

CiftPay sells **proof**. A merchant trusts it because it produces a receipt that KRA accepts; a buyer trusts it because the receipt they get is verifiable. The interface therefore borrows from the two most trusted paper objects in a Kenyan shop — the thermal receipt and the ruled ledger book — and from nothing else. No fintech dashboard tropes.

This file is the source of truth for `web/src/styles/tokens.css`, the `ui/` components, and any designer or agent producing screens. When in doubt: **would this look right printed on a till roll?**

## 1. Anti-generic rules (hard constraints)

These exist because generated UIs converge on the same look. Violating any of them is a review blocker.

1. **No gradients** of any kind on surfaces or text. Especially no purple/indigo/blue-to-pink. Colour is flat.
2. **No glassmorphism**, no frosted blur, no translucent cards, no neon glows.
3. **No 3-column "feature cards" grid** with icon-title-blurb. No marketing hero inside the app. `/today` is the first screen after login, not a dashboard splash.
4. **No stock illustrations**, no 3D blobs, no isometric people, no emoji anywhere in UI text or buttons. Icons are 1.5 px stroke line icons (Lucide subset) or nothing.
5. **No Inter, Roboto, Poppins, Montserrat, Open Sans** for display or body. Display is `Fraunces`, body is `IBM Plex Sans`, numerals are `IBM Plex Mono`.
6. **No rounded-pill everything.** Max radius 6 px. Buttons are rectangles with 4 px radius. Chips are 3 px.
7. **No drop shadows on cards.** Depth is expressed by 1 px hairlines and paper texture. Shadows are permitted only on `Sheet` (bottom sheet) and `Toast`.
8. **No centred layouts on desktop.** Content is left-anchored with a fixed left rail; whitespace accumulates on the right, like a ledger with a wide margin.
9. **Numbers are the hero.** Every amount is tabular mono, right-aligned, with `KES` set in small caps and a thin-space (U+2009) thousands separator: `KES 12 400.00`. No `$`, no `Ksh`.
10. **Copy is direct.** No "Oops!", no "Something went wrong 😅", no "Awesome!". Plain Kenyan English and Swahili (see §7).

## 2. Colour tokens

Palette is derived from ink on cream till paper, a green accountant's ledger, and KRA's red stamp.

| Token | Hex | Role |
|---|---|---|
| `--ink` | `#14130F` | Primary text, borders at full strength |
| `--ink-2` | `#3A3833` | Secondary text |
| `--muted` | `#6B675E` | Tertiary text, placeholders, disabled |
| `--hairline` | `#D9D2C3` | 1 px rules, table borders, dotted leaders |
| `--paper` | `#F6F1E7` | App background (with grain, §5) |
| `--paper-2` | `#FFFDF8` | Card / receipt surface (slightly whiter than background) |
| `--paper-3` | `#EEE7D8` | Pressed / selected row background |
| `--ledger-green` | `#0B3D2E` | Primary brand, primary button, active nav |
| `--ledger-green-2` | `#0F5A43` | Hover on primary |
| `--ochre` | `#C97B12` | Accent: pending states, "Record a sale" emphasis, focus ring |
| `--kra-red` | `#9E2B25` | Failed, needs attention, destructive |
| `--ok` | `#2E7D4F` | ACKED, verified, delivered |
| `--ok-bg` | `#E4EFE7` | Background for ok chips |
| `--warn-bg` | `#F6E9D2` | Background for pending chips |
| `--bad-bg` | `#F3DEDC` | Background for failed chips |

Dark mode: **not in Phase 0–1.** Receipts are read on paper-coloured screens; a dark variant is a Phase 2 decision.

Contrast: every text token on `--paper` and `--paper-2` meets WCAG AA (ink 15.6:1, ink-2 9.8:1, muted 4.9:1, ledger-green 11.2:1, kra-red 7.1:1, ochre 3.9:1 — **ochre is decorative only, never text under 18 px**).

## 3. Typography

Self-hosted, subset to Latin + Swahili diacritics (none needed beyond basic Latin), `font-display: swap`, in `web/public/fonts/`.

| Role | Family | Weights | Notes |
|---|---|---|---|
| Display | `Fraunces` (variable) | 400–600, `opsz` auto, `SOFT` 50 | Page titles, big totals on Today, receipt merchant name |
| Body | `IBM Plex Sans` | 400, 500, 600 | Everything else |
| Numerals / receipt | `IBM Plex Mono` | 400, 500 | All amounts, KRA invoice numbers, codes, receipt body, tables |

Scale (1.2 modular, base 16 px, `rem`):

| Token | Size | Line height | Use |
|---|---|---|---|
| `--t-xs` | 12.5 px | 1.4 | Timestamps, footnotes on receipt |
| `--t-sm` | 14 px | 1.45 | Table cells, chips, secondary |
| `--t-md` | 16 px | 1.5 | Body (minimum on mobile) |
| `--t-lg` | 19 px | 1.4 | Section titles, list item primary |
| `--t-xl` | 23 px | 1.3 | Page titles |
| `--t-2xl` | 28 px | 1.2 | Today total (mobile) |
| `--t-3xl` | 33 px | 1.15 | Today total (desktop), receipt total |

Rules: `font-variant-numeric: tabular-nums` on every element that can hold a number. Letter-spacing 0 except small caps `KES` label (`0.04em`). Sentence case everywhere; no ALL CAPS except the receipt header block, which mimics thermal print (`text-transform: uppercase; letter-spacing: 0.08em; font-family: mono`).

## 4. Spacing, radius, borders

- Spacing scale: 4, 8, 12, 16, 24, 32, 48, 64 px (`--s-1` … `--s-8`).
- Radius: `--r-1` 3 px (chips), `--r-2` 4 px (buttons, inputs), `--r-3` 6 px (cards, sheets). Nothing larger.
- Borders: 1 px `--hairline` by default; 1 px `--ink` for focused/active; 2 px `--ochre` outline offset 2 px for keyboard focus.
- Touch targets ≥ 44 × 44 px; list rows ≥ 56 px on mobile.

## 5. Texture & signature elements

**Paper grain.** `body` background is `--paper` plus an inline SVG `feTurbulence` noise at 4 % opacity, tiled 240 px, defined once in `globals.css` as a data URI (~600 bytes). It must be subtle: visible on a 6-inch screen at arm's length only if you look for it.

**Perforated edge.** `ReceiptCard` has a top (and optionally bottom) edge made with a `radial-gradient` mask of 6 px circles every 12 px, so the card looks torn from a roll. This is the one place "gradient" is allowed because it is a mask, not a colour transition.

**Dotted leaders.** Label–amount rows in receipts and summaries use a flex row where the middle element is `border-bottom: 1px dotted var(--hairline)` aligned to the baseline — like a menu or a ledger. Component: `<Leader label amount />`.

**Ruled lines.** Tables and lists use horizontal hairlines only; no vertical rules, no zebra striping. Selected row uses `--paper-3`.

**Stamp.** Status of a receipt is a rotated (−6°) rectangular outline stamp in `--ok` ("KRA VERIFIED"), `--ochre` ("PENDING KRA"), or `--kra-red` ("FAILED") in mono uppercase. Used only on receipt detail and the public receipt page. Never animated except on first ack (§6).

## 6. Motion

- Duration 120–180 ms, easing `cubic-bezier(0.2, 0, 0, 1)` (ease-out). Nothing bounces.
- **Print reveal:** when an invoice becomes `ACKED` while on screen, the `ReceiptCard` grows from the perforated edge downward (`clip-path: inset(0 0 100% 0)` → `inset(0)`) over 400 ms, then the stamp fades in at 150 ms. This is the single "delight" moment and must respect `prefers-reduced-motion` (falls back to a fade).
- Sheets slide up 200 ms; toasts slide in from top 150 ms.
- No skeleton shimmer; loading uses a static dotted leader row pattern in `--hairline`.

## 7. Voice & tone

Kenyan English by default, Swahili when the user chooses. Direct, specific, present tense, second person. Numbers and names before adjectives.

| Situation | Yes | No |
|---|---|---|
| Payment received & queued | "KES 2,400 from 0712•••345 — invoice going to KRA" / "Umelipwa KES 2,400 — invoice inatumwa KRA" | "Payment received successfully! 🎉" |
| KRA slow | "KRA is slow right now. We'll keep trying and text the buyer when it's through." | "Oops, something went wrong." |
| Terminal failure | "KRA rejected this invoice: item code missing. Pick an item to fix it." | "Error 422" |
| Empty payments | "No M-Pesa payments yet today. The next one shows up here on its own." | "Nothing to see here!" |
| Unverified shortcode | "Till 512345 isn't verified. We'll send KES 1 to your phone to confirm you control it." | "Verification required" |

Rules: never blame the user; always say what happens next; name the counterparty (KRA, Safaricom) instead of "the system"; masked phone numbers show first 4 and last 3 digits.

Swahili strings must be reviewed by a native speaker before release and tested for overflow in `ReceiptCard` (Swahili runs ~15 % longer).

## 8. Components (`web/src/components/ui`)

| Component | Anatomy & rules |
|---|---|
| `Button` | Variants `primary` (ledger-green fill, paper text), `secondary` (paper-2 fill, ink border), `ghost` (no border), `danger` (kra-red outline). Sizes `md` 44 px, `sm` 36 px. Always a verb: "Record a sale", "Retry with KRA", "Resend receipt". Loading state replaces label with three static dots, no spinner. |
| `Money` | `<Money cents currency="KES" />` → `KES 12 400.00` in mono, tabular, right-aligned; negative in kra-red with leading `−`; `size` maps to type scale. Never renders floats. |
| `ReceiptCard` | Paper-2 surface, perforated top, mono header block (merchant, PIN, KRA invoice no., date), `Leader` rows for lines and totals, QR at bottom left, `Stamp` at top right. Prop `state` drives stamp. Max width 360 px; on desktop it stays receipt-shaped, never stretches. |
| `Leader` | `label · · · · · amount` row. |
| `StatusChip` | `acked` (ok/ok-bg), `pending` (ochre/warn-bg), `failed` (kra-red/bad-bg), `unmatched` (ink/paper-3), `cash` (ink/paper-3). 3 px radius, mono `--t-sm`, dot before label. |
| `DataTable` | Hairline rows, sticky header, numeric columns right-aligned mono; on mobile collapses to a list with primary/secondary text + trailing `Money`. |
| `Sheet` | Bottom sheet on mobile, right drawer on desktop; the only component with a shadow. Used for convert-payment, item picker, filters. |
| `Tabs` | Underline tabs, 2 px ink underline, no pill background. |
| `EmptyState` | One sentence of copy (§7) + optional single `Button`. No illustration. |
| `Stamp` | Rotated outline label, see §5. |
| `Field` | Label above, 44 px input, hairline border, ochre focus ring, error text in kra-red below. Phone inputs pre-fill `+254`. |
| `Toast` | Top, paper-2 with ink border, auto-dismiss 4 s, never stacks more than 2. |

Shell components (`components/shell`): `BottomNav` (5 tabs: Today · Payments · Invoices · Attention · More; active tab = ledger-green icon + 2 px top rule; Attention shows a count badge in kra-red), `TopBar` (page title in Fraunces, org name in mono, optional right action), `OrgSwitcher` (for accountants/admins; a `Sheet` listing orgs with health chips).

## 9. Layout

- Mobile (< 768 px): single column, 16 px gutters, `BottomNav` fixed, `TopBar` sticky.
- Desktop (≥ 1024 px): 240 px left rail (nav + org switcher), content column max 880 px left-anchored, right margin free. Receipt detail shows the `ReceiptCard` on the right at natural width.
- The public receipt page ignores the shell: centred `ReceiptCard` at 360 px on `--paper`, footer CTA below. This is the one centred layout, because it is a document, not an app.

## 10. Iconography & imagery

- Icons: Lucide, 20 px, 1.5 px stroke, `currentColor`, inline SVG only. Max one icon per row.
- QR codes: generated server-side as SVG (`/r/<code>.svg`), 3 px module, `--ink` on `--paper-2`, no logo overlay (KRA scanners).
- Logo: wordmark "CiftPay" in Fraunces 600 with the `f` and `t` ligature; monogram is a perforated-edge square with a "C". No gradients.

## 11. Accessibility & performance budgets

- WCAG 2.2 AA; focus visible everywhere; all interactive elements reachable by keyboard; `aria-live="polite"` on the Today total and on Attention count.
- Text resizes to 200 % without horizontal scroll.
- Colour is never the only signal: chips have labels, stamps have text.
- Budgets (enforced by Lighthouse CI in `web-ci.yml` from Phase 1): app shell JS ≤ 180 KB gz, fonts ≤ 120 KB total, `/r/<code>` total ≤ 30 KB and zero JS, LCP ≤ 2.5 s on simulated 3G/Moto G4.

## 12. Do / don't gallery (for reviewers)

- **Do** open on Today with the number first, then the chips, then the list.
- **Don't** add a welcome banner, tips carousel, or "What's new".
- **Do** let a failed invoice look like a rejected receipt (red stamp) with one button.
- **Don't** show error codes without the fix.
- **Do** keep the receipt exactly the same in-app, on `/r/<code>`, and in the shared PDF.
- **Don't** invent a second "pretty" receipt for marketing.
