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
let selectedGroupID = "";
let groupData = { groups: [], users: [], databases: [], members: [], grants: [] };
let groupTab = "members";
let connectionVersion = 0;

function formSignature(form) {
  return JSON.stringify([...form.querySelectorAll("input, select, textarea")].map(field => [field.id || field.name, field.type === "checkbox" || field.type === "radio" ? field.checked : field.value]));
}
function showFormDialog(dialog) {
  clearNotice();
  const form = dialog.querySelector("form");
  form.dataset.saved = formSignature(form);
  dialog.showModal();
}
function formChanged(form) { return form?.dataset.saved !== undefined && form.dataset.saved !== formSignature(form); }


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
    if (form.getAttribute("aria-busy") === "true") return;
    const data = new FormData(form);
    const fields = [...form.querySelectorAll("input, select, textarea")].map(field => [field, field.disabled]);
    fields.forEach(([field]) => { field.disabled = true; });
    const otherButtons = [...form.querySelectorAll('button:not([type="submit"])')].map(button => [button, button.disabled]);
    otherButtons.forEach(([button]) => { button.disabled = true; });
    form.setAttribute("aria-busy", "true");
    try { await busy(form.querySelector('[type="submit"]'), () => work(data, form)); }
    catch (error) { fail(error, form); }
    finally {
      fields.forEach(([field, disabled]) => { field.disabled = disabled; });
      otherButtons.forEach(([button, disabled]) => { button.disabled = disabled; });
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
  showFormDialog(dialog);
}

function openGroup(group) {
  const { dialog, form } = resetDialog("group-dialog", group ? "Rename group" : "New group");
  if (group) { form.elements.id.value = group.id; form.elements.name.value = group.name; }
  showFormDialog(dialog);
}

function setPasswordEditing(role, editing) {
  const form = $("connection-form");
  const stored = Boolean(form.elements.id.value);
  $(role + "-secret-state").hidden = !stored || editing;
  $(role + "-password").hidden = stored && !editing;
  $(role + "-password").required = !stored || editing;
  $(role + "-password").disabled = stored && !editing;
  $("keep-" + role + "-password").hidden = !stored || !editing;
  if (!editing) $(role + "-password").value = "";
}
function updateCredentialFields() {
  const same = $("same-credentials").checked;
  $("migrator-fields").hidden = same;
  $("shared-credential-note").hidden = !same;
  $("migrator-username").disabled = same;
  $("migrator-password").disabled = same || $("migrator-password").hidden;
}
async function openConnection(id) {
  const version = ++connectionVersion;
  const connection = id ? await api("/api/v1/admin/databases/" + encode(id)) : null;
  if (version !== connectionVersion) return;
  const { dialog, form } = resetDialog("connection-dialog", id ? "Edit connection" : "Add connection");
  $("connection-enabled-field").hidden = !id;
  $("connection-impact").hidden = !id;
  if (connection) {
    for (const [key, value] of Object.entries(connection)) {
      const field = form.elements.namedItem(key);
      if (!field) continue;
      if (field.type === "checkbox") field.checked = value;
      else field.value = value;
    }
  }
  // Matching usernames do not imply matching stored passwords.
  $("same-credentials").checked = false;
  for (const role of ["reader", "migrator"]) setPasswordEditing(role, !id);
  updateCredentialFields();
  $("connection-test-result").hidden = true;
  updateConnectionEngine(false);
  showFormDialog(dialog);
  dialog.querySelector(".dialog-body").scrollTop = 0;
}

function bindDialogs() {
  document.querySelectorAll("[data-open]").forEach(button => button.addEventListener("click", () => {
    if (button.dataset.open === "user-dialog") openUser();
    else if (button.dataset.open === "group-dialog") openGroup();
    else openConnection().catch(error => fail(error));
  }));
  async function dismiss(dialog) {
    const form = dialog.querySelector("form");
    if (form?.getAttribute("aria-busy") === "true" || dialog.dataset.dismissing) return;
    dialog.dataset.dismissing = "true";
    try {
      if (formChanged(form) && !await confirmAction("Discard unsaved changes?", "Your changes have not been saved. Discard them and close this dialog?", "Discard changes")) return;
      dialog.close();
    } finally { delete dialog.dataset.dismissing; }
  }
  document.querySelectorAll("[data-close]").forEach(button => button.addEventListener("click", () => dismiss(button.closest("dialog"))));
  document.querySelectorAll("dialog:not(#confirm-dialog)").forEach(dialog => {
    dialog.addEventListener("cancel", event => { event.preventDefault(); dismiss(dialog); });
    dialog.addEventListener("close", () => {
      dialog.querySelectorAll('input[type="password"]').forEach(field => { field.value = ""; });
      delete dialog.querySelector("form").dataset.saved;
    });
  });
}

async function loadDatabaseContext() {
  const database = await api(databaseAPI);
  activeDatabase = database;
  document.querySelectorAll(".engine-label").forEach(element => { element.textContent = database.engine === "mysql" ? "MySQL" : "PostgreSQL"; });
  if (state.page === "query") {
    const params = new URLSearchParams(location.search);
    if (params.has("schema") && params.has("table") && !$("sql").dataset.initialized) {
      $("sql").value = "SELECT * FROM " + qualifiedName(params.get("schema"), params.get("table"), database.engine) + " LIMIT 50;";
      $("sql").dataset.initialized = "true";
    }
    $("sql").placeholder = database.engine === "mysql" ? "SELECT * FROM `" + database.database_name.replaceAll("`", "``") + "`.`your_table` LIMIT 50;" : "SELECT * FROM public.your_table LIMIT 50;";
  }
  if ($("migration-save-note") && !dirty) $("migration-save-note").textContent = migrationPolicy();
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
      ? "No databases registered yet. Click 'Add database' to connect your first database."
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
  if (params.has("schema") && params.has("table")) $("sql").value = "SELECT * FROM " + qualifiedName(params.get("schema"), params.get("table"), activeDatabase?.engine) + " LIMIT 50;";
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

function migrationPolicy() {
  return activeDatabase?.engine === "mysql"
    ? "MySQL: one schema statement per migration (DDL auto-commits), or transactional INSERT/UPDATE/DELETE on InnoDB tables."
    : "All statements execute together in a transaction.";
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
    $("migration-save-note").textContent = dirty ? "Unsaved changes. Save the draft before executing." : migrationPolicy();
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
    if (!await confirmAction("Execute migration?", "Run the saved SQL against " + activeDatabase.database_name + " (" + activeDatabase.label + ")? " + migrationPolicy(), "Execute migration")) return;
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
      $("migration-save-note").textContent = draft ? migrationPolicy() : "Executed SQL is read only.";
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
      const explanation = errBox.querySelector(".migration-error-subtitle");
      explanation.textContent = activeDatabase?.engine === "mysql"
        ? "MySQL schema changes auto-commit. Check the target schema and error details before retrying. DML migrations use a transaction."
        : "Review the error details to confirm the execution and rollback outcome before retrying.";
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

function selectedGroup() { return groupData.groups.find(group => group.id === selectedGroupID); }
function groupName(id) { return groupData.groups.find(group => group.id === id)?.name || id; }
function userAccess(user, database, excludedGroup = "") {
  if (!user.is_active) return { level: "DISABLED", sources: ["Inactive account"] };
  if (!database.is_enabled) return { level: "DISABLED", sources: ["Disabled connection"] };
  if (user.is_platform_admin) return { level: "VIEW_EDIT", sources: ["Platform administrator"] };
  const memberships = groupData.members.filter(member => member.user_id === user.id && member.group_id !== excludedGroup).map(member => member.group_id);
  const grants = groupData.grants.filter(grant => memberships.includes(grant.group_id) && grant.database_id === database.id);
  return { level: grants.some(grant => grant.access_level === "VIEW_EDIT") ? "VIEW_EDIT" : grants.length ? "VIEW_ONLY" : "NONE", sources: grants.map(grant => groupName(grant.group_id)) };
}
function accessLabel(level) { return {VIEW_ONLY:"Read only", VIEW_EDIT:"Read + migrations", NONE:"No access", DISABLED:"Unavailable"}[level] || level; }
function switchGroupTab(tab, focus = false) {
  groupTab = tab;
  document.querySelectorAll("[data-group-tab]").forEach(button => {
    const active = button.dataset.groupTab === tab;
    button.setAttribute("aria-selected", String(active));
    button.tabIndex = active ? 0 : -1;
    $(button.getAttribute("aria-controls")).hidden = !active;
    if (active && focus) button.focus();
  });
}
function openMembership() {
  const group = selectedGroup();
  if (!group) return;
  const {dialog, form} = resetDialog("membership-dialog");
  form.elements.group_id.value = group.id;
  const memberIDs = groupData.members.filter(member => member.group_id === group.id).map(member => member.user_id);
  options("member-user", groupData.users.filter(user => user.is_active && !memberIDs.includes(user.id)), user => user.username, "Select a user…");
  $("membership-context").textContent = "Add a user to " + group.name + ".";
  form.querySelector('[type="submit"]').disabled = !groupData.users.some(user => user.is_active && !memberIDs.includes(user.id));
  showFormDialog(dialog);
}
function describePermission() {
  const edit = $("grant-form").elements.access_level.value === "VIEW_EDIT";
  $("permission-description").textContent = edit ? "Inspect schemas, browse data, run read-only queries, and create, edit, or execute migrations. Migrations can change schemas and data." : "Inspect schemas, browse table rows, and run permitted read-only queries. Cannot author or execute migrations.";
}
function openGrant(grant) {
  const group = selectedGroup();
  if (!group) return;
  const { dialog, form } = resetDialog("grant-dialog", grant ? "Edit database access" : "Add database access");
  form.elements.group_id.value = group.id;
  const available = groupData.databases.filter(database => grant ? database.id === grant.database_id : database.is_enabled && !groupData.grants.some(item => item.group_id === group.id && item.database_id === database.id));
  options("grant-database", available, database => database.label, "Select a database…");
  $("grant-database").value = grant ? grant.database_id : "";
  form.elements.access_level.value = grant?.access_level || "VIEW_ONLY";
  form.querySelector('[type="submit"]').disabled = !available.length;
  $("grant-context").textContent = "Permissions for " + group.name + ".";
  describePermission();
  showFormDialog(dialog);
}
function renderGroup() {
  const group = selectedGroup();
  $("group-detail").hidden = !group;
  if (!group) return;
  document.querySelectorAll("[data-group-id]").forEach(button => button.setAttribute("aria-pressed", String(button.dataset.groupId === group.id)));
  const memberships = groupData.members.filter(member => member.group_id === group.id);
  const members = groupData.users.filter(user => memberships.some(member => member.user_id === user.id));
  const grants = groupData.grants.filter(grant => grant.group_id === group.id);
  $("selected-group-name").textContent = group.name;
  $("group-summary").textContent = `${members.length} member${members.length === 1 ? "" : "s"} · ${grants.length} database${grants.length === 1 ? "" : "s"}`;
  $("group-actions").replaceChildren(action("Rename", () => openGroup(group)), action("Delete group", async () => {
    if (!await confirmAction("Delete " + group.name + "?", "This removes the group, its memberships, and database grants. Access provided by other groups or administrator roles remains. Affected sessions will refresh.", "Delete group")) return;
    await api("/api/v1/admin/groups/" + encode(group.id), {method:"DELETE"});
    await reload(); notice(group.name + " deleted.", "success");
  }, "danger"));
  rows("group-members", members, user => [node("strong", user.username), user.is_active ? user.is_platform_admin ? "Administrator" : "User" : "Inactive account", action("Remove", async () => {
    const retained = groupData.databases.filter(db => db.is_enabled).map(db => db.label + ": " + accessLabel(userAccess(user, db, group.id).level)).join("; ");
    if (!await confirmAction("Remove " + user.username + " from " + group.name + "?", "Access from other groups or administrator roles remains. After removal: " + (retained || "No enabled databases.") + ". The user's sessions will refresh.", "Remove member")) return;
    await api("/api/v1/admin/groups/" + encode(group.id) + "/members/" + encode(user.id), {method:"DELETE"});
    await reload(); notice(user.username + " removed from " + group.name + ".", "success");
  }, "danger")], 3, "No members yet. Add a user to this group.");
  rows("group-grants", grants, grant => {
    const database = groupData.databases.find(db => db.id === grant.database_id);
    const controls = node("div", undefined, "toolbar-actions");
    controls.append(action("Edit", () => openGrant(grant)), action("Revoke", async () => {
      const retained = members.map(user => {
        const access = userAccess(user, database || {id:grant.database_id,is_enabled:false}, group.id);
        return user.username + ": " + accessLabel(access.level) + (access.sources.length ? " via " + access.sources.join(", ") : "");
      }).join("; ");
      if (!await confirmAction("Revoke " + group.name + " access to " + (database?.label || grant.database_id) + "?", "Members may retain access through other groups or administrator roles. After revoking: " + (retained || "No current members.") + ". Affected sessions will refresh.", "Revoke access")) return;
      await api("/api/v1/admin/groups/" + encode(group.id) + "/databases/" + encode(grant.database_id) + "/access", {method:"DELETE"});
      await reload(); notice("Access to " + (database?.label || grant.database_id) + " revoked for " + group.name + ".", "success");
    }, "danger"));
    return [database ? database.label + (database.is_enabled ? "" : " (disabled)") : grant.database_id, badge(grant.access_level), controls];
  }, 3, "No database access yet.");
  const effective = members.flatMap(user => groupData.databases.filter(db => db.is_enabled).map(database => ({user, database, ...userAccess(user, database)})));
  rows("effective-access", effective, item => {
    const sources = node("div");
    for (const source of item.sources) sources.append(node("span", source, "access-source"));
    if (!item.sources.length) sources.textContent = "No group grant";
    return [item.user.username, item.database.label, accessLabel(item.level), sources];
  }, 4, "Add members and connections to inspect effective access.");
  switchGroupTab(groupTab);
}
async function groupsPage() {
  try {
    const [groups, users, databases, snapshot] = await Promise.all([api("/api/v1/admin/groups"), api("/api/v1/admin/users"), api("/api/v1/admin/databases"), api("/api/v1/admin/access")]);
    groupData = {groups:groups || [], users:users || [], databases:databases || [], members:snapshot.members || [], grants:snapshot.grants || []};
    if (!groupData.groups.some(group => group.id === selectedGroupID)) selectedGroupID = groupData.groups[0]?.id || "";
    $("groups-empty").hidden = Boolean(groupData.groups.length);
    $("groups-layout").hidden = !groupData.groups.length;
    $("group-list").replaceChildren(...groupData.groups.map(group => {
      const button = node("button", group.name, "group-choice");
      button.type = "button"; button.dataset.groupId = group.id;
      button.onclick = () => { selectedGroupID = group.id; renderGroup(); };
      return button;
    }));
    renderGroup();
  } catch(error) {
    $("group-detail").hidden = true;
    $("groups-empty").hidden = true;
    $("group-list").replaceChildren(node("p", "Unable to load groups and access. Try again.", "inline-state"));
    throw error;
  }
}
function bindGroupForms() {
  $("add-member").onclick = openMembership;
  $("add-access").onclick = () => openGrant();
  const tabs = [...document.querySelectorAll("[data-group-tab]")];
  tabs.forEach((button, index) => {
    button.onclick = () => switchGroupTab(button.dataset.groupTab);
    button.onkeydown = event => {
      let next;
      if (event.key === "ArrowRight") next = (index + 1) % tabs.length;
      if (event.key === "ArrowLeft") next = (index + tabs.length - 1) % tabs.length;
      if (event.key === "Home") next = 0;
      if (event.key === "End") next = tabs.length - 1;
      if (next !== undefined) { event.preventDefault(); switchGroupTab(tabs[next].dataset.groupTab, true); }
    };
  });
  $("grant-form").addEventListener("change", describePermission);
  submit("group-form", async data => {
    const id = data.get("id");
    const result = await api("/api/v1/admin/groups" + (id ? "/" + encode(id) : ""), {method:id ? "PATCH" : "POST",data:{name:data.get("name").trim()}});
    selectedGroupID = result.id;
    $("group-dialog").close(); await reload(); notice(data.get("name") + " saved.", "success");
  });
  submit("membership-form", async data => {
    const groupID = data.get("group_id"), userID = data.get("user_id");
    if (!groupID || !userID) throw new Error("Select a user to add.");
    const user = groupData.users.find(user => user.id === userID), name = groupName(groupID);
    await api("/api/v1/admin/groups/" + encode(groupID) + "/members/" + encode(userID), {method:"PUT"});
    $("membership-dialog").close(); await reload(); notice(user.username + " added to " + name + ".", "success");
  });
  submit("grant-form", async data => {
    const groupID = data.get("group_id"), databaseID = data.get("database_id");
    if (!groupID || !databaseID) throw new Error("Select a database.");
    const name = groupName(groupID), database = groupData.databases.find(db => db.id === databaseID);
    await api("/api/v1/admin/groups/" + encode(groupID) + "/databases/" + encode(databaseID) + "/access", {method:"PUT",data:{access_level:data.get("access_level")}});
    $("grant-dialog").close(); await reload(); notice("Access to " + database.label + " updated for " + name + ".", "success");
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
      }, "primary"));
    }
    const statusBadge = node("span", connection.is_enabled ? "Enabled" : "Disabled", "badge " + (connection.is_enabled ? "green" : "red"));
    return [node("strong", connection.label), node("span", connection.database_name, "mono"), statusBadge, controls];
  }, 4, "No connections yet. Add a database connection to get started.");
}

function updateConnectionEngine(changePort = true) {
  const mysql = $("connection-engine").value === "mysql";
  const port = $("connection-port");
  if (changePort && ["5432", "3306"].includes(port.value)) port.value = mysql ? "3306" : "5432";
  const verifyCA = $("connection-ssl").querySelector('[value="verify-ca"]');
  verifyCA.hidden = mysql;
  verifyCA.disabled = mysql;
  if (mysql && $("connection-ssl").value === "verify-ca") $("connection-ssl").value = "verify-full";
  updateSSLDescription();
}

function updateSSLDescription() {
  $("ssl-description").textContent = {
    "disable":"Traffic is not encrypted. Use only on a trusted local network, such as an internal Docker network.",
    "require":"Traffic is encrypted; the server's identity is not verified.",
    "verify-ca":"Traffic is encrypted and the server certificate authority is verified.",
    "verify-full":"Traffic is encrypted; the certificate authority and server hostname are verified. The server must trust the database certificate."
  }[$("connection-ssl").value];
}
function connectionPayload(data) {
  const payload = {};
  for (const key of ["engine", "label", "host", "database_name", "ssl_mode", "reader_username", "reader_password", "migrator_username", "migrator_password"]) {
    if (!data.has(key)) continue;
    payload[key] = key.endsWith("_password") ? data.get(key) : data.get(key).trim();
  }
  payload.port = Number(data.get("port"));
  payload.use_reader_credentials = $("same-credentials").checked;
  if (data.get("id")) payload.is_enabled = data.has("is_enabled");
  return payload;
}
function bindConnectionForm() {
  const form = $("connection-form");
  if (form._boundConnection) return;
  form._boundConnection = true;
  $("same-credentials").onchange = updateCredentialFields;
  $("connection-ssl").onchange = updateSSLDescription;
  $("connection-engine").onchange = () => updateConnectionEngine();
  for (const role of ["reader", "migrator"]) {
    $("change-" + role + "-password").onclick = () => { setPasswordEditing(role, true); $(role + "-password").focus(); };
    $("keep-" + role + "-password").onclick = () => { setPasswordEditing(role, false); $("change-" + role + "-password").focus(); };
  }
  form.addEventListener("input", () => { $("connection-test-result").hidden = true; });
  $("test-connection").onclick = () => {
    if (!form.reportValidity() || form.getAttribute("aria-busy") === "true") return;
    const data = new FormData(form), payload = connectionPayload(data);
    if (data.get("id")) payload.database_id = data.get("id");
    const fields = [...form.querySelectorAll("input, select, button")].map(field => [field, field.disabled]);
    fields.forEach(([field]) => { if (field !== $("test-connection")) field.disabled = true; });
    form.setAttribute("aria-busy", "true");
    form.querySelector(".form-message").hidden = true;
    $("connection-test-result").hidden = true;
    busy($("test-connection"), async () => {
      const result = await api("/api/v1/admin/databases/test-connection", {method:"POST",data:payload});
      $("connection-test-result").textContent = result.message || (result.connected ? "Connection succeeded." : "Connection failed. Check the host and credentials, then try again.");
      $("connection-test-result").className = "test-result " + (result.connected ? "success" : "error");
      $("connection-test-result").hidden = false;
    }).catch(error => fail(error, form)).finally(() => {
      fields.forEach(([field, disabled]) => { field.disabled = disabled; });
      form.removeAttribute("aria-busy");
    });
  };
  submit("connection-form", async (data, form) => {
    const id = data.get("id"), payload = connectionPayload(data);
    const label = payload.label;
    await api("/api/v1/admin/databases" + (id ? "/" + encode(id) : ""), {method:id ? "PATCH" : "POST",data:payload});
    $("connection-dialog").close(); await reload(); notice("Connection saved. " + label + " is ready.", "success");
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
  if (dirty || [...document.querySelectorAll("dialog[open] form")].some(formChanged)) { event.preventDefault(); event.returnValue = ""; }
});
if (state.page !== "login") {
  bindNavigation();
  bindDialogs();
  $("dismiss-notice").onclick = clearNotice;
  $("retry").onclick = () => busy($("retry"), start).catch(error => fail(error));
}
start().catch(error => fail(error));
