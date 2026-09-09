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

// 1×1 transparent PNG; the api only checks type and size.
const letterPng = {
  name: "letter.png",
  mimeType: "image/png",
  buffer: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==", "base64"),
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

async function addTill(page: Page, number: string) {
  await expect(page.getByText("Step 2 of 3")).toBeVisible();
  await expect(page.getByRole("heading", { level: 1, name: "Where do customers pay you?" })).toBeVisible();
  await page.getByLabel("Type").selectOption("till");
  await page.getByLabel("Number").fill(number);
  await page.getByRole("button", { name: "Add number" }).click();
}

test.describe("onboarding", () => {
  test("a new user walks business → till → authorization letter → Today", async ({ page }) => {
    await signInAsNewUser(page);
    await expect(page.getByText("Step 1 of 3")).toBeVisible();

    await fillBusiness(page, "P123456789K");
    await addTill(page, "600123");

    // Step 3: the administrative gate. Instructions, printable letter, upload.
    await expect(page.getByText("Step 3 of 3")).toBeVisible();
    await expect(page.getByRole("heading", { level: 1, name: "Authorization document" })).toBeVisible();
    await expect(page.getByText("Print the CiftPay Safaricom Authorization Letter.")).toBeVisible();
    await expect(page.getByText("Sign it as the registered owner.")).toBeVisible();
    await expect(page.getByText("Add the business stamp.")).toBeVisible();
    await expect(page.getByText("Take a clear photo (or scan to PDF) and upload it here.")).toBeVisible();
    await expect(page.getByText(/usually takes 1–3 business days/)).toBeVisible();
    await expect(page.getByText(/KES 1/)).toHaveCount(0);

    const printLink = page.getByRole("link", { name: "Print the letter" });
    await expect(printLink).toHaveAttribute("target", "_blank");
    const href = await printLink.getAttribute("href");
    expect(href).toMatch(/^\/onboarding\/letter\?shortcode=[0-9a-f-]{36}$/);

    // The letter is pre-filled with the org and the shortcode; the print button stays out of print.
    await page.goto(href!);
    await expect(page.getByRole("heading", { level: 1, name: "Authorization Letter — Safaricom Daraja API (C2B) Integration" })).toBeVisible();
    const letter = page.getByRole("article", { name: "Authorization Letter" });
    await expect(letter).toContainText("Kamau Hardware");
    await expect(letter).toContainText("Till");
    await expect(letter).toContainText("600123");
    await expect(letter).toContainText("Business stamp");
    const printButton = page.getByRole("button", { name: "Print / Save as PDF" });
    await expect(printButton).toBeVisible();
    await page.emulateMedia({ media: "print" });
    await expect(printButton).toBeHidden();
    await expect(letter).toBeVisible();
    await page.emulateMedia({ media: "screen" });

    // Back on onboarding the flow resumes at step 3 for the same Till.
    await page.goto("/onboarding");
    await expect(page.getByText("Step 3 of 3")).toBeVisible();
    const submit = page.getByRole("button", { name: "Submit for verification" });
    await expect(submit).toBeDisabled();
    await page.locator('input[type="file"]').setInputFiles(letterPng);
    await expect(page.getByText("letter.png")).toBeVisible();
    await expect(submit).toBeEnabled();
    await submit.click();
    await expect(page).toHaveURL(/\/today/);
    await expect(page.getByRole("heading", { level: 1, name: "Today" })).toBeVisible();

    // With the letter uploaded there is nothing left to onboard.
    await page.goto("/onboarding");
    await expect(page).toHaveURL(/\/today/);
  });

  test("a wrong file type is refused before upload", async ({ page }) => {
    await signInAsNewUser(page);
    await fillBusiness(page, "P323456789K");
    await addTill(page, "600124");
    await page.locator('input[type="file"]').setInputFiles({ name: "letter.txt", mimeType: "text/plain", buffer: Buffer.from("hello") });
    await expect(page.getByText("That file type won’t work.")).toBeVisible();
    await expect(page.getByRole("button", { name: "Submit for verification" })).toBeDisabled();
  });

  test("Do this later skips the letter and opens Today", async ({ page }) => {
    await signInAsNewUser(page);
    await fillBusiness(page, "P423456789K");
    await addTill(page, "600125");
    await expect(page.getByText("Step 3 of 3")).toBeVisible();
    await page.getByRole("button", { name: "Do this later" }).click();
    await expect(page).toHaveURL(/\/today/);
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

  test("rows show the three authorization states with the rejection reason", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible();

    const verified = page.locator("li", { hasText: "123456" }).first();
    await expect(verified).toContainText("Verified");
    await expect(verified).toContainText("C2B connected");
    await expect(verified.getByRole("button")).toHaveCount(0);

    const pending = page.locator("li", { hasText: "654321" }).first();
    await expect(pending).toContainText("Pending authorization");
    await expect(pending.getByRole("button", { name: "Upload letter" })).toBeVisible();

    const rejected = page.locator("li", { hasText: "111222" }).first();
    await expect(rejected).toContainText("Rejected");
    await expect(rejected).toContainText("Stamp missing");
    await expect(rejected.getByRole("button", { name: "Upload letter" })).toBeVisible();

    await expect(page.getByRole("button", { name: "Verify", exact: true })).toHaveCount(0);
  });

  test("Add opens a sheet, creates a Paybill and moves straight to the letter step", async ({ page }) => {
    await page.goto("/settings");
    await expect(page.getByRole("heading", { level: 1, name: "Settings" })).toBeVisible();
    await page.getByRole("button", { name: "Add", exact: true }).click();

    const sheet = page.getByRole("dialog");
    await expect(sheet.getByRole("heading", { name: "Add a Till or Paybill" })).toBeVisible();
    await sheet.getByLabel("Type").selectOption("paybill");
    await sheet.getByLabel("Number").fill("400200");
    await sheet.getByLabel("Label (optional)").fill("Shop");
    await sheet.getByRole("button", { name: "Add number" }).click();

    await expect(sheet.getByRole("heading", { name: "Authorization letter" })).toBeVisible();
    await expect(sheet.getByText("Print the CiftPay Safaricom Authorization Letter.")).toBeVisible();
    await expect(sheet.getByRole("link", { name: "Print the letter" })).toHaveAttribute("href", /^\/onboarding\/letter\?shortcode=/);
    await sheet.locator('input[type="file"]').setInputFiles(letterPng);
    await sheet.getByRole("button", { name: "Submit for verification" }).click();
    await expect(sheet).toBeHidden();

    // Back on the list, the new row is pending with the letter in (stub state is
    // shared across the mobile/desktop projects, so there may be more than one 400200 row).
    const row = page.locator("li", { hasText: "400200" }).first();
    await expect(row).toContainText("Pending authorization");
    await expect(row).toContainText("Letter uploaded · waiting for Safaricom");
    await expect(row.getByRole("button")).toHaveCount(0);
  });
});
