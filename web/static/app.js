import { api, displayValue, returnPath, qualifiedName, accessCapabilities, columnKeys } from "./api.mjs";

const $ = id => document.getElementById(id);
const state = document.body.dataset;
const encode = value => encodeURIComponent(value);
const databasePath = "/databases/" + encode(state.databaseId || "");
const databaseAPI = "/api/v1" + databasePath;
let reload = async () => {};
let canEdit = false;
let dirty = false;
let savedSQL = "";
let activeDatabase = null;
let currentUser = null;
let selectedSchema = state.schema || "";
let explorerEntries = new Map();
let explorerVersion = 0;
let schemaRequest = 0;

function node(tag, text, className) {
  const element = document.createElement(tag);
  if (text !== undefined) element.textContent = text;
  if (className) element.className = className;
  return element;
}

function link(text, href, className) {
  const element = node("a", text, className);
  element.href = href;
  return element;
}

function notice(message, type = "error", retry = false) {
  if (!$("notice")) return;
  $("notice").hidden = false;
  $("notice").className = "notice " + type;
  $("notice-text").textContent = message;
  $("retry").hidden = !retry;
}

function clearNotice() { if ($("notice")) $("notice").hidden = true; }

function fail(error, form) {
  if (error.status === 401 && state.page !== "login") {
    location.assign("/login?next=" + encode(location.pathname + location.search));
    return;
  }
  const message = error.message || "Something went wrong. Please try again.";
  const local = form?.querySelector(".form-message");
  if (local) { local.textContent = message; local.hidden = false; }
  else notice(message, "error", !form);
  if ($("access-badge")?.textContent === "Checking access…") $("access-badge").textContent = "Access unavailable";
  document.querySelectorAll(".empty-cell, .inline-state").forEach(element => {
    if (element.textContent.startsWith("Loading")) element.textContent = "Unable to load records. Please try again.";
  });
}

async function busy(button, work) {
  if (button?.disabled) return;
  const label = button?.textContent;
  if (button) { button.disabled = true; button.textContent = "Working…"; }
  try { return await work(); }
  finally { if (button) { button.disabled = false; button.textContent = label; } }
}

function action(label, work, className = "") {
  const button = node("button", label, "button small " + className);
  button.type = "button";
  button.addEventListener("click", () => busy(button, work).catch(error => fail(error)));
  return button;
}

function submit(id, work) {
  const form = $(id);
  if (!form || form._boundSubmit) return;
  form._boundSubmit = true;
  form.addEventListener("submit", async event => {
    event.preventDefault();
    const message = form.querySelector(".form-message");
    if (message) message.hidden = true;
    clearNotice();
    const data = new FormData(form);
    const fields = [...form.querySelectorAll("input, select, textarea")].map(field => [field, field.disabled]);
    fields.forEach(([field]) => { field.disabled = true; });
    form.setAttribute("aria-busy", "true");
    try { await busy(form.querySelector('[type="submit"]'), () => work(data, form)); }
    catch (error) { fail(error, form); }
    finally {
      fields.forEach(([field, disabled]) => { field.disabled = disabled; });
      form.removeAttribute("aria-busy");
      if (form.id === "login-form" && !$("login-error").hidden) $("password").focus();
    }
  });
}

function badge(value) {
  const labels = { VIEW_ONLY: "Read only", VIEW_EDIT: "Read + migrations", DRAFT: "Draft", RUNNING: "Running", APPLIED: "Applied", FAILED: "Failed", ACTIVE: "Active", DISABLED: "Disabled" };
  const colors = { APPLIED: "green", ACTIVE: "green", FAILED: "red", DISABLED: "red", RUNNING: "amber", VIEW_EDIT: "blue", VIEW_ONLY: "purple" };
  return node("span", labels[value] || value, "badge " + (colors[value] || ""));
}


function date(value) {
  if (!value) return "--";
  const result = new Date(value);
  return Number.isNaN(result.getTime()) ? "--" : result.toLocaleString([], { dateStyle: "medium", timeStyle: "short" });
}

function tableRow(values) {
  const tr = node("tr");
  for (const value of values) {
    const td = node("td");
    if (value instanceof Node) td.append(value);
    else td.textContent = value ?? "--";
    tr.append(td);
  }
  return tr;
}

function rows(id, data, render, columns, empty = "No records found.") {
  const container = $(id);
  container.replaceChildren();
  if (!data?.length) {
    const tr = tableRow([empty]);
    tr.firstChild.colSpan = columns;
    tr.firstChild.className = "empty-cell";
    container.append(tr);
  } else for (const item of data) container.append(tableRow(render(item)));
}

function dataTable(id, columns, records) {
  const target = $(id);
  target.replaceChildren();
  if (!columns?.length) { target.append(node("p", "No columns returned.", "inline-state")); return; }
  const table = node("table", undefined, "data-table");
  const head = node("thead");
  const headings = node("tr");
  for (const column of columns) { const th = node("th", column); th.scope = "col"; headings.append(th); }
  head.append(headings);
  table.append(head);
  const body = node("tbody");
  if (!records?.length) {
    const row = tableRow(["No rows returned."]);
    row.firstChild.colSpan = columns.length;
    row.firstChild.className = "empty-cell";
    body.append(row);
  }
  for (const record of records || []) {
    body.append(tableRow(columns.map((_, index) => {
      const value = record[index];
      const cell = node("span", displayValue(value), value === null ? "null-value" : "");
      return cell;
    })));
  }
  table.append(body);
  target.append(table);
}

function options(id, data, label, placeholderText) {
  const select = $(id);
  if (!select) return;
  const previous = select.value;
  select.replaceChildren();
  // Always add a disabled placeholder first option
  const ph = node("option", placeholderText || "Select…");
  ph.value = "";
  ph.disabled = true;
  ph.selected = true;
  select.append(ph);
  if (!data?.length) {
    const emptyOption = node("option", "No items available");
    emptyOption.value = "";
    emptyOption.disabled = true;
    select.append(emptyOption);
    select.dataset.empty = "";
    delete select.dataset.placeholder;
    return;
  }
  delete select.dataset.empty;
  select.dataset.placeholder = "";
  for (const item of data) {
    const option = node("option", label(item));
    option.value = item.id;
    select.append(option);
  }
  if (previous && [...select.options].some(option => option.value === previous)) {
    select.value = previous;
    delete select.dataset.placeholder;
  }
}

function confirmAction(title, description, label) {
  return new Promise(resolve => {
    const dialog = $("confirm-dialog");
    $("confirm-title").textContent = title;
    $("confirm-description").textContent = description;
    $("confirm-button").textContent = label;
    dialog.returnValue = "cancel";
    dialog.addEventListener("close", () => resolve(dialog.returnValue === "confirm"), { once: true });
    dialog.showModal();
    dialog.querySelector('[value="cancel"]').focus();
  });
}

function resetDialog(id, title) {
  const dialog = $(id);
  const form = dialog.querySelector("form");
  form.reset();
  if (form.elements.id) form.elements.id.value = "";
  const message = form.querySelector(".form-message");
  if (message) message.hidden = true;
  if (title) $(id + "-title").textContent = title;
  return { dialog, form };
}

function openUser(user) {
  const { dialog, form } = resetDialog("user-dialog", user ? "Edit user" : "New user");
  $("user-admin-field").hidden = Boolean(user);
  $("user-active-field").hidden = !user;
  form.elements.password.required = !user;
  $("user-password-help").textContent = user ? "Leave blank to keep the current password." : "Use a password of up to 72 bytes.";
  if (user) {
    form.elements.id.value = user.id;
    form.elements.username.value = user.username;
    form.elements.is_active.checked = user.is_active;
  }
  dialog.showModal();
}

function openGroup(group) {
  const { dialog, form } = resetDialog("group-dialog", group ? "Rename group" : "New group");
  if (group) { form.elements.id.value = group.id; form.elements.name.value = group.name; }
  dialog.showModal();
}

async function openConnection(id) {
  const { dialog, form } = resetDialog("connection-dialog", id ? "Edit connection" : "Add connection");
  $("connection-enabled-field").hidden = !id;
  for (const name of ["reader_password", "migrator_password"]) form.elements[name].required = !id;
  $("connection-password-help").textContent = id ? "Leave passwords blank to keep the stored credentials." : "Passwords are never displayed after saving.";
  const sameCreds = $("same-credentials");
  const migratorFields = $("migrator-fields");
  if (!id) {
    if (!form.elements.host.value) form.elements.host.value = "postgres";
    if (!form.elements.port.value) form.elements.port.value = "5432";
    if (!form.elements.database_name.value) form.elements.database_name.value = "target_db";
    if (!form.elements.ssl_mode.value) form.elements.ssl_mode.value = "disable";
    if (!form.elements.reader_username.value) form.elements.reader_username.value = "admin";
    if (!form.elements.reader_password.value) form.elements.reader_password.value = "root";
    if (sameCreds) {
      sameCreds.checked = true;
      if (migratorFields) migratorFields.hidden = true;
    }
  } else {
    const connection = await api("/api/v1/admin/databases/" + encode(id));
    for (const [key, value] of Object.entries(connection)) {
      const field = form.elements.namedItem(key);
      if (!field) continue;
      if (field.type === "checkbox") field.checked = value;
      else field.value = value;
    }
    if (sameCreds && migratorFields) {
      const isSame = connection.reader_username === connection.migrator_username;
      sameCreds.checked = isSame;
      migratorFields.hidden = isSame;
    }
  }
  dialog.showModal();
}

function bindDialogs() {
  document.querySelectorAll("[data-open]").forEach(button => button.addEventListener("click", () => {
    if (button.dataset.open === "user-dialog") openUser();
    else if (button.dataset.open === "group-dialog") openGroup();
    else openConnection().catch(error => fail(error));
  }));
  document.querySelectorAll("[data-close]").forEach(button => button.addEventListener("click", () => {
    const dialog = button.closest("dialog");
    dialog.querySelectorAll('input[type="password"]').forEach(field => { field.value = ""; });
    dialog.close();
  }));
  document.querySelectorAll("dialog").forEach(dialog => dialog.addEventListener("close", () => {
    dialog.querySelectorAll('input[type="password"]').forEach(field => { field.value = ""; });
  }));
}

async function loadDatabaseContext() {
  const database = await api(databaseAPI);
  activeDatabase = database;
  document.querySelectorAll(".database-label").forEach(element => { element.textContent = database.label; });
  $("database-name").textContent = database.database_name;
  document.querySelectorAll(".target-database").forEach(element => { element.textContent = database.database_name; });
  const access = accessCapabilities(database.effective_access);
  $("access-badge").textContent = access.label;
  $("access-capabilities").textContent = access.detail;
  canEdit = database.effective_access === "VIEW_EDIT";
  document.querySelectorAll("[data-edit-only]").forEach(element => { element.hidden = !canEdit; });
  if ($("migration-permission")) $("migration-permission").textContent = canEdit ? "Only drafts can be edited or executed. SELECT queries remain read only." : "Read only. Creating or executing migrations requires migration access.";
  const statusEl = document.querySelector(".connection-status");
  if (statusEl) {
    statusEl.textContent = "Connected";
    statusEl.title = "Target database reachable";
    statusEl.style.color = "var(--green)";
  }
  return database;
}

function permissionCell(level) {
  const access = accessCapabilities(level);
  const cell = node("div");
  cell.append(node("span", access.label, "permission-name"));
  cell.append(node("span", level === "VIEW_EDIT" ? "SELECT · create & execute migrations" : level === "VIEW_ONLY" ? "SELECT · schema · history" : "Permissions not verified", "permission-detail"));
  cell.title = access.detail;
  return cell;
}

async function databasesPage() {
  const databases = await api("/api/v1/databases");
  const filter = () => {
    const query = $("database-filter").value.toLowerCase();
    const visible = (databases || []).filter(item => (item.label + " " + item.database_name).toLowerCase().includes(query));
    $("database-count").textContent = visible.length + (query ? " of " + (databases?.length || 0) : "") + (visible.length === 1 ? " connection" : " connections");
    const emptyMessage = query
      ? "No databases match your filter."
      : currentUser?.is_platform_admin
      ? "No databases registered yet. Click 'Add database' to connect your first PostgreSQL database."
      : "No databases are assigned to your groups. Ask an administrator for access.";
    rows("database-list", visible, item => [
      link(item.label, "/databases/" + encode(item.id)),
      node("span", item.database_name, "mono"),
      permissionCell(item.effective_access),
      link("Open", "/databases/" + encode(item.id), "button small open-database"),
    ], 4, emptyMessage);
  };
  $("database-filter").oninput = filter;
  filter();
}

async function explorerTables(entry) {
  if (!entry.promise) {
    entry.links.replaceChildren(node("p", "Loading tables…", "inline-state"));
    entry.promise = api(databaseAPI + "/schemas/" + encode(entry.name) + "/tables").then(tables => {
      entry.error = false;
      entry.tables = tables || [];
      entry.links.replaceChildren();
      for (const table of entry.tables) {
        const item = link(table.name, databasePath + "/schemas/" + encode(entry.name) + "/tables/" + encode(table.name));
        item.setAttribute("aria-label", entry.name + "." + table.name);
        if (state.schema === entry.name && state.table === table.name) item.setAttribute("aria-current", "page");
        entry.links.append(item);
      }
      if (!entry.tables.length) entry.links.append(node("p", "No tables", "inline-state"));
      return entry.tables;
    }).catch(error => {
      entry.error = true;
      entry.promise = null;
      entry.links.replaceChildren(node("p", error.message, "inline-state"), action("Retry", () => explorerTables(entry)));
      if (error.status === 401) fail(error);
      throw error;
    });
  }
  return entry.promise;
}

async function loadExplorer() {
  const version = ++explorerVersion;
  const previousOpen = new Set([...explorerEntries.values()].filter(entry => entry.details.open).map(entry => entry.name));
  const schemas = await api(databaseAPI + "/schemas");
  if (version !== explorerVersion) return;
  const list = $("schema-list");
  list.replaceChildren();
  explorerEntries = new Map();
  if (!schemas?.length) { list.append(node("p", "No schemas available.", "inline-state")); return; }
  if (!schemas.some(schema => schema.name === selectedSchema)) selectedSchema = schemas[0].name;
  for (const schema of schemas) {
    const details = node("details");
    const summary = node("summary", schema.name);
    summary.setAttribute("aria-current", String(schema.name === selectedSchema));
    const links = node("div", undefined, "object-links");
    const entry = { name: schema.name, details, summary, links, tables: null, promise: null };
    explorerEntries.set(schema.name, entry);
    details.append(summary, links);
    details.addEventListener("toggle", () => {
      if (details.open) explorerTables(entry).catch(() => {});
    });
    summary.addEventListener("click", () => {
      selectedSchema = schema.name;
      for (const item of explorerEntries.values()) item.summary.setAttribute("aria-current", String(item === entry));
      if (state.page === "database") schemaPage().catch(error => fail(error));
    });
    list.append(details);
    details.open = schema.name === selectedSchema || previousOpen.has(schema.name);
  }
  const selected = explorerEntries.get(selectedSchema);
  if (selected) await explorerTables(selected).catch(() => {});
  let filterVersion = 0;
  let searching = false;
  const filter = async () => {
    const request = ++filterVersion;
    const query = $("object-filter").value.trim().toLowerCase();
    if (query && !searching) for (const entry of explorerEntries.values()) entry.beforeSearch = entry.details.open;
    searching = Boolean(query);
    if (query) await Promise.allSettled([...explorerEntries.values()].map(explorerTables));
    if (request !== filterVersion || version !== explorerVersion) return;
    let matches = 0;
    for (const entry of explorerEntries.values()) {
      const schemaMatches = entry.name.toLowerCase().includes(query);
      const tableMatches = (entry.tables || []).some(table => (entry.name + "." + table.name).toLowerCase().includes(query));
      entry.details.hidden = Boolean(query) && !schemaMatches && !tableMatches && !entry.error;
      if (!entry.details.hidden) matches++;
      entry.links.querySelectorAll("a").forEach(item => {
        item.hidden = Boolean(query) && !schemaMatches && !(entry.name + "." + item.textContent).toLowerCase().includes(query);
      });
      entry.details.open = query ? !entry.details.hidden : (entry.beforeSearch ?? entry.details.open);
    }
    $("object-empty")?.remove();
    if (!matches) { const empty = node("p", "No matching objects.", "inline-state"); empty.id = "object-empty"; list.append(empty); }
  };
  $("object-filter").oninput = () => filter().catch(error => fail(error));
  if ($("object-filter").value) await filter();
}

async function schemaPage() {
  const request = ++schemaRequest;
  const entry = explorerEntries.get(selectedSchema);
  if (!entry) {
    rows("table-list", [], () => [], 3, "No schema selected.");
    return;
  }
  $("schema-name").textContent = selectedSchema;
  $("table-count").textContent = "Loading tables…";
  try {
    const tables = await explorerTables(entry);
    if (request !== schemaRequest || entry !== explorerEntries.get(selectedSchema)) return;
    $("table-count").textContent = tables.length + (tables.length === 1 ? " table" : " tables");
    rows("table-list", tables, table => [
      link(table.name, databasePath + "/schemas/" + encode(entry.name) + "/tables/" + encode(table.name)),
      node("span", entry.name, "mono"), "Table",
    ], 3, "This schema has no visible tables.");
  } catch (error) {
    if (request !== schemaRequest) return;
    $("table-count").textContent = "Unable to load tables.";
    throw error;
  }
}

function bindNavigation() {
  const toggle = $("navigation-toggle");
  const media = window.matchMedia("(max-width: 760px)");
  const setOpen = (open, focus = true) => {
    document.body.classList.toggle("navigation-open", open);
    toggle.setAttribute("aria-expanded", String(open));
    toggle.textContent = open ? "Close" : "Menu";
    $("main").inert = open;
    $("account").inert = open;
    if (focus) (open ? $("sidebar").querySelector("a") : toggle).focus();
  };
  toggle.onclick = () => setOpen(toggle.getAttribute("aria-expanded") !== "true");
  document.addEventListener("keydown", event => {
    if (event.key === "Escape" && toggle.getAttribute("aria-expanded") === "true") setOpen(false);
  });
  $("sidebar").addEventListener("click", event => {
    if (media.matches && event.target.closest("a")) setOpen(false, false);
  });
  media.addEventListener("change", event => { if (!event.matches) setOpen(false, false); });
}

function bindTabs() {
  const tabs = [...document.querySelectorAll("[data-tab]")];
  const activate = tab => {
    for (const item of tabs) {
      const selected = item === tab;
      item.setAttribute("aria-selected", String(selected));
      item.tabIndex = selected ? 0 : -1;
      $(item.dataset.tab).hidden = !selected;
    }
  };
  for (const tab of tabs) {
    tab.addEventListener("click", () => activate(tab));
    tab.addEventListener("keydown", event => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
      event.preventDefault();
      let index = tabs.indexOf(tab);
      index = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : (index + (event.key === "ArrowRight" ? 1 : -1) + tabs.length) % tabs.length;
      activate(tabs[index]); tabs[index].focus();
    });
  }
}

function tablePage() {
  let offset = 0;
  let requestID = 0;
  const tableAPI = databaseAPI + "/schemas/" + encode(state.schema) + "/tables/" + encode(state.table);
  const loadRows = async () => {
    const request = ++requestID;
    const limit = Number($("page-size").value);
    const data = await api(tableAPI + "/rows?limit=" + limit + "&offset=" + offset);
    if (request !== requestID) return;
    dataTable("rows-table", data.columns, data.rows);
    $("rows-range").textContent = data.rows?.length ? "Rows " + (offset + 1) + "–" + (offset + data.rows.length) : "No rows";
    $("previous-page").disabled = offset === 0;
    $("next-page").disabled = !data.has_more;
  };
  $("previous-page").onclick = () => { offset = Math.max(0, offset - Number($("page-size").value)); loadRows().catch(error => fail(error)); };
  $("next-page").onclick = () => { offset += Number($("page-size").value); loadRows().catch(error => fail(error)); };
  $("page-size").onchange = () => { offset = 0; loadRows().catch(error => fail(error)); };
  bindTabs();
  $("copy-object").onclick = () => busy($("copy-object"), async () => {
    const name = qualifiedName(state.schema, state.table);
    try { await navigator.clipboard.writeText(name); notice("Copied " + name, "success"); }
    catch { notice("Could not copy automatically. Qualified name: " + name, "error"); }
  }).catch(error => fail(error));
  reload = async () => {
    const detail = await api(tableAPI);
    rows("column-list", detail.columns, column => [node("span", column.name, "mono"), column.data_type, column.nullable ? "Yes" : "No", node("span", columnKeys(column.name, detail.constraints), "key-marker")], 4, "No column metadata.");
    rows("constraint-list", detail.constraints, constraint => [constraint.name, constraint.type, (constraint.columns || []).join(", ")], 3, "No constraints.");
    rows("index-list", detail.indexes, index => [index.name, (index.columns || []).join(", "), index.is_unique ? "Yes" : "No"], 3, "No indexes.");
    await loadRows();
  };
}

function queryPage() {
  const params = new URLSearchParams(location.search);
  if (params.has("schema") && params.has("table")) $("sql").value = "SELECT * FROM " + qualifiedName(params.get("schema"), params.get("table")) + " LIMIT 50;";
  submit("query-form", async data => {
    const started = performance.now();
    $("query-summary").textContent = "Running…";
    $("query-results").setAttribute("aria-busy", "true");
    try {
      const result = await api(databaseAPI + "/queries", { method: "POST", data: { sql: data.get("sql"), max_rows: Number(data.get("max_rows")) } });
      dataTable("query-results", result.columns, result.rows);
      $("query-summary").textContent = result.row_count + (result.row_count === 1 ? " row" : " rows") + " · " + ((performance.now() - started) / 1000).toFixed(2) + "s" + (result.truncated ? " · Row limit reached" : "");
      $("query-results").focus();
    } catch (error) {
      $("query-summary").textContent = "Query failed · " + ((performance.now() - started) / 1000).toFixed(2) + "s";
      throw error;
    } finally { $("query-results").removeAttribute("aria-busy"); }
  });
  $("sql").addEventListener("keydown", event => {
    if ((event.ctrlKey || event.metaKey) && event.key === "Enter") { event.preventDefault(); $("query-form").requestSubmit(); }
  });
}

async function migrationsPage() {
  const filter = $("migration-filter").value;
  const migrations = await api(databaseAPI + "/migrations" + (filter ? "?status=" + encode(filter) : ""));
  rows("migration-list", migrations, migration => [
    link(migration.name, databasePath + "/migrations/" + encode(migration.id)),
    migration.version === null ? "--" : migration.version, badge(migration.status),
    migration.executed_by || "--", date(migration.executed_at),
  ], 5, "No migrations found. Users with edit access can create a draft.");
}

function migrationPage() {
  const creating = state.page === "migration-new";
  const endpoint = databaseAPI + "/migrations" + (creating ? "" : "/" + encode(state.migrationId));
  let draft = creating;
  let executing = false;
  const markSaved = () => { savedSQL = $("sql").value; dirty = false; };

  if (creating) {
    const template = sessionStorage.getItem("clone_migration");
    if (template) {
      sessionStorage.removeItem("clone_migration");
      try {
        const data = JSON.parse(template);
        if ($("migration-name") && data.name) $("migration-name").value = data.name;
        if ($("sql") && data.sql) $("sql").value = data.sql;
        markSaved();
      } catch {}
    }
  }

  $("sql").addEventListener("input", () => {
    dirty = $("sql").value !== savedSQL;
    if (!creating) $("execute-migration").disabled = dirty || !draft || !canEdit || executing;
    $("migration-save-note").textContent = dirty ? "Unsaved changes. Save the draft before executing." : "Changes execute together in a transaction.";
  });
  $("migration-name")?.addEventListener("input", () => { dirty = true; });
  submit("migration-form", async data => {
    if (executing) throw new Error("Wait for migration execution to finish before editing.");
    if (!canEdit || !draft) throw new Error("You need edit access to save a draft migration.");
    const payload = { sql_text: data.get("sql_text") };
    if (creating) payload.name = data.get("name");
    const result = await api(endpoint, { method: creating ? "POST" : "PATCH", data: payload });
    markSaved();
    if (creating) location.assign(databasePath + "/migrations/" + encode(result.id));
    else {
      $("execute-migration").disabled = false;
      $("migration-save-note").textContent = "Draft saved. Review the SQL before executing.";
      notice("Migration draft saved.", "success");
    }
  });
  $("execute-migration").addEventListener("click", () => busy($("execute-migration"), async () => {
    if (creating || !draft || !canEdit || dirty || executing) return;
    if (!await confirmAction("Execute migration?", "Run the saved SQL against " + activeDatabase.database_name + " (" + activeDatabase.label + ")? All statements must succeed for the transaction to commit.", "Execute migration")) return;
    executing = true;
    $("sql").readOnly = true;
    $("save-migration").disabled = true;
    $("migration-save-note").textContent = "Executing on " + activeDatabase.database_name + "…";
    try {
      await api(endpoint + "/execute", { method: "POST" });
      clearNotice();
      await reload();
      notice("Migration execution completed. Review its status below.", "success");
    } catch (error) {
      clearNotice();
      try {
        await reload();
        // reload() already renders the formatted failure card on the migration detail page
        return;
      } catch (reloadErr) {
        fail(reloadErr);
      }
    } finally {
      executing = false;
      $("sql").readOnly = !canEdit || !draft;
      $("save-migration").disabled = false;
      $("migration-save-note").textContent = draft ? "Changes execute together in a transaction." : "Executed SQL is read only.";
    }
  }).catch(error => { if (error) fail(error); }).finally(() => { $("execute-migration").disabled = !draft || dirty || !canEdit || executing; }));
  reload = async () => {
    if (creating) { $("sql").readOnly = !canEdit; return; }
    const migration = await api(endpoint);
    draft = migration.status === "DRAFT";
    $("migration-status").replaceChildren(badge(migration.status));
    $("migration-version").textContent = migration.version ?? "—";
    $("migration-executor").textContent = migration.executed_by || "—";
    $("migration-time").textContent = date(migration.executed_at);
    document.querySelector("h1").textContent = migration.name;
    if (!dirty) { $("sql").value = migration.sql_text; markSaved(); }
    $("sql").readOnly = !canEdit || !draft || executing;
    $("save-migration").hidden = !canEdit || !draft;
    $("execute-migration").hidden = !canEdit || !draft;
    $("execute-migration").disabled = !draft || !canEdit || dirty || executing;
    $("migration-help").textContent = draft ? "Saving a draft does not execute SQL." : "Executed migrations are read only. Create a new draft for further changes.";
    
    const errBox = $("migration-error");
    if (errBox) {
      errBox.hidden = !migration.error_summary;
      const errText = $("migration-error-text");
      if (errText) errText.textContent = migration.error_summary || "";
    }

    const cloneAction = () => {
      sessionStorage.setItem("clone_migration", JSON.stringify({
        name: migration.name + (migration.status === "FAILED" ? " (retry)" : " (copy)"),
        sql: migration.sql_text,
      }));
      location.assign(databasePath + "/migrations/new");
    };

    for (const cloneId of ["clone-migration-btn"]) {
      const btn = $(cloneId);
      if (btn) {
        btn.hidden = !canEdit || draft;
        btn.onclick = cloneAction;
      }
    }

    if (migration.status === "RUNNING") notice("This migration is running or awaiting confirmation. Refresh its status before taking further action.", "", true);
  };
}

async function usersPage() {
  const users = await api("/api/v1/admin/users");
  rows("user-list", users, user => {
    const controls = node("div", undefined, "toolbar-actions");
    controls.append(action("Edit", () => openUser(user)));
    return [node("strong", user.username), user.is_platform_admin ? "Administrator" : "User", badge(user.is_active ? "ACTIVE" : "DISABLED"), controls];
  }, 4, "No users found.");
}

function bindUserForm() {
  submit("user-form", async (data, form) => {
    const id = data.get("id");
    const password = data.get("password");
    if (new TextEncoder().encode(password).length > 72) throw new Error("Password must be no more than 72 bytes.");
    if (id && !data.has("is_active") && !await confirmAction("Disable this account?", "The user will be signed out and will no longer be able to sign in.", "Disable account")) return;
    const payload = { username: data.get("username") };
    if (password || !id) payload.password = password;
    if (id) payload.is_active = data.has("is_active");
    else payload.is_platform_admin = data.has("is_platform_admin");
    await api("/api/v1/admin/users" + (id ? "/" + encode(id) : ""), { method: id ? "PATCH" : "POST", data: payload });
    form.elements.password.value = "";
    $("user-dialog").close();
    await reload();
    notice(id ? "User updated." : "User created.", "success");
  });
}

async function groupsPage() {
  const groups = await api("/api/v1/admin/groups");
  rows("group-list", groups, group => {
    const controls = node("div", undefined, "toolbar-actions");
    controls.append(action("Rename", () => openGroup(group)), action("Delete", async () => {
      if (!await confirmAction("Delete group?", "Delete " + group.name + " and its memberships and database grants? Users may lose access.", "Delete group")) return;
      await api("/api/v1/admin/groups/" + encode(group.id), { method: "DELETE" });
      await reload();
      notice("Group deleted.", "success");
    }, "danger"));
    return [node("strong", group.name), node("span", group.id, "mono"), controls];
  }, 3, "No groups yet. Create a group to assign database access.");
  options("member-group", groups, item => item.name, "Select a group…");
  options("grant-group", groups, item => item.name, "Select a group…");
  const results = await Promise.allSettled([api("/api/v1/admin/users"), api("/api/v1/admin/databases")]);
  if (results[0].status === "fulfilled") options("member-user", results[0].value, item => item.username, "Select a user…");
  else fail(results[0].reason);
  if (results[1].status === "fulfilled") options("grant-database", results[1].value, item => item.label, "Select a database…");
  else fail(results[1].reason);
}

function bindGroupForms() {
  // Dynamic submit button label for membership form
  const membershipForm = $("membership-form");
  const membershipSubmit = $("membership-submit");
  if (membershipForm && membershipSubmit) {
    const updateMembershipLabel = () => {
      const action = membershipForm.elements["action"]?.value;
      membershipSubmit.textContent = action === "remove" ? "Remove from group" : "Add to group";
      membershipSubmit.className = action === "remove"
        ? "button danger full-width"
        : "button success full-width";
    };
    membershipForm.querySelectorAll('input[name="action"]').forEach(radio => {
      radio.addEventListener("change", updateMembershipLabel);
    });
    updateMembershipLabel();
  }

  // Dynamic submit button label for grant form
  const grantForm = $("grant-form");
  const grantSubmit = $("grant-submit");
  if (grantForm && grantSubmit) {
    const updateGrantLabel = () => {
      const level = grantForm.elements["access_level"]?.value;
      if (level === "REVOKE") {
        grantSubmit.textContent = "Revoke database access";
        grantSubmit.className = "button danger full-width";
      } else if (level === "VIEW_EDIT") {
        grantSubmit.textContent = "Grant read + migration access";
        grantSubmit.className = "button primary full-width";
      } else {
        grantSubmit.textContent = "Grant read-only access";
        grantSubmit.className = "button primary full-width";
      }
    };
    grantForm.querySelectorAll('input[name="access_level"]').forEach(radio => {
      radio.addEventListener("change", updateGrantLabel);
    });
    updateGrantLabel();
  }

  submit("group-form", async data => {
    const id = data.get("id");
    await api("/api/v1/admin/groups" + (id ? "/" + encode(id) : ""), { method: id ? "PATCH" : "POST", data: { name: data.get("name") } });
    $("group-dialog").close(); await reload(); notice("Group saved.", "success");
  });
  submit("membership-form", async data => {
    const groupId = data.get("group_id");
    const userId = data.get("user_id");
    if (!groupId) throw new Error("Please select a group.");
    if (!userId) throw new Error("Please select a user.");
    const remove = data.get("action") === "remove";
    if (remove && !await confirmAction("Remove group member?", "The user may lose database access provided by this group.", "Remove member")) return;
    await api("/api/v1/admin/groups/" + encode(groupId) + "/members/" + encode(userId), { method: remove ? "DELETE" : "PUT" });
    notice(remove ? "Member removed from group." : "Member added to group.", "success");
  });
  submit("grant-form", async data => {
    const groupId = data.get("group_id");
    const databaseId = data.get("database_id");
    if (!groupId) throw new Error("Please select a group.");
    if (!databaseId) throw new Error("Please select a database.");
    const revoke = data.get("access_level") === "REVOKE";
    if (revoke && !await confirmAction("Revoke database access?", "Members may lose access to this database unless another group provides it.", "Revoke access")) return;
    await api("/api/v1/admin/groups/" + encode(groupId) + "/databases/" + encode(databaseId) + "/access", { method: revoke ? "DELETE" : "PUT", ...(revoke ? {} : { data: { access_level: data.get("access_level") } }) });
    notice(revoke ? "Database access revoked." : "Database access updated.", "success");
  });
}

async function connectionsPage() {
  const connections = await api("/api/v1/admin/databases");
  rows("connection-list", connections, connection => {
    const controls = node("div", undefined, "toolbar-actions");
    controls.append(
      action("Edit", () => openConnection(connection.id)),
      action("Test", async () => {
        const result = await api("/api/v1/admin/databases/" + encode(connection.id) + "/test-connection", { method: "POST" });
        notice(
          result.connected
            ? "Reader and migrator connections succeeded."
            : ("Connection test failed: " + (result.message || "Could not reach target database.")),
          result.connected ? "success" : "error"
        );
      })
    );
    if (connection.is_enabled) {
      controls.append(action("Disable", async () => {
        if (!await confirmAction("Disable connection?", "Users will no longer be able to access " + connection.label + ". Migration history is preserved.", "Disable connection")) return;
        await api("/api/v1/admin/databases/" + encode(connection.id), { method: "DELETE" });
        await reload(); notice("Connection disabled.", "success");
      }, "danger"));
    } else {
      controls.append(action("Re-enable", async () => {
        if (!await confirmAction("Re-enable connection?", "Users with group access will be able to connect to " + connection.label + " again.", "Re-enable")) return;
        await api("/api/v1/admin/databases/" + encode(connection.id), { method: "PATCH", data: { is_enabled: true } });
        await reload(); notice("Connection re-enabled.", "success");
      }, "success"));
    }
    const statusBadge = node("span", connection.is_enabled ? "Enabled" : "Disabled", "badge " + (connection.is_enabled ? "green" : "red"));
    return [node("strong", connection.label), node("span", connection.database_name, "mono"), statusBadge, controls];
  }, 4, "No connections yet. Add a PostgreSQL connection to get started.");
}

function bindConnectionForm() {
  const sameCreds = $("same-credentials");
  const migratorFields = $("migrator-fields");
  if (sameCreds && migratorFields && !sameCreds._boundChange) {
    sameCreds._boundChange = true;
    const updateVisibility = () => {
      migratorFields.hidden = sameCreds.checked;
      if (sameCreds.checked) {
        $("migrator-username").value = $("reader-username").value;
        $("migrator-password").value = $("reader-password").value;
      }
    };
    sameCreds.onchange = updateVisibility;
    $("reader-username").addEventListener("input", () => {
      if (sameCreds.checked) $("migrator-username").value = $("reader-username").value;
    });
    $("reader-password").addEventListener("input", () => {
      if (sameCreds.checked) $("migrator-password").value = $("reader-password").value;
    });
  }

  submit("connection-form", async (data, form) => {
    const id = data.get("id");
    const same = $("same-credentials")?.checked;
    const payload = {};
    for (const key of ["label", "host", "database_name", "ssl_mode", "reader_username", "reader_password", "migrator_username", "migrator_password"]) {
      if (id && key.endsWith("_password") && !data.get(key)) continue;
      payload[key] = data.get(key);
    }
    if (same) {
      payload.migrator_username = payload.reader_username;
      payload.migrator_password = payload.reader_password;
    }
    payload.port = Number(data.get("port")) || 5432;
    if (id) payload.is_enabled = data.has("is_enabled");
    await api("/api/v1/admin/databases" + (id ? "/" + encode(id) : ""), { method: id ? "PATCH" : "POST", data: payload });
    form.querySelectorAll('input[type="password"]').forEach(input => { input.value = ""; });
    $("connection-dialog").close(); await reload(); notice("Connection saved.", "success");
  });
}

async function start() {
  if (state.page === "login") {
    $("password-toggle").onclick = () => {
      const visible = $("password").type === "password";
      $("password").type = visible ? "text" : "password";
      $("password-toggle").textContent = visible ? "Hide" : "Show";
      $("password-toggle").setAttribute("aria-label", visible ? "Hide password" : "Show password");
      $("password-toggle").setAttribute("aria-pressed", String(visible));
    };
    $("password").addEventListener("keyup", event => { $("caps-lock").hidden = !event.getModifierState("CapsLock"); });
    submit("login-form", async data => {
      const error = $("login-error"); error.hidden = true;
      try {
        await api("/api/v1/auth/login", { method: "POST", data: { username: data.get("username"), password: data.get("password") } });
        $("password").value = "";
        location.assign(returnPath(new URLSearchParams(location.search).get("next"), location.origin));
      } catch (failure) { error.textContent = failure.message; error.hidden = false; $("password").focus(); }
    });
    return;
  }
  $("logout").onclick = () => busy($("logout"), async () => {
    if (dirty && !await confirmAction("Leave without saving?", "Your unsaved migration changes will be lost.", "Sign out")) return;
    await api("/api/v1/auth/logout", { method: "POST" });
    dirty = false; location.assign("/login");
  }).catch(error => fail(error));
  const user = await api("/api/v1/auth/me");
  currentUser = user;
  $("account-name").textContent = user.username;
  document.querySelector(".avatar").textContent = user.username.slice(0, 1).toUpperCase();
  $("admin-nav").hidden = !user.is_platform_admin;
  if (user.is_platform_admin) {
    document.querySelectorAll("[data-admin-only]").forEach(element => { element.hidden = false; });
    bindConnectionForm();
  }
  if (["users", "groups", "connections"].includes(state.page) && !user.is_platform_admin) {
    $("page-content").replaceChildren(node("p", "Administrator access is required to manage this section.", "notice error"));
    $("page-content").inert = false;
    document.querySelector(".heading-actions").hidden = true;
    return;
  }
  const setup = {
    databases: () => { reload = databasesPage; },
    database: () => { reload = schemaPage; },
    table: tablePage,
    query: queryPage,
    migrations: () => { reload = migrationsPage; $("migration-filter").onchange = () => reload().catch(error => fail(error)); },
    "migration-new": migrationPage,
    migration: migrationPage,
    users: () => { reload = usersPage; bindUserForm(); },
    groups: () => { reload = groupsPage; bindGroupForms(); },
    connections: () => { reload = connectionsPage; bindConnectionForm(); },
  };
  setup[state.page]?.();
  $("page-content").inert = false;
  document.querySelector(".heading-actions").inert = false;
  const refresh = async () => {
    clearNotice();
    if (state.databaseId) await loadDatabaseContext();
    if (state.databaseId) {
      try { await loadExplorer(); }
      catch (error) { $("schema-list").replaceChildren(node("p", error.message, "inline-state")); fail(error); }
    }
    await reload();
  };
  for (const id of ["refresh", "retry"]) if ($(id)) $(id).onclick = event => busy(event.currentTarget, refresh).catch(error => fail(error));
  await refresh();
}

window.addEventListener("beforeunload", event => {
  if (dirty) { event.preventDefault(); event.returnValue = ""; }
});
if (state.page !== "login") {
  bindNavigation();
  bindDialogs();
  $("retry").onclick = () => busy($("retry"), start).catch(error => fail(error));
}
start().catch(error => fail(error));
