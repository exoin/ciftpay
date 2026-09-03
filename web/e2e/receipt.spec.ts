import { expect, test } from "@playwright/test";

// The public receipt must work with JavaScript disabled (design-system §11).
test.use({ javaScriptEnabled: false });

test("/r/<code> renders the receipt server-side without JS", async ({ page }) => {
  await page.goto("/r/7KQ2M9");
  await expect(page.getByRole("article", { name: /Tax invoice 7KQ2M9/ })).toBeVisible();
  await expect(page.getByText("Mama Njeri Groceries")).toBeVisible();
  await expect(page.getByText("PIN A123456789B")).toBeVisible();
  await expect(page.getByText("KRAMW0100000001/2026")).toBeVisible();
  await expect(page.getByText("Sukari 2kg")).toBeVisible();
  await expect(page.getByRole("status")).toHaveText("KRA verified");
  await expect(page.getByLabel("KES\u2009820.00")).toBeVisible();
  // QR is inline SVG rendered on the server.
  await expect(page.locator("article svg").first()).toBeAttached();
  await expect(page.getByRole("link", { name: /Save to phone/ })).toHaveAttribute("download", "ciftpay-7KQ2M9.txt");
  await expect(page.getByRole("link", { name: /Issue your own eTIMS receipts/ })).toBeVisible();
});

test("/r/<code> HTML document stays small", async ({ request }) => {
  const res = await request.get("/r/7KQ2M9");
  expect(res.ok()).toBeTruthy();
  const html = await res.text();
  // Budget for the document itself (N6). CSS/fonts are shared, cached assets.
  expect(html.length).toBeLessThan(30 * 1024);
});

test("unknown code shows the receipt-shaped not-found", async ({ page }) => {
  const res = await page.goto("/r/ZZZZZZ");
  expect(res?.status()).toBe(404);
  await expect(page.getByRole("heading", { name: "No receipt with that code." })).toBeVisible();
});

test("Swahili via cookie", async ({ page, context }) => {
  await context.addCookies([{ name: "ciftpay_locale", value: "sw", url: "http://localhost" }]);
  await page.goto("/r/7KQ2M9");
  await expect(page.getByRole("status")).toHaveText("KRA imethibitisha");
});
