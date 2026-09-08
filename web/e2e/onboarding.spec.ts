import { expect, test, type Page } from "@playwright/test";

// The stub treats this phone as a brand-new user (session with no orgs).
const NEW_PHONE = "0700 000 001";

const existingSession = {
  user_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  csrf_token: "csrf-test",
  expires_at: new Date(Date.now() + 3600_000).toISOString(),
  msisdn_masked: "2547•••••678",
  orgs: [{ org_id: "0f0f0f0f-0f0f-4f0f-8f0f-0f0f0f0f0f0f", name: "Mama Njeri Groceries", role: "owner", is_default: true }],
};

async function signInAsNewUser(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Phone number").fill(NEW_PHONE);
  await page.getByRole("button", { name: "Text me a code" }).click();
  await page.getByLabel("6-digit code").fill("123456");
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page).toHaveURL(/\/onboarding/);
}

async function fillBusiness(page: Page, pin: string) {
  await expect(page.getByRole("heading", { level: 1, name: "Your business" })).toBeVisible();
  await page.getByLabel("Business name").fill("Kamau Hardware");
  await page.getByLabel("KRA PIN").fill(pin);
  await page.getByRole("button", { name: "Save business" }).click();
}

test.describe("onboarding", () => {
  test("a new user is sent to /onboarding and walks business → till → KES 1 → Today", async ({ page }) => {
    await signInAsNewUser(page);
    await expect(page.getByText("Step 1 of 3")).toBeVisible();

    await fillBusiness(page, "P123456789K");

    // Step 2: the Till.
    await expect(page.getByText("Step 2 of 3")).toBeVisible();
    await expect(page.getByRole("heading", { level: 1, name: "Where do customers pay you?" })).toBeVisible();
    await page.getByLabel("Type").selectOption("till");
    await page.getByLabel("Number").fill("600123");
    await page.getByRole("button", { name: "Add number" }).click();

    // Step 3: the control check. 202 → instruction card + countdown, then the
    // stub reports verified on the second poll (~6 s at 3 s intervals).
    await expect(page.getByText("Step 3 of 3")).toBeVisible();
    await expect(page.getByRole("heading", { level: 1, name: "Pay KES 1 to your own Till" })).toBeVisible();
    await expect(page.getByText("Till number: 600123")).toBeVisible();
    await expect(page.getByText("Amount: KES 1")).toBeVisible();
    await expect(page.getByText(/\d+:\d\d left/)).toBeVisible();
    await expect(page.getByRole("button", { name: "Do this later" })).toBeVisible();

    await expect(page.getByText("Verified. Payments to 600123 now become KRA receipts.")).toBeVisible({ timeout: 15_000 });
    await page.getByRole("button", { name: "Go to Today" }).click();
    await expect(page).toHaveURL(/\/today/);
    await expect(page.getByRole("heading", { level: 1, name: "Today" })).toBeVisible();
  });

  test("an unknown KRA PIN and a taken PIN get specific messages", async ({ page }) => {
    await signInAsNewUser(page);
    await fillBusiness(page, "P000000000Z");
    // Next's route announcer is also role=alert, so match on the copy.
    await expect(page.getByText("KRA doesn't recognise this PIN")).toBeVisible();

    await page.getByLabel("KRA PIN").fill("P051234567X");
    await page.getByRole("button", { name: "Save business" }).click();
    await expect(page.getByText("already on CiftPay")).toBeVisible();

    // Client-side format check never hits the api.
    await page.getByLabel("KRA PIN").fill("12345");
    await page.getByRole("button", { name: "Save business" }).click();
    await expect(page.getByText("That isn't a KRA PIN.")).toBeVisible();
  });

  test("a number already verified by another business is refused with a clear message", async ({ page }) => {
    await signInAsNewUser(page);
    await fillBusiness(page, "P223456789K");
    await page.getByLabel("Number").fill("999999");
    await page.getByRole("button", { name: "Add number" }).click();
    await expect(page.getByText("already verified by another business")).toBeVisible();
    // Still on step 2; the merchant can try another number.
    await expect(page.getByText("Step 2 of 3")).toBeVisible();
  });

  test("a signed-in user without a business is redirected from the app shell", async ({ page }) => {
    await page.addInitScript((s) => {
      window.sessionStorage.setItem("ciftpay.session", JSON.stringify(s));
      window.sessionStorage.setItem("ciftpay.csrf", s.csrf_token);
    }, { ...existingSession, orgs: [] });
    await page.goto("/today");
    await expect(page).toHaveURL(/\/onboarding/);
    await expect(page.getByRole("heading", { level: 1, name: "Your business" })).toBeVisible();
  });

  test("signed-out visit to /onboarding goes to login with next", async ({ page }) => {
    await page.goto("/onboarding");
    await expect(page).toHaveURL(/\/login\?next=%2Fonboarding/);
  });
});

test.describe("settings shortcodes", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript((s) => {
      window.sessionStorage.setItem("ciftpay.session", JSON.stringify(s));
      window.sessionStorage.setItem("ciftpay.csrf", s.csrf_token);
      window.localStorage.setItem("ciftpay.org", s.orgs[0]!.org_id);
    }, existingSession);
  });

  test("Add opens a sheet, creates a Paybill and moves straight to the KES 1 check", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible();
    await page.getByRole("button", { name: "Add", exact: true }).click();

    const sheet = page.getByRole("dialog");
    await expect(sheet.getByRole("heading", { name: "Add a Till or Paybill" })).toBeVisible();
    await sheet.getByLabel("Type").selectOption("paybill");
    await sheet.getByLabel("Number").fill("400200");
    await sheet.getByLabel("Label (optional)").fill("Shop");
    await sheet.getByRole("button", { name: "Add number" }).click();

    await expect(sheet.getByRole("heading", { name: "Prove it's yours" })).toBeVisible();
    await expect(sheet.getByText("Business number: 400200")).toBeVisible();
    await expect(sheet.getByText("Account number: CIFTPAY")).toBeVisible();
    await expect(sheet.getByText("Verified. Payments to 400200 now become KRA receipts.")).toBeVisible({ timeout: 15_000 });
    await sheet.getByRole("button", { name: "Done" }).click();

    // Back on the list, the new row shows verified (stub state is shared across
    // the mobile/desktop projects, so there may be more than one 400200 row).
    const row = page.locator("li", { hasText: "400200" }).first();
    await expect(row).toContainText("Verified");
    await expect(row).toContainText("M-Pesa confirmations connected");
  });
});
