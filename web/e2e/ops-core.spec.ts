import { expect, test } from "@playwright/test";

const nonAdminSession = {
  user_id: "user-owner-1111-2222-3333",
  csrf_token: "csrf-test",
  expires_at: new Date(Date.now() + 3600_000).toISOString(),
  orgs: [{ org_id: "0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", name: "Mama Njeri Groceries", role: "owner", is_default: true }],
};

const adminSession = {
  user_id: "user-admin-9999-8888-7777",
  csrf_token: "csrf-test",
  expires_at: new Date(Date.now() + 3600_000).toISOString(),
  orgs: [{ org_id: "0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", name: "Mama Njeri Groceries", role: "admin", is_default: true }],
};

test.describe("Ops-Core Security & RBAC Guards", () => {
  test("non-admin (owner) is blocked with 403 Forbidden across all Ops routes", async ({ page }) => {
    await page.addInitScript((s) => {
      window.sessionStorage.setItem("ciftpay.session", JSON.stringify(s));
      window.sessionStorage.setItem("ciftpay.csrf", s.csrf_token);
      window.localStorage.setItem("ciftpay.org", s.orgs[0]!.org_id);
    }, nonAdminSession);

    // 1. /ops/safaricom
    await page.goto("/ops/safaricom");
    await expect(page.getByRole("heading", { name: "403 - Forbidden" })).toBeVisible();
    await expect(page.getByText(/You must hold an admin role/)).toBeVisible();

    // 2. /ops/merchants
    await page.goto("/ops/merchants");
    await expect(page.getByRole("heading", { name: "403 - Forbidden" })).toBeVisible();

    // 3. /ops/kra-dlq
    await page.goto("/ops/kra-dlq");
    await expect(page.getByRole("heading", { name: "403 - Forbidden" })).toBeVisible();

    // 4. /ops/system
    await page.goto("/ops/system");
    await expect(page.getByRole("heading", { name: "403 - Forbidden" })).toBeVisible();
  });
});

test.describe("Ops-Core Functional Modules (Admin Session)", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript((s) => {
      window.sessionStorage.setItem("ciftpay.session", JSON.stringify(s));
      window.sessionStorage.setItem("ciftpay.csrf", s.csrf_token);
      window.localStorage.setItem("ciftpay.org", s.orgs[0]!.org_id);
    }, adminSession);
  });

  test("Safaricom Pipeline: displays merchants, changes status, and exports batch", async ({ page }) => {
    await page.goto("/ops/safaricom");

    await expect(page.getByRole("heading", { level: 1, name: "Safaricom Daraja Onboarding Pipeline" })).toBeVisible();
    await expect(page.getByText("Mama Njeri Groceries")).toBeVisible();

    // Test inline status dropdown change
    const statusSelect = page.locator("select").first();
    await expect(statusSelect).toBeVisible();
    await statusSelect.selectOption("sent_to_safaricom");

    // Test Export Ready Merchants modal
    const exportBtn = page.getByRole("button", { name: "Export Ready Merchants" });
    await expect(exportBtn).toBeVisible();
    await exportBtn.click();

    await expect(page.getByText("Safaricom Batch Onboarding Export")).toBeVisible();
    await expect(page.getByRole("button", { name: "Copy Export JSON" })).toBeVisible();
    await page.getByRole("button", { name: "Close" }).click();
  });

  test("Merchant CRM: search, tier upgrade, and notification broadcast", async ({ page }) => {
    await page.goto("/ops/merchants");

    await expect(page.getByRole("heading", { level: 1, name: "Merchant CRM & Entitlements" })).toBeVisible();
    await expect(page.getByText("Mama Njeri Groceries")).toBeVisible();

    // Test search filter
    const searchInput = page.getByPlaceholder("Search by merchant name or ID...");
    await searchInput.fill("Acme");
    await expect(page.getByText("Acme Enterprises")).toBeVisible();
    await searchInput.fill("");

    // Test subscription tier change
    const tierSelect = page.locator("select").first();
    await tierSelect.selectOption("biashara");
    await expect(page.getByText(/Updated/)).toBeVisible();

    // Test Notify modal
    const notifyBtn = page.getByRole("button", { name: "Notify" }).first();
    await notifyBtn.click();

    await expect(page.getByText(/Direct Broadcast to/)).toBeVisible();
    await page.locator("textarea").fill("Please upload your company registration certificate.");
    await page.getByRole("button", { name: "Send Notification" }).click();
    await expect(page.getByText(/Notification dispatched successfully/)).toBeVisible();
  });

  test("KRA DLQ: inspects raw JSON error and tests Force ACKED manual override", async ({ page }) => {
    await page.goto("/ops/kra-dlq");

    await expect(page.getByRole("heading", { level: 1, name: "KRA Fiscal Dead-Letter Queue (DLQ)" })).toBeVisible();
    await expect(page.getByText("invo-dlq-1111-2222-3333")).toBeVisible();
    await expect(page.getByText("oscu_rejected_901: Invalid taxpayer device state")).toBeVisible();

    // Test JSON payload modal
    await page.getByRole("button", { name: "{ } JSON" }).first().click();
    await expect(page.getByText("Raw Upstream KRA Response")).toBeVisible();
    await expect(page.getByText("TERMINAL_REJECTED")).toBeVisible();
    await page.getByRole("button", { name: "Close" }).click();

    // Test Force ACKED override modal
    await page.getByRole("button", { name: "Force ACKED" }).first().click();
    await expect(page.getByText("Manual Fiscal State Override (Force ACKED)")).toBeVisible();

    await page.getByPlaceholder("e.g. KRAMW0100000001/2026").fill("KRAMW999999/2026");
    await page.getByPlaceholder("Cryptographic receipt signature string...").fill("TEST_SIGNATURE_MEQCIDUMMY");
    await page.getByRole("button", { name: "Confirm Force ACKED" }).click();

    await expect(page.getByText(/manually marked ACKED/)).toBeVisible();
  });

  test("System Health: updates Daraja IP allowlist and replays webhook event", async ({ page }) => {
    await page.goto("/ops/system");

    await expect(page.getByRole("heading", { level: 1, name: "Engineering Health & System Controls" })).toBeVisible();

    // Test Dynamic IP Allowlist
    const textarea = page.locator("textarea");
    await expect(textarea).toBeVisible();
    await textarea.fill("196.201.214.0/24\n196.201.213.0/24\n196.201.212.0/24");
    await page.getByRole("button", { name: "Apply Dynamic Allowlist" }).click();
    await expect(page.getByText("Daraja dynamic CIDR allowlist updated in memory.")).toBeVisible();

    // Test Webhook Inspector & Replay
    await expect(page.getByText("RKTQDM7W6S")).toBeVisible();

    // View Payload
    await page.getByRole("button", { name: "Payload" }).first().click();
    await expect(page.getByText("Webhook Raw Ingestion Payload")).toBeVisible();
    await page.getByRole("button", { name: "Close" }).click();

    // Trigger Replay
    await page.getByRole("button", { name: "Replay" }).first().click();
    await expect(page.getByText("Webhook payload replayed successfully through ingestion engine.")).toBeVisible();
  });
});
