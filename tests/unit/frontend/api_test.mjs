import test, { afterEach, beforeEach } from "node:test";
import assert from "node:assert/strict";
import {
  api, APIError, accessDescription, accessLabel, databaseAPI, databaseURL,
  displayValue, qualifiedName, returnPath,
} from "../../artifacts/frontend/api.js";

const originalGlobals = new Map(["document", "window"].map(name => [name, Object.getOwnPropertyDescriptor(globalThis, name)]));

beforeEach(() => {
  globalThis.document = { cookie: "" };
  globalThis.window = Object.assign(new EventTarget(), { location: { origin: "https://platform.test" } });
});

afterEach(() => {
  for (const [name, descriptor] of originalGlobals) {
    if (descriptor) Object.defineProperty(globalThis, name, descriptor);
    else delete globalThis[name];
  }
});

test("object names are quoted independently without treating dots as separators", () => {
  assert.equal(qualifiedName('odd"schema', 'order.items'), '"odd""schema"."order.items"');
  assert.equal(qualifiedName('odd`schema', 'order.items', 'mysql'), '`odd``schema`.`order.items`');
});

test("permissions describe migrations without promising unrestricted SQL writes", () => {
  assert.equal(accessLabel("VIEW_EDIT"), "Read + migrations");
  assert.match(accessDescription("VIEW_EDIT"), /SELECT/);
  assert.equal(accessLabel("VIEW_ONLY"), "Read only");
  assert.match(accessDescription("VIEW_ONLY"), /Changes require migration access/);
  assert.match(accessDescription("UNKNOWN"), /could not be verified/);
});

test("database links and API paths encode an identifier as one path segment", () => {
  assert.equal(databaseURL("db/with spaces?#"), "/databases/db%2Fwith%20spaces%3F%23");
  assert.equal(databaseAPI("db/with spaces?#"), "/api/v1/databases/db%2Fwith%20spaces%3F%23");
});

test("redirects preserve SPA search and hash while staying on the current origin", () => {
  assert.equal(returnPath("/databases/db/query?tab=plan#results"), "/databases/db/query?tab=plan#results");
  assert.equal(returnPath("https://platform.test/admin/users"), "/admin/users");
  for (const candidate of [null, "//attacker.test", "javascript:alert(1)", "/login?next=/admin/users", "https://["]) {
    assert.equal(returnPath(candidate), "/databases");
  }
});

test("API mutations send JSON and the separate CSRF cookie with same-origin credentials", async t => {
  document.cookie = "sid=private; csrf_token=abc%2B123; theme=light";
  const controller = new AbortController();
  t.mock.method(globalThis, "fetch", async (path, options) => {
    assert.equal(path, "/api/v1/admin/users");
    assert.equal(options.method, "POST");
    assert.equal(options.credentials, "same-origin");
    assert.equal(options.headers.Accept, "application/json");
    assert.equal(options.headers["Content-Type"], "application/json");
    assert.equal(options.headers["X-CSRF-Token"], "abc+123");
    assert.equal(options.signal, controller.signal);
    assert.deepEqual(JSON.parse(options.body), { username: "alice" });
    return Response.json({ data: { id: "user-1", username: "alice", is_platform_admin: false, is_active: true } });
  });
  assert.deepEqual(await api("/api/v1/admin/users", { method: "POST", data: { username: "alice" }, signal: controller.signal }), {
    id: "user-1", username: "alice", is_platform_admin: false, is_active: true,
  });
});

for (const cookie of ["sid=private", "csrf_token=%invalid"]) {
  test(`missing or malformed CSRF cookie never sends the session identifier: ${cookie}`, async t => {
    document.cookie = cookie;
    t.mock.method(globalThis, "fetch", async (_path, options) => {
      assert.equal(options.headers["X-CSRF-Token"], "");
      return new Response(null, { status: 204 });
    });
    await api("/api/v1/auth/logout", { method: "POST" });
  });
}

test("GET requests omit mutation headers and a request body", async t => {
  document.cookie = "csrf_token=csrf-value";
  t.mock.method(globalThis, "fetch", async (_path, options) => {
    assert.equal(options.method, "GET");
    assert.equal(options.headers["X-CSRF-Token"], undefined);
    assert.equal(options.headers["Content-Type"], undefined);
    assert.equal(options.body, undefined);
    return Response.json({ data: [] });
  });
  assert.deepEqual(await api("/api/v1/admin/users"), []);
});

test("expired sessions notify the SPA instead of replacing the page location", async t => {
  let expirations = 0;
  window.addEventListener("session-expired", () => expirations++);
  t.mock.method(globalThis, "fetch", async () => Response.json({ error: { code: "UNAUTHENTICATED", message: "Session expired" } }, { status: 401 }));
  await assert.rejects(api("/api/v1/admin/users"), error => error instanceof APIError && error.status === 401 && error.code === "UNAUTHENTICATED");
  assert.equal(expirations, 1);
});

test("login and session discovery failures do not broadcast session expiry", async t => {
  let expirations = 0;
  window.addEventListener("session-expired", () => expirations++);
  t.mock.method(globalThis, "fetch", async () => Response.json({ error: { code: "UNAUTHENTICATED", message: "Not signed in" } }, { status: 401 }));
  for (const path of ["/api/v1/auth/login", "/api/v1/auth/me"]) {
    await assert.rejects(api(path), error => error instanceof APIError && error.status === 401);
  }
  assert.equal(expirations, 0);
});

test("unavailable endpoints preserve their status and structured error code", async t => {
  for (const [status, code] of [[501, "NOT_IMPLEMENTED"], [503, "MIDDLEWARE_NOT_CONFIGURED"], [403, "FORBIDDEN"]]) {
    t.mock.method(globalThis, "fetch", async () => Response.json({ error: { code, message: "Unavailable" } }, { status }));
    await assert.rejects(api("/api/v1/admin/users"), error => error instanceof APIError && error.status === status && error.code === code && error.message === "Unavailable");
  }
});

test("204 responses need no JSON body and structured cells retain their values", async t => {
  t.mock.method(globalThis, "fetch", async () => new Response(null, { status: 204 }));
  assert.equal(await api("/api/v1/admin/groups/group-1", { method: "DELETE" }), undefined);
  assert.equal(displayValue(null), "NULL");
  assert.equal(displayValue({ active: true }), '{"active":true}');
  assert.equal(displayValue(false), "false");
  assert.equal(displayValue(0), "0");
});

test("non-JSON responses become API errors with their HTTP status", async t => {
  t.mock.method(globalThis, "fetch", async () => new Response("<html>Unavailable</html>", { status: 502 }));
  await assert.rejects(api("/api/v1/admin/users"), error => error instanceof APIError && error.status === 502);
});

test("network errors are normalized while cancellation propagates unchanged", async t => {
  const failure = new TypeError("fetch failed");
  t.mock.method(globalThis, "fetch", async () => { throw failure; });
  await assert.rejects(api("/api/v1/admin/users"), error => error instanceof APIError && error.status === 0);
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(api("/api/v1/admin/users", { signal: controller.signal }), error => error === failure);
});
