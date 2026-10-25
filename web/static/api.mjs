export class APIError extends Error {
  constructor(message, status = 0, code = "") {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export function csrfToken(cookie) {
  const entry = cookie.split(";").map(value => value.trim()).find(value => value.startsWith("csrf_token="));
  try { return entry ? decodeURIComponent(entry.slice("csrf_token=".length)) : ""; }
  catch { return ""; }
}

export async function api(path, { method = "GET", data } = {}) {
  const headers = { Accept: "application/json" };
  if (data !== undefined) headers["Content-Type"] = "application/json";
  if (!["GET", "HEAD"].includes(method)) headers["X-CSRF-Token"] = csrfToken(document.cookie);
  let response;
  try {
    response = await fetch(path, {
      method, headers, credentials: "same-origin",
      ...(data !== undefined ? { body: JSON.stringify(data) } : {}),
    });
  } catch {
    throw new APIError("Unable to reach the server. Check your connection and try again.");
  }
  if (response.status === 204) return null;
  let body;
  try { body = await response.json(); }
  catch { throw new APIError("The server returned an unexpected response. Please try again.", response.status); }
  if (!response.ok) {
    const code = body.error?.code || "";
    let message = body.error?.message || "The request could not be completed.";
    if (response.status === 501 || code === "MIDDLEWARE_NOT_CONFIGURED" || code === "NOT_IMPLEMENTED") {
      message = "This feature is not available yet. Please try again later.";
    } else if (response.status === 403 && code === "CSRF_INVALID") {
      message = "Your request could not be verified. Refresh the page and try again.";
    } else if (response.status >= 500 && code !== "SESSION_BUSY") {
      message = "The server could not complete this request. Please try again.";
    }
    throw new APIError(message, response.status, code);
  }
  return body.data;
}

export function displayValue(value) {
  if (value === null) return "NULL";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value ?? "");
}

export function returnPath(candidate, origin) {
  try {
    const url = new URL(candidate || "/databases", origin);
    return url.origin === origin && url.pathname !== "/login" ? url.pathname + url.search + url.hash : "/databases";
  } catch { return "/databases"; }
}

export function qualifiedName(schema, table, engine = "postgres") {
  const quote = engine === "mysql" ? "`" : '"';
  return [schema, table].map(name => quote + String(name).replaceAll(quote, quote + quote) + quote).join(".");
}

export function accessCapabilities(level) {
  if (level === "VIEW_EDIT") return {
    label: "Read + migrations",
    detail: "Browse schema and data, run SELECT queries, review history, and create or execute migrations.",
  };
  if (level === "VIEW_ONLY") return {
    label: "Read only",
    detail: "Browse schema and data, run SELECT queries, and review migration history. Changes require migration access.",
  };
  return { label: "Access unavailable", detail: "Effective database permissions could not be verified." };
}

export function columnKeys(column, constraints = []) {
  const list = constraints || [];
  const keys = new Set(list.filter(item => item.columns?.includes(column)).map(item => item.type.toUpperCase()));
  return [["PRIMARY KEY", "PK"], ["FOREIGN KEY", "FK"], ["UNIQUE", "Unique"]]
    .filter(([type]) => keys.has(type)).map(([, label]) => label).join(", ") || "--";
}
