// Minimal CiftPay API stub for Playwright smoke tests. Shapes follow
// api/openapi.yaml; only the endpoints the shell touches are implemented.
import http from "node:http";

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

const routes = {
  "GET /healthz": () => [200, { status: "ok", db: "ok", queue: "ok" }],
  "GET /r/7KQ2M9": () => [200, receipt],
  "GET /reports/today": () => [200, today],
  "GET /attention": () => [200, { failed_invoices: [], unmatched_payments: [today.recent_payments[1]], unverified_shortcodes: [], pending_long: [], actionable_count: 2 }],
  "GET /payments": () => [200, { data: today.recent_payments, next_cursor: null }],
  "GET /invoices": () => [200, { data: [], next_cursor: null }],
  "GET /items": () => [200, { data: [] }],
  "GET /shortcodes": () => [200, { data: [] }],
  "GET /orgs": () => [200, { data: [{ org_id: "0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", name: "Mama Njeri Groceries", role: "owner", is_default: true }] }],
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
