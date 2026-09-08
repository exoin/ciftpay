// Minimal CiftPay API stub for Playwright smoke tests. Shapes follow
// api/openapi.yaml; only the endpoints the shell and onboarding touch are
// implemented. Onboarding routes keep a little in-memory state so a spec can
// create a business, add a Till, open the KES 1 check and poll it to verified.
import http from "node:http";
import { randomUUID } from "node:crypto";

const port = Number(process.env.STUB_PORT ?? 18080);
const origin = process.env.WEB_ORIGIN ?? "http://localhost:3000";

const receipt = {
  receipt_code: "7KQ2M9",
  state: "verified",
  kind: "INVOICE",
  seller: { name: "Mama Njeri Groceries", kra_pin: "A123456789B" },
  kra_invoice_no: "KRAMW0100000001/2026",
  kra_qr_payload: "https://etims.kra.go.ke/verify?inv=KRAMW0100000001",
  buyer_pin_masked: null,
  lines: [
    { description: "Sukari 2kg", qty: "2.000", unit_price_cents: 32000, tax_category: "B", line_total_cents: 64000, line_tax_cents: 8828 },
    { description: "Unga 2kg", qty: "1.000", unit_price_cents: 18000, tax_category: "A", line_total_cents: 18000, line_tax_cents: 0 },
  ],
  vat_by_category: [
    { category: "B", taxable_cents: 55172, tax_cents: 8828 },
    { category: "A", taxable_cents: 18000, tax_cents: 0 },
  ],
  subtotal_cents: 73172,
  tax_cents: 8828,
  total_cents: 82000,
  issued_at: "2026-09-02T11:15:30Z",
  credit_note_of: null,
};

const payment = {
  id: "11111111-1111-4111-8111-111111111111",
  trans_id: "RKTQDM7W6S",
  amount_cents: 240000,
  payer_msisdn_masked: "2547•••••345",
  paid_at: "2026-09-03T08:15:30Z",
  status: "cash_sale",
  match_rule: "auto_invoice",
};

const today = {
  date: "2026-09-03",
  received_cents: 1240000,
  payments_count: 7,
  invoices_acked: 6,
  invoices_pending: 1,
  attention_count: 2,
  recent_payments: [payment, { ...payment, id: "22222222-2222-4222-8222-222222222222", status: "unmatched", amount_cents: 50000 }],
};

const defaultOrg = { org_id: "0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", name: "Mama Njeri Groceries", role: "owner", is_default: true };

// Phones the specs sign in with. A brand-new user has no org and must be onboarded.
const NEW_USER_MSISDN = "254700000001";
// Reserved values mirrored from the backend mock adapter / handler tests.
const UNKNOWN_PIN = "P000000000Z";
const TAKEN_PIN = "P051234567X";
const CLAIMED_SHORTCODE = "999999";

const state = {
  orgs: [], // memberships created through POST /orgs in this process
  shortcodes: [], // Shortcode rows created through POST /shortcodes
  polls: new Map(), // shortcode id -> GET /shortcodes/{id} count since verify
};

function maskMsisdn(m) {
  return `${m.slice(0, 4)}•••••${m.slice(-3)}`;
}

// The PWA polls every 3 s (plus one refetch right after the 202), so three
// polls leave the instruction card on screen for ~6 s before "verified".
const POLLS_UNTIL_VERIFIED = 3;

function shortcodeView(sc) {
  const polls = state.polls.get(sc.id);
  const verified = sc.verified || (polls !== undefined && polls >= POLLS_UNTIL_VERIFIED);
  const view = { ...sc, verified, verified_at: verified ? new Date().toISOString() : null };
  if (polls !== undefined) view.verification = { status: verified ? "verified" : "pending", expires_at: sc.expires_at };
  return view;
}

// Routes with a path parameter or a body; matched before the static table.
const dynamic = [
  {
    method: "POST",
    re: /^\/auth\/otp\/verify$/,
    handle: (_m, body) => {
      const brandNew = body?.msisdn === NEW_USER_MSISDN;
      return [
        200,
        {
          user_id: brandNew ? "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" : "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
          csrf_token: "csrf-test",
          expires_at: new Date(Date.now() + 3600_000).toISOString(),
          msisdn_masked: maskMsisdn(body?.msisdn ?? "254712345678"),
          orgs: brandNew ? [] : [defaultOrg],
        },
      ];
    },
  },
  {
    method: "POST",
    re: /^\/orgs$/,
    handle: (_m, body) => {
      if (!body?.name || !/^[AP]\d{9}[A-Z]$/.test(body.kra_pin ?? "")) return [422, { error: { code: "validation", message: "KRA PIN must be A or P, nine digits and a letter" } }];
      if (body.kra_pin === UNKNOWN_PIN) return [422, { error: { code: "pin_unknown", message: "KRA does not recognise this PIN. Check it on iTax and try again" } }];
      if (body.kra_pin === TAKEN_PIN) return [409, { error: { code: "conflict", message: "A business with this KRA PIN is already registered" } }];
      const id = randomUUID();
      state.orgs.push({ org_id: id, name: body.name, role: "owner", is_default: state.orgs.length === 0 });
      return [
        201,
        {
          id,
          name: body.name,
          kra_pin_masked: `${body.kra_pin[0]}•••••••••${body.kra_pin.slice(-1)}`,
          kra_pin_verified_at: new Date().toISOString(),
          vat_registered: Boolean(body.vat_registered),
          locale: body.locale ?? "en",
          fiscal_adapter: "mock",
          created_at: new Date().toISOString(),
        },
      ];
    },
  },
  {
    method: "POST",
    re: /^\/shortcodes$/,
    handle: (_m, body) => {
      if (!/^\d{5,12}$/.test(body?.shortcode ?? "")) return [422, { error: { code: "validation", message: "shortcode must be 5–12 digits" } }];
      if (body.shortcode === CLAIMED_SHORTCODE) {
        return [409, { error: { code: "shortcode_claimed", message: "This number is already verified by another business. If it is yours, contact support." } }];
      }
      const sc = { id: randomUUID(), kind: body.kind, shortcode: body.shortcode, label: body.label ?? "", default_item_id: null, auto_invoice: body.auto_invoice ?? true, verified: false, verified_at: null, c2b_urls_registered_at: null };
      state.shortcodes.push(sc);
      return [201, shortcodeView(sc)];
    },
  },
  {
    method: "POST",
    re: /^\/shortcodes\/([^/]+)\/verify$/,
    handle: (m) => {
      const sc = state.shortcodes.find((s) => s.id === m[1]);
      if (!sc) return [404, { error: { code: "not_found", message: "Not found" } }];
      if (shortcodeView(sc).verified) return [200, { status: "verified", shortcode: shortcodeView(sc) }];
      sc.expires_at = new Date(Date.now() + 10 * 60_000).toISOString();
      sc.c2b_urls_registered_at = new Date().toISOString();
      state.polls.set(sc.id, 0);
      return [
        202,
        {
          status: "pending",
          msisdn_masked: maskMsisdn(NEW_USER_MSISDN),
          pay: { kind: sc.kind, shortcode: sc.shortcode, amount_cents: 100, account_ref: "CIFTPAY" },
          expires_at: sc.expires_at,
        },
      ];
    },
  },
  {
    method: "GET",
    re: /^\/shortcodes\/([^/]+)$/,
    handle: (m) => {
      const sc = state.shortcodes.find((s) => s.id === m[1]);
      if (!sc) return [404, { error: { code: "not_found", message: "Not found" } }];
      if (state.polls.has(sc.id)) state.polls.set(sc.id, state.polls.get(sc.id) + 1);
      return [200, shortcodeView(sc)];
    },
  },
];

const routes = {
  "GET /healthz": () => [200, { status: "ok", db: "ok", queue: "ok" }],
  "GET /r/7KQ2M9": () => [200, receipt],
  "GET /reports/today": () => [200, today],
  "GET /attention": () => [200, { failed_invoices: [], unmatched_payments: [today.recent_payments[1]], unverified_shortcodes: [], pending_long: [], actionable_count: 2 }],
  "GET /payments": () => [200, { data: today.recent_payments, next_cursor: null }],
  "GET /invoices": () => [200, { data: [], next_cursor: null }],
  "GET /items": () => [200, { data: [] }],
  "GET /shortcodes": () => [200, { data: state.shortcodes.map(shortcodeView) }],
  "GET /orgs": () => [200, { data: [defaultOrg, ...state.orgs] }],
  "GET /orgs/current": () => [200, { id: "0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", name: "Mama Njeri Groceries", kra_pin_masked: "A•••••••••B", locale: "en", vat_registered: true }],
  "GET /billing/entitlement": () => [200, { plan: "Hustler", used: 12, limit: 30 }],
  "POST /auth/otp/request": () => [202, { expires_in_seconds: 300 }],
  "POST /auth/otp/verify": () => [
    200,
    {
      user_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
      csrf_token: "csrf-test",
      expires_at: new Date(Date.now() + 3600_000).toISOString(),
      orgs: [{ org_id: "0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", name: "Mama Njeri Groceries", role: "owner", is_default: true }],
    },
  ],
};

const server = http.createServer((req, res) => {
  const url = new URL(req.url ?? "/", `http://localhost:${port}`);
  res.setHeader("Access-Control-Allow-Origin", origin);
  res.setHeader("Access-Control-Allow-Credentials", "true");
  res.setHeader("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Org-Id, X-Request-ID");
  res.setHeader("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS");
  if (req.method === "OPTIONS") {
    res.writeHead(204).end();
    return;
  }
  const handler = routes[`${req.method} ${url.pathname}`];
  res.setHeader("Content-Type", "application/json");
  if (!handler) {
    res.writeHead(404).end(JSON.stringify({ error: { code: "not_found", message: "No receipt with that code." } }));
    return;
  }
  const [status, body] = handler();
  res.writeHead(status).end(JSON.stringify(body));
});

server.listen(port, "127.0.0.1", () => {
  console.log(`stub api listening on http://127.0.0.1:${port}`);
});
