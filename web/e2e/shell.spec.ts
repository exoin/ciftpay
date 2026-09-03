import { expect, test } from "@playwright/test";

const session = {
  user_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  csrf_token: "csrf-test",
  expires_at: new Date(Date.now() + 3600_000).toISOString(),
  orgs: [{ org_id: "0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", name: "Mama Njeri Groceries", role: "owner", is_default: true }],
};

test("signed-out visit lands on /login with the OTP form", async ({ page }) => {
  await page.goto("/today");
  await expect(page).toHaveURL(/\/login/);
  await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
  await expect(page.getByLabel("Phone number")).toBeVisible();
  await expect(page.getByRole("button", { name: "Text me a code" })).toBeVisible();
});

test("OTP flow signs in and opens Today", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Phone number").fill("0712 345 678");
  await page.getByRole("button", { name: "Text me a code" }).click();
  await expect(page.getByText(/Code sent to 2547/)).toBeVisible();
  await page.getByLabel("6-digit code").fill("123456");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/today/);
  await expect(page.getByRole("heading", { level: 1, name: "Today" })).toBeVisible();
});

test.describe("merchant shell", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript((s) => {
      window.sessionStorage.setItem("ciftpay.session", JSON.stringify(s));
      window.sessionStorage.setItem("ciftpay.csrf", s.csrf_token);
      window.localStorage.setItem("ciftpay.org", s.orgs[0]!.org_id);
    }, session);
  });

  test("Today shows the live total, chips, recent payments and the 5 tabs", async ({ page }) => {
    await page.goto("/today");
    await expect(page.getByRole("heading", { level: 1, name: "Today" })).toBeVisible();
    // KES 12 400.00 with thin-space separator, from the stub's 1_240_000 cents.
    await expect(page.getByLabel("KES\u200912\u2009400.00").first()).toBeVisible();
    await expect(page.getByText("7 payments")).toBeVisible();
    await expect(page.getByText("6 KRA verified")).toBeVisible();
    await expect(page.getByText("2547•••••345").first()).toBeVisible();

    const nav = page.getByRole("navigation", { name: "Primary" });
    for (const label of ["Today", "Payments", "Invoices", "Attention", "More"]) {
      await expect(nav.getByRole("link", { name: new RegExp(label) })).toBeVisible();
    }
    // Attention badge from the stub's actionable_count.
    await expect(nav.getByRole("link", { name: /Attention/ })).toContainText("2");
    await expect(nav.getByRole("link", { name: /Today/ })).toHaveAttribute("aria-current", "page");
  });

  test("Payments lists the feed with status chips", async ({ page }) => {
    await page.goto("/payments");
    await expect(page.getByRole("heading", { level: 1, name: "Payments" })).toBeVisible();
    await expect(page.getByText("Cash sale").first()).toBeVisible();
    await expect(page.getByText("Unmatched").first()).toBeVisible();
  });

  test("Attention lists the unmatched payment with one fix action", async ({ page }) => {
    await page.goto("/attention");
    await expect(page.getByRole("heading", { level: 1, name: "Needs attention" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Make it a sale" })).toBeVisible();
  });

  test("manifest is installable", async ({ request }) => {
    const res = await request.get("/manifest.webmanifest");
    expect(res.ok()).toBeTruthy();
    const m = (await res.json()) as { name: string; display: string; start_url: string; icons: unknown[] };
    expect(m.name).toBe("CiftPay");
    expect(m.display).toBe("standalone");
    expect(m.start_url).toBe("/today");
    expect(m.icons.length).toBeGreaterThanOrEqual(3);
  });
});
