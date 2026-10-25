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

let redirectingToLogin = false;

function redirectOnUnauthorized(status) {
  if (status !== 401 || typeof window === "undefined" || window.location.pathname === "/login" || redirectingToLogin) return;
  redirectingToLogin = true;
  const next = window.location.pathname + window.location.search + window.location.hash;
  window.location.replace("/login?next=" + encodeURIComponent(next));
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
  // Redirect as soon as the server reports an expired or invalid session.
  // Keep login failures on the login page so the user can correct credentials.
  redirectOnUnauthorized(response.status);
  if (response.status === 204) return null;
  let body;
  try { body = await response.json(); }
  catch { throw new APIError("The server returned an unexpected response. Please try again.", response.status); }

  if (!response.ok) {
    const code = body.error?.code || body.code || "";
    const serverMsg = body.error?.message || body.message || "";
    let categoryMsg = "The request could not be completed.";

    if (response.status === 501 || code === "MIDDLEWARE_NOT_CONFIGURED" || code === "NOT_IMPLEMENTED") {
      categoryMsg = "Feature Unavailable: This feature is not enabled or implemented yet.";
    } else if (response.status === 403 && code === "CSRF_INVALID") {
      categoryMsg = "Security Verification Failed: Your session state is invalid. Please refresh the page.";
    } else if (response.status === 409 || code === "RESOURCE_CONFLICT" || code === "MIGRATION_HISTORY_EXISTS") {
      categoryMsg = "Conflict Error: The operation cannot be completed due to current resource state.";
    } else if (response.status >= 500 && code !== "SESSION_BUSY") {
      categoryMsg = "Server Failure: An internal server error occurred.";
    } else if (response.status === 400 || response.status === 422) {
      categoryMsg = "Validation Error: Invalid input parameters.";
    }

    // Combine the category prefix with the specific server message if present
    const finalMessage = serverMsg ? `${categoryMsg} (${serverMsg})` : categoryMsg;

    throw new APIError(finalMessage, response.status, code);
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

export function qualifiedName(schema, table) {
  return [schema, table].map(name => '"' + String(name).replaceAll('"', '""') + '"').join(".");
}

export function accessCapabilities(level) {
  if (level === "VIEW_EDIT") return {
    label: "Read + migrations",
    detail: "Browse schema and data, run SELECT queries, review history, and create or execute migrations.",
  };
  if (level === "VIEW_ONLY") return {
    label: "Read only",
    detail:"Browse schema and data, run SELECT queries, and review migration history. Changes require migration access.",
  };
  return { label: "Access unavailable", detail: "Effective database permissions could not be verified." };
}

export function columnKeys(column, constraints = []) {
  const list = constraints || [];
  const keys = new Set(list.filter(item => item.columns?.includes(column)).map(item => item.type.toUpperCase()));
  return [["PRIMARY KEY", "PK"], ["FOREIGN KEY", "FK"], ["UNIQUE", "Unique"]]
    .filter(([type]) => keys.has(type)).map(([, label]) => label).join(", ") || "--";
}
