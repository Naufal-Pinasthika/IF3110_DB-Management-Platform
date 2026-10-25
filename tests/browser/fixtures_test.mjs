// UI contract checks using API fixtures; real auth E2E lives in auth_test.mjs.
import assert from "node:assert/strict";
import { artifact } from "./support.mjs";
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "playwright");
const origin = process.env.PREVIEW_ORIGIN || "http://127.0.0.1:28081";
const browser = await chromium.launch({ headless: true, args: ["--no-sandbox"] });
const errors = [];
const db = { id: "db1", label: "Inventory development", database_name: "target_db", effective_access: "VIEW_EDIT" };
let unavailable = false;
let administrator = true;
let access = "VIEW_EDIT";
let migrationStatus = "DRAFT";
let queryUnavailable = false;
let authenticationUnavailable = false;
let holdMigrationSave = false;
let releaseMigrationSave;
let holdMigrationExecute = false;
let releaseMigrationExecute;
const requests = [];
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
await context.tracing.start({ screenshots: true, snapshots: true });
await context.addCookies([{ name: "csrf_token", value: "test-csrf", url: origin }]);
await context.route("**/api/v1/**", async route => {
  const request = route.request();
  const path = new URL(request.url()).pathname;
  const method = request.method();
  const data = request.postDataJSON();
  requests.push({ path, method, data, csrf: request.headers()["x-csrf-token"] });
  let result;
  let status = 200;
  if (path === "/api/v1/auth/me") {
    if (authenticationUnavailable) return route.fulfill({ status: 500, contentType: "application/json", body: JSON.stringify({ error: { code: "INTERNAL_ERROR", message: "Server error" } }) });
    result = { id: "u1", username: "alex", is_platform_admin: administrator };
  }
  else if (path === "/api/v1/auth/login") result = { id: "u1", username: "alex", is_platform_admin: true };
  else if (path === "/api/v1/auth/logout") result = { success: true };
  else if (path === "/api/v1/databases") {
    if (unavailable) return route.fulfill({ status: 501, contentType: "application/json", body: JSON.stringify({ error: { code: "NOT_IMPLEMENTED", message: "Not configured" } }) });
    result = [db, { id: "db2", label: "Analytics reporting", database_name: "analytics_db", effective_access: "VIEW_ONLY" }];
  } else if (path === "/api/v1/databases/db1") result = { ...db, effective_access: access };
  else if (path.endsWith("/schemas")) result = [{ name: "public" }, { name: "audit" }];
  else if (path.endsWith("/tables")) result = [{ name: "customers" }, { name: "orders" }];
  else if (path.endsWith("/tables/customers")) result = { schema: "public", table: "customers", columns: [{ name: "id", data_type: "uuid", nullable: false }, { name: "name", data_type: "text", nullable: false }, { name: "email", data_type: "text", nullable: true }], constraints: [{ name: "customers_pkey", type: "PRIMARY KEY", columns: ["id"] }], indexes: [{ name: "customers_pkey", columns: ["id"], is_unique: true }] };
  else if (path.endsWith("/rows")) result = { columns: ["id", "name", "email"], rows: [["c1", "Nia", null], ["c2", "<img src=x onerror=window.injected=true>", "test@example.com"]], limit: 50, offset: 0, has_more: false };
  else if (path.endsWith("/queries")) {
    if (queryUnavailable) return route.fulfill({ status: 501, contentType: "application/json", body: JSON.stringify({ error: { code: "NOT_IMPLEMENTED", message: "Not configured" } }) });
    result = { columns: ["answer"], rows: [[42]], row_count: 1, truncated: false };
  }
  else if (path.endsWith("/execute")) {
    if (holdMigrationExecute) await new Promise(resolve => { releaseMigrationExecute = resolve; });
    migrationStatus = "APPLIED"; result = { id: "m1", status: migrationStatus, version: 1 };
  }
  else if (path.endsWith("/migrations/m1")) {
    if (method === "PATCH" && holdMigrationSave) await new Promise(resolve => { releaseMigrationSave = resolve; });
    result = { id: "m1", name: "Add customer email", database_id: "db1", sql_text: "ALTER TABLE customers ADD COLUMN email TEXT;", status: migrationStatus, version: migrationStatus === "DRAFT" ? null : 1, executed_by: migrationStatus === "DRAFT" ? null : "alex", executed_at: migrationStatus === "DRAFT" ? null : "2026-10-25T04:00:00Z", error_summary: null };
  }
  else if (path.endsWith("/migrations")) result = method === "POST" ? { id: "m1", status: "DRAFT" } : [{ id: "m1", name: "Add customer email", status: migrationStatus, version: null, executed_by: null, executed_at: null }];
  else if (path === "/api/v1/admin/users") result = method === "GET" ? [{ id: "u1", username: "alex", is_platform_admin: true, is_active: true }, { id: "u2", username: "nia", is_platform_admin: false, is_active: true }] : { id: "u3", ...data };
  else if (path === "/api/v1/admin/groups") result = method === "GET" ? [{ id: "g1", name: "Developers" }] : { id: "g2", ...data };
  else if (path === "/api/v1/admin/databases") result = [{ id: "db1", label: db.label, database_name: db.database_name, is_enabled: true }];
  else if (path === "/api/v1/admin/databases/db1") result = { id: "db1", label: db.label, host: "localhost", port: 5432, database_name: db.database_name, ssl_mode: "require", reader_username: "reader", migrator_username: "migrator", is_enabled: true };
  else if (method === "DELETE") { status = 204; result = null; }
  else result = data || {};
  await route.fulfill({ status, contentType: "application/json", ...(status === 204 ? {} : { body: JSON.stringify({ data: result }) }) });
});
const page = await context.newPage();
page.setDefaultTimeout(10000);
page.on("pageerror", error => errors.push(error.message));
async function visit(path, ready) {
  await page.goto(origin + path);
  if (path !== "/login") await page.locator("#account-name").filter({ hasText: "alex" }).waitFor();
  if (ready) await page.locator(ready).first().waitFor();
}
try {
  const nativeContext = await browser.newContext({ javaScriptEnabled: false });
  const nativePage = await nativeContext.newPage();
  await nativePage.goto(origin + "/login");
  await nativePage.fill("#username", "fixture-user");
  await nativePage.fill("#password", "fixture-password");
  const nativeRequest = nativePage.waitForRequest(request => request.isNavigationRequest());
  await nativePage.click('#login-form [type="submit"]');
  const fallback = await nativeRequest;
  assert.equal(fallback.method(), "POST", "password forms must never fall back to GET");
  assert.equal(new URL(fallback.url()).search, "");
  await nativeContext.close();
  await visit("/login", "#login-form");
  assert.equal(await page.locator(".login-intro-copy").count(), 0, "sign-in should not contain marketing content");
  await page.click("#password-toggle");
  assert.equal(await page.locator("#password").getAttribute("type"), "text");
  await page.click("#password-toggle");
  await page.screenshot({ path: artifact("login.png"), fullPage: true });
  await page.fill("#username", "alex"); await page.fill("#password", "test-password");
  await page.click('#login-form [type="submit"]');
  await page.waitForURL("**/databases");
  await page.getByRole("link", { name: "Inventory development", exact: true }).waitFor();
  assert.equal(await page.locator(".eyebrow").count(), 0, "page headings must not have decorative micro-headings");
  assert.equal(await page.locator(".content-footer").count(), 0);
  await page.screenshot({ path: artifact("databases.png"), fullPage: true });
  await page.fill("#database-filter", "no-match");
  assert.match(await page.locator("#database-list").textContent(), /No databases match/);
  await page.fill("#database-filter", "");

  unavailable = true;
  await visit("/databases");
  await page.getByText("This feature is not available yet. Please try again later.").waitFor();
  unavailable = false;
  await page.click("#retry");
  await page.getByRole("link", { name: "Inventory development", exact: true }).waitFor();

  await visit("/databases/db1", "#schema-list summary");
  await page.getByRole("link", { name: "customers", exact: true }).waitFor();
  await page.locator("#schema-list summary").filter({ hasText: "audit" }).click();
  await page.locator("#schema-name").filter({ hasText: "audit" }).waitFor();
  await page.click("#refresh");
  await page.locator("#schema-name").filter({ hasText: "audit" }).waitFor();
  await visit("/databases/db1/schemas/public/tables/customers");
  await page.locator("#rows-table td").first().waitFor();
  await page.locator('#schema-list a[aria-current="page"]').filter({ hasText: "customers" }).waitFor();
  await page.fill("#object-filter", "orders");
  await page.getByRole("link", { name: "audit.orders", exact: true }).waitFor();
  assert.equal(await page.getByRole("link", { name: "public.customers", exact: true }).isVisible(), false);
  await page.fill("#object-filter", "");
  assert.match(await page.locator("#rows-table").textContent(), /NULL/);
  assert.equal(await page.evaluate(() => window.injected), undefined);
  await page.click("#structure-trigger");
  await page.getByText("customers_pkey").first().waitFor();
  await page.locator("#column-list").getByText("PK", { exact: true }).waitFor();
  assert.ok((await page.locator("#column-list tr").first().boundingBox()).height <= 48);
  await page.screenshot({ path: artifact("table.png"), fullPage: true });
  await page.getByRole("link", { name: "Open in SQL", exact: true }).click();
  await page.waitForFunction(() => document.getElementById("sql").value.includes('"public"."customers"'));
  assert.equal(requests.filter(request => request.path.endsWith("/queries")).length, 0, "opening an object in SQL must not execute it");

  await visit("/databases/db1/query");
  await page.fill("#sql", "SELECT 42 AS answer;");
  queryUnavailable = true;
  await page.click('#query-form [type="submit"]');
  await page.getByText("This feature is not available yet. Please try again later.").waitFor();
  assert.equal(await page.locator("#retry").isVisible(), false);
  assert.equal(await page.inputValue("#sql"), "SELECT 42 AS answer;");
  queryUnavailable = false;
  await page.click('#query-form [type="submit"]');
  await page.locator("#query-summary").filter({ hasText: /^1 row ·/ }).waitFor();
  assert.match(await page.locator("#query-results").textContent(), /42/);
  const query = requests.find(request => request.path.endsWith("/queries"));
  assert.deepEqual(query.data, { sql: "SELECT 42 AS answer;", max_rows: 50 });
  assert.equal(query.csrf, "test-csrf");

  await visit("/databases/db1/migrations/m1");
  await page.waitForFunction(() => !document.getElementById("sql").readOnly);
  await page.fill("#sql", "ALTER TABLE customers ADD COLUMN verified BOOLEAN;");
  assert.equal(await page.locator("#execute-migration").isDisabled(), true);
  holdMigrationSave = true;
  await page.click("#save-migration");
  await page.waitForFunction(() => document.getElementById("save-migration").disabled);
  assert.equal(await page.locator("#sql").isDisabled(), true, "SQL must not change while its draft is being saved");
  releaseMigrationSave(); holdMigrationSave = false;
  await page.getByText("Migration draft saved.", { exact: true }).waitFor();
  assert.equal(await page.locator("#execute-migration").isDisabled(), false);
  await page.click("#execute-migration");
  await page.locator("#confirm-dialog[open]").waitFor();
  assert.match(await page.locator("#confirm-description").textContent(), /target_db/);
  holdMigrationExecute = true;
  await page.click("#confirm-button");
  await page.waitForFunction(() => document.getElementById("sql").readOnly);
  assert.equal(await page.locator("#save-migration").isDisabled(), true);
  releaseMigrationExecute(); holdMigrationExecute = false;
  await page.locator("#migration-status").getByText("Applied", { exact: true }).waitFor();
  assert.equal(await page.locator("#sql").getAttribute("readonly"), "");

  access = "VIEW_ONLY";
  await visit("/databases/db1/migrations/new");
  await page.waitForFunction(() => document.getElementById("access-badge").textContent === "Read only");
  assert.equal(await page.locator("#save-migration").isVisible(), false);
  assert.equal(await page.locator("#sql").getAttribute("readonly"), "");
  access = "VIEW_EDIT";

  await visit("/admin/users");
  await page.getByRole("cell", { name: "nia", exact: true }).waitFor();
  await page.getByRole("button", { name: "New user", exact: true }).click();
  await page.fill("#user-username", "sam"); await page.fill("#user-password", "safe-password");
  await page.click('#user-form [type="submit"]');
  await page.getByText("User created.", { exact: true }).waitFor();
  const userRequest = requests.find(request => request.path === "/api/v1/admin/users" && request.method === "POST");
  assert.deepEqual(userRequest.data, { username: "sam", password: "safe-password", is_platform_admin: false });

  await visit("/admin/groups");
  await page.selectOption("#member-group", "g1"); await page.selectOption("#member-user", "u2");
  await page.click('#membership-form [type="submit"]');
  await page.getByText("Member added to group.", { exact: true }).waitFor();
  await page.selectOption("#grant-group", "g1"); await page.selectOption("#grant-database", "db1");
  await page.selectOption("#grant-level", "VIEW_EDIT"); await page.click('#grant-form [type="submit"]');
  await page.getByText("Database access updated.", { exact: true }).waitFor();
  assert.ok(requests.some(request => request.path === "/api/v1/admin/groups/g1/databases/db1/access" && request.method === "PUT" && request.data.access_level === "VIEW_EDIT"));

  await visit("/admin/databases");
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await page.locator("#connection-dialog[open]").waitFor();
  assert.equal(await page.inputValue("#reader-password"), "");
  await page.click('#connection-form [type="submit"]');
  await page.getByText("Connection saved.", { exact: true }).waitFor();
  const edit = requests.find(request => request.path === "/api/v1/admin/databases/db1" && request.method === "PATCH");
  assert.equal(edit.data.port, 5432);
  assert.equal("reader_password" in edit.data, false);

  administrator = false;
  await visit("/admin/users");
  await page.getByText("Administrator access is required to manage this section.").waitFor();
  assert.equal(await page.locator("#user-form").count(), 0);
  administrator = true;

  await page.setViewportSize({ width: 390, height: 844 });
  await visit("/databases");
  await page.getByRole("link", { name: "Inventory development", exact: true }).waitFor();
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  for (const button of await page.locator(".open-database").all()) {
    const bounds = await button.boundingBox();
    assert.ok(bounds && bounds.x >= 0 && bounds.x + bounds.width <= 390, "Open must be visible without horizontal scrolling");
  }
  await page.click("#navigation-toggle");
  assert.equal(await page.locator("#navigation-toggle").getAttribute("aria-expanded"), "true");
  await page.keyboard.press("Escape");
  assert.equal(await page.locator("#navigation-toggle").getAttribute("aria-expanded"), "false");
  await page.screenshot({ path: artifact("mobile.png"), fullPage: true });
  const originalLabel = db.label;
  const originalName = db.database_name;
  db.label = "Production_".repeat(8);
  db.database_name = "customer_".repeat(7);
  await visit("/databases");
  await page.getByRole("link", { name: db.label, exact: true }).waitFor();
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, "long identifiers must not push Open off screen");
  db.label = originalLabel;
  db.database_name = originalName;
  await visit("/databases/db1/schemas/public/tables/customers");
  await page.locator("#rows-table td").first().waitFor();
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  assert.ok(await page.locator("#rows-table").evaluate(element => element.scrollWidth > element.clientWidth), "wide technical grids must scroll within their own region");
  await page.screenshot({ path: artifact("mobile-table.png"), fullPage: true });
  authenticationUnavailable = true;
  await page.goto(origin + "/databases");
  await page.getByText("The server could not complete this request. Please try again.").waitFor();
  authenticationUnavailable = false;
  await page.click("#retry");
  await page.getByRole("link", { name: "Inventory development", exact: true }).waitFor();
  assert.deepEqual(errors, []);
  console.log("Browser checks passed: login, filtering, unavailable/retry, schema/table and escaping, query + CSRF, migration save/execute/read-only, admin forms, access denial, mobile layout.");
} catch (error) {
  await page.screenshot({ path: artifact("fixtures-failure.png"), fullPage: true });
  throw error;
} finally {
  await context.tracing.stop({ path: artifact("fixtures-trace.zip") });
  await browser.close();
}
