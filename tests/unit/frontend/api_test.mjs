import test from "node:test";
import assert from "node:assert/strict";
import { api, csrfToken, displayValue, returnPath, qualifiedName, accessCapabilities, columnKeys } from "../../../web/static/api.mjs";

test("object names are quoted independently without treating dots as separators", () => {
  assert.equal(qualifiedName('odd"schema', 'order.items'), '"odd""schema"."order.items"');
});

test("permissions describe migrations without promising unrestricted SQL writes", () => {
  assert.equal(accessCapabilities("VIEW_EDIT").label, "Read + migrations");
  assert.match(accessCapabilities("VIEW_EDIT").detail, /SELECT/);
  assert.equal(accessCapabilities("VIEW_ONLY").label, "Read only");
  assert.equal(accessCapabilities("UNKNOWN").label, "Access unavailable");
});

test("key markers come from actual constraint metadata", () => {
  assert.equal(columnKeys("id", []), "--");
  assert.equal(columnKeys("id", [{type:"PRIMARY KEY", columns:["id"]}]), "PK");
  assert.equal(columnKeys("customer_id", [{type:"FOREIGN KEY", columns:["customer_id"]}, {type:"UNIQUE", columns:["customer_id"]}]), "FK, Unique");
});

test("CSRF extraction uses the separate readable cookie", () => {
  assert.equal(csrfToken("sid=secret; csrf_token=abc%2B123; theme=light"), "abc+123");
  assert.equal(csrfToken("sid=secret"), "");
  assert.equal(csrfToken("csrf_token=%invalid"), "");
});

test("redirects remain on the current origin", () => {
  assert.equal(returnPath("/databases/db/query", "https://platform.test"), "/databases/db/query");
  assert.equal(returnPath("//attacker.test", "https://platform.test"), "/databases");
  assert.equal(returnPath("javascript:alert(1)", "https://platform.test"), "/databases");
  assert.equal(returnPath("/login", "https://platform.test"), "/databases");
});

test("API mutations send JSON and CSRF with same-origin cookies", async () => {
  globalThis.document = { cookie: "sid=private; csrf_token=csrf-value" };
  globalThis.fetch = async (path, options) => {
    assert.equal(path, "/api/v1/databases/db/queries");
    assert.equal(options.credentials, "same-origin");
    assert.equal(options.headers["X-CSRF-Token"], "csrf-value");
    assert.deepEqual(JSON.parse(options.body), { sql: "SELECT 1", max_rows: 50 });
    return new Response(JSON.stringify({ data: { rows: [[1]] } }), { status: 200 });
  };
  assert.deepEqual(await api("/api/v1/databases/db/queries", { method: "POST", data: { sql: "SELECT 1", max_rows: 50 } }), { rows: [[1]] });
});

test("unfinished endpoints and authentication errors never become success", async () => {
  for (const [status, code] of [[501, "NOT_IMPLEMENTED"], [503, "MIDDLEWARE_NOT_CONFIGURED"], [401, "UNAUTHENTICATED"]]) {
    globalThis.fetch = async () => new Response(JSON.stringify({ error: { code, message: "Unavailable" } }), { status });
    await assert.rejects(api("/api/v1/databases"), error => error.status === status && error.code === code);
  }
});

test("204 deletion and structured cell values are supported", async () => {
  globalThis.fetch = async () => new Response(null, { status: 204 });
  assert.equal(await api("/api/v1/admin/groups/id", { method: "DELETE" }), null);
  assert.equal(displayValue(null), "NULL");
  assert.equal(displayValue({ active: true }), '{"active":true}');
  assert.equal(displayValue(false), "false");
});
