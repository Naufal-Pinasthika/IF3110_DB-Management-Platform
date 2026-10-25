// Full browser -> production router -> PostgreSQL auth flow, without API mocks.
import assert from "node:assert/strict";
import { artifact } from "./support.mjs";

const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "playwright");
const origin = process.env.PREVIEW_ORIGIN;
assert.ok(origin, "PREVIEW_ORIGIN must be supplied by the Go integration test");
const browser = await chromium.launch({ headless: true, args: ["--no-sandbox"] });
const context = await browser.newContext();
await context.tracing.start({ screenshots: true, snapshots: true });
const page = await context.newPage();
page.setDefaultTimeout(10000);
const errors = [];
page.on("pageerror", error => errors.push(error.message));

try {
  await page.goto(origin + "/databases");
  await page.waitForURL("**/login?next=*");
  await page.fill("#username", "browser-user");
  await page.fill("#password", "wrong-password");
  const failedLogin = page.waitForResponse(response => response.url().endsWith("/api/v1/auth/login"));
  await page.click('#login-form [type="submit"]');
  assert.equal((await failedLogin).status(), 401);
  await page.locator("#login-error:not([hidden])").waitFor();
  assert.equal((await context.cookies()).some(cookie => cookie.name === "sid"), false);

  await page.fill("#password", "browser-password");
  await page.click('#login-form [type="submit"]');
  await page.waitForURL("**/databases");
  await page.locator("#account-name").filter({ hasText: "browser-user" }).waitFor();
  // The unfinished database feature must stay explicitly unavailable after login.
  await page.getByText("This feature is not available yet. Please try again later.").waitFor();
  const cookies = await context.cookies();
  const sid = cookies.find(cookie => cookie.name === "sid");
  const csrf = cookies.find(cookie => cookie.name === "csrf_token");
  assert.ok(sid?.httpOnly);
  assert.ok(csrf && !csrf.httpOnly);
  assert.equal(sid.sameSite, "Lax");
  assert.equal(await page.evaluate(() => document.cookie.includes("sid=")), false);
  const me = await context.request.get(origin + "/api/v1/auth/me");
  assert.equal(me.status(), 200);
  assert.equal((await me.json()).data.username, "browser-user");

  const rejected = await context.request.post(origin + "/api/v1/auth/logout", { headers: { Origin: origin } });
  assert.equal(rejected.status(), 403, "logout must reject missing CSRF");
  assert.equal((await context.request.get(origin + "/api/v1/auth/me")).status(), 200);
  // A non-admin account must see the access-denied state in the actual frontend.
  await page.goto(origin + "/admin/users");
  await page.getByText("Administrator access is required to manage this section.").waitFor();

  const logoutResponse = page.waitForResponse(response => response.url().endsWith("/api/v1/auth/logout"));
  await page.click("#logout");
  const logout = await logoutResponse;
  assert.equal(logout.status(), 200);
  assert.equal(logout.request().headers()["x-csrf-token"], csrf.value);
  await page.waitForURL("**/login");
  assert.equal((await context.cookies()).some(cookie => ["sid", "csrf_token"].includes(cookie.name)), false);
  assert.equal((await context.request.get(origin + "/api/v1/auth/me")).status(), 401);
  // Replaying the captured session cookie after logout must fail server-side too.
  const replay = await browser.newContext();
  try {
    await replay.addCookies([{ name: "sid", value: sid.value, url: origin }]);
    assert.equal((await replay.request.get(origin + "/api/v1/auth/me")).status(), 401);
  } finally { await replay.close(); }
  await page.goto(origin + "/databases");
  await page.waitForURL("**/login?next=*");
  assert.deepEqual(errors, []);
  console.log("Real auth browser checks passed: invalid/valid login, cookies, me, unavailable feature, admin denial, CSRF, logout, and revoked-cookie replay.");
} catch (error) {
  await page.screenshot({ path: artifact("auth-failure.png"), fullPage: true });
  throw error;
} finally {
  await context.tracing.stop({ path: artifact("auth-trace.zip") });
  await browser.close();
}
