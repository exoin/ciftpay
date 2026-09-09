---
sessionId: session-260908-175410-1gfa
supersedes: phase1-onboarding-shortcode-verification.md (Steps 3–7, verification part)
---

# Requirements

### Overview & Goals
Replace the **own‑Till KES 1 control check** shipped on 2026‑09‑08 with the **Administrative Gate**: ownership of a Till/Paybill/Pochi is proven by Safaricom's own Go‑Live paperwork (signed + stamped *CiftPay Safaricom Authorization Letter*, KYC‑checked by Safaricom before they map the shortcode to CiftPay's Daraja app), and a CiftPay operator flips the shortcode to `verified` once Safaricom confirms. No M‑Pesa transaction, no B2B, no merchant Daraja keys, no passkey.

Decisions confirmed (2026‑09‑09):
- (a) **No API‑based ownership proof** — no B2B micro‑payment, no KES 1, no receipt codes.
- (b) **No escape hatch** — CiftPay never stores merchant Daraja credentials (zero‑credentials, ADR‑0003).
- (c) **No CiftPay production/B2B shortcode exists** — `DARAJA_SHORTCODE=600000` (sandbox C2B) is the only shortcode CiftPay itself uses.

Why this is correct (from the 2026‑09‑09 analysis): paying KES 1 to a Till only proves you are a *customer* of that Till; and for a Till that is not yet mapped to CiftPay's app Safaricom never sends the callback, so the old check could not even succeed in production. The letter is the one artefact Safaricom actually checks against the Till's KYC.

### Scope
**In scope**
- Schema: drop `shortcode_verifications`; `mpesa_shortcodes.status` (`pending_authorization` default | `verified` | `rejected`), letter path/submitted‑at/reviewed‑by/rejection reason; admin RLS scope.
- Backend: remove `POST /shortcodes/{id}/verify`, `tryVerification`, the STK `verify` purpose branch and the unconfigured‑Daraja auto‑verify hole; add `POST /shortcodes/{id}/authorization` (multipart upload); admin `GET /admin/shortcodes`, `PATCH /admin/shortcodes/{id}/verify|reject`, `GET /admin/shortcodes/{id}/authorization`; `ciftctl verify-shortcode`; C2B ingest resolves **only `verified`** rows; `RegisterURL` runs when an operator verifies.
- Config: drop `DARAJA_PASSKEY`; `DARAJA_SHORTCODE` default `600000`; new `UPLOAD_DIR`; production refuses to start without Daraja credentials.
- Web: onboarding Step 3 = *Authorization Document* (instruction card, printable pre‑filled letter, file upload, "Submit for verification" → `/today`); Settings shows `Pending authorization / Verified / Rejected` and lets the merchant (re)upload; remove countdown/polling/KES 1 UI.
- Docs: ADR‑0008, `plan.md` §4.1 corrected, `docs/api.md`, `docs/ux-flows.md`, `docs/data-model.md`, runbook (ops procedure), `api/openapi.yaml`, `.env.example`, master `.junie` plan.

**Out of scope**
- Automated letter OCR / e‑signature; S3 storage (local disk behind a `storage.Store` interface is enough for design partners); an admin *UI* (the JSON endpoints + `ciftctl` are the ops tool for now, `plan.md` §4.6 keeps the `/ops` UI); Safaricom partner/aggregator agreement (relationship work, §9.2).

### User Stories
- As a merchant, I want to add my Till, print the pre‑filled authorization letter, sign/stamp it, upload a photo and get on with using CiftPay (manual sales, items, staff) while Safaricom processes it.
- As a merchant, I want Settings to tell me honestly whether my Till is *pending*, *verified* or *rejected* (and why), and let me upload a corrected letter.
- As a CiftPay operator, I want a queue of pending shortcodes with the letter and the org's KRA PIN so I can send the batch to Safaricom and flip each one to `verified` (or `rejected` with a reason) when they answer.
- As a buyer/merchant, I never want money to reach a tax ledger of a business that Safaricom has not confirmed owns the Till.

### Functional Requirements
1. `POST /shortcodes` → 201 row with `status = pending_authorization`; `409 shortcode_claimed` when another org holds a **verified** row for the number. Duplicate pending rows across orgs are allowed (Safaricom's answer decides).
2. `POST /shortcodes/{id}/authorization` (multipart, field `letter`, JPEG/PNG/WebP/PDF ≤ 10 MB) stores the file under `UPLOAD_DIR/authorizations/<org>/<shortcode-id>.<ext>`, sets `authorization_letter_path`, `authorization_submitted_at = now()`; a `rejected` row goes back to `pending_authorization` and clears `rejection_reason`; a `verified` row → `409 conflict`; wrong type/size/missing file → `422 validation`. Audit `shortcode.authorization_submitted`.
3. `GET /shortcodes/{id}` / list return `status`, `verified` (compat bool), `verified_at`, `authorization_submitted_at`, `authorization_letter_uploaded`, `rejection_reason`, `c2b_urls_registered_at`. No `verification` object.
4. Admin (`memberships.role = admin`, existing `RequireRole`): `GET /admin/shortcodes?status=` lists rows across orgs with `org_id`, `org_name`, `org_kra_pin`; `PATCH /admin/shortcodes/{id}/verify` → `status = verified`, `verified_at`, `reviewed_by`, audit `shortcode.verified {mode: admin}`, then best‑effort `RegisterC2BURLs` (sets `c2b_urls_registered_at`); partial unique index violation → `409 shortcode_claimed`. `PATCH /admin/shortcodes/{id}/reject {reason}` → `status = rejected`, audit `shortcode.rejected`. `GET /admin/shortcodes/{id}/authorization` streams the letter.
5. C2B confirmation for a number with **no verified row** → event stored, marked processed with error `no verified shortcode <n>`, webhook still answers 200 (Daraja must not retry), **no payment/sale/invoice**. Reconcile behaves the same.
6. `ciftctl verify-shortcode <id|number> [--reject "reason"]` does the same as the admin endpoint from the shell (ops without an admin session).
7. Web `/onboarding` Step 3: instruction card (download/print letter → sign → stamp → photo → upload), "Print the letter" (pre‑filled printable page), file input (`accept="image/*,application/pdf"`, camera on mobile), "Submit for verification" → upload → `/today`; "Do this later" stays. Settings: status chip per row, "Upload letter" for pending/rejected rows, rejection reason shown, "Add" sheet = form → letter step.
8. Seed data marks the seed Till `verified` via the new query so `make replay-webhook` keeps working.

### Non‑Functional Requirements
- Zero custody / zero credentials (ADR‑0003) — nothing about the merchant's M‑Pesa account is stored beyond the shortcode itself and the letter image.
- RLS (ADR‑0007): the admin path reads `mpesa_shortcodes` under a new `app.scope = 'admin'` SELECT policy; every write still happens under the org scope so audit rows land in the right tenant.
- Letter files are never served publicly; only the admin endpoint reads them. Local disk in dev/compose (`uploads` volume).
- `go build/vet/test ./...`, `tsc`, `eslint`, vitest, Playwright green.

# Technical Design

### Key Decisions
1. **Migration `0003_administrative_gate.sql`, not a rewrite of `0002`.** `0002` is already applied in local databases; goose history stays linear. `0003` drops the table, adds the columns, backfills `status` from `verified_at`, replaces the partial unique index with `WHERE status = 'verified'` and adds `CHECK ((status = 'verified') = (verified_at IS NOT NULL))`.
2. **`ResolveShortcode` filters `status = 'verified'`.** This is the single gate for the tax ledger: unverified numbers are simply unknown to the ingest path. No per‑payment branching, no "quarantine" ledger.
3. **Admin decisions live in `internal/admin`** (`Service.VerifyShortcode/RejectShortcode/PendingShortcodes/OpenLetter`) so the HTTP handler and `ciftctl` share one implementation; `RegisterURL` is called there because that is the moment Safaricom has mapped the Till.
4. **`storage.Store` interface with a local‑disk implementation** (`internal/platform/storage`). Keys are relative paths; the DB stores the key, never an absolute path. Swappable for S3 later without touching handlers.
5. **Printable letter rendered by the PWA** (`/onboarding/letter?shortcode=<id>`), pre‑filled from the session org + shortcode, `window.print()` → PDF. No server‑side PDF library.
6. **Keep `verified` boolean in the API** for compatibility with existing web code and e2e stubs; `status` is the source of truth.
7. **`DARAJA_PASSKEY` removed.** `mpesa.Client.STKPush` returns `ErrSTKNotConfigured` until Phase 2 (§5.1/§5.6) re‑introduces a passkey for CiftPay's own Paybill.

### Proposed Changes (checklist)
- [x] Plan written
- [x] `api/openapi.yaml` contract (Shortcode/status, authorization upload, admin paths; drop `/verify`)
- [x] Migration `0003`, queries, `sqlc generate`
- [x] Config (`Passkey` gone, `Shortcode` default, `UploadDir`, prod validation), `.env.example`, `.env`, `docker-compose.yml`
- [x] `internal/platform/storage`
- [x] Ledger: remove verification, gate ingest, authorization upload, views
- [x] Admin service + handler; `ciftctl verify-shortcode`; seed
- [x] Backend tests (`authorization_test.go` replaces `verification_test.go`)
- [x] Web (delegated): onboarding Step 3, letter page, Settings, hooks, i18n, stub API, Playwright
- [x] Docs: ADR‑0008, `plan.md`, `docs/api.md`, `docs/ux-flows.md`, `docs/data-model.md`, runbook, master `.junie` plan
- [x] Verification: `go test ./...` (DB), `make e2e`, vitest, tsc/eslint

### Deviations / notes
- Migration `0003` backfill needed `NO FORCE ROW LEVEL SECURITY` around the `UPDATE` (owner is RLS-scoped too); logged in the runbook 2026-09-09.
- Attention page: "Verify now" became a link to Settings ("Upload letter") since inline verification no longer exists.
- Verification: backend `go build/vet/test ./...` green (4 new DB-backed gate scenarios in `authorization_test.go`); web `tsc`, `eslint`, vitest 21/21, Playwright 38/38.
- `web/public/sw.js` is regenerated by `next build` and git-ignored; not part of the change set.
