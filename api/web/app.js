const SAMPLE_XML = `<?xml version="1.0" encoding="UTF-8"?>
<catalog generated="2026-01-01" version="2" active="true">
  <title>Type Converter Catalog</title>
  <count>3</count>
  <rating>4.75</rating>
  <notes><![CDATA[Special chars: <tag> & "quotes"]]></notes>
  <flags>
    <flag>featured</flag>
    <flag>sale</flag>
  </flags>
  <book id="1" available="true" sku="GO-001">
    <title>Go in Action</title>
    <price>32.5</price>
    <pages>300</pages>
    <tags>
      <tag>lang</tag>
      <tag>backend</tag>
    </tags>
  </book>
  <book id="2">
    <title>XML Cookbook</title>
    <meta>
      <author>Ann</author>
      <year>2020</year>
    </meta>
  </book>
  <book id="3" available="false">
    <title>Надёжный JSON</title>
    <edition year="2024" pages="128"/>
  </book>
</catalog>
`;

const SAMPLE_JSON = `{
  "generated": "2026-01-01",
  "version": 2,
  "active": true,
  "title": "Type Converter Catalog",
  "count": 3,
  "rating": 4.75,
  "notes": "Special chars: <tag> & \\"quotes\\"",
  "flags": ["featured", "sale"],
  "books": [
    {"id": 1, "available": true, "sku": "GO-001", "title": "Go in Action", "price": 32.5, "pages": 300, "tags": ["lang", "backend"]},
    {"id": 2, "title": "XML Cookbook", "meta": {"author": "Ann", "year": 2020}},
    {"id": 3, "available": false, "title": "Надёжный JSON", "editions": [{"year": 2024, "pages": 128}]}
  ]
}
`;

const SAMPLE_XSD = `<?xml version="1.0" encoding="UTF-8"?>
<xs:schema elementFormDefault="qualified" xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="catalog" type="Catalog"/>
  <xs:complexType name="Catalog">
    <xs:sequence>
      <xs:element name="title" type="xs:string"/>
      <xs:element name="book" type="Book" minOccurs="0" maxOccurs="unbounded"/>
    </xs:sequence>
    <xs:attribute name="generated" type="xs:string"/>
    <xs:attribute name="version" type="xs:int"/>
  </xs:complexType>
  <xs:complexType name="Book">
    <xs:sequence>
      <xs:element name="title" type="xs:string"/>
      <xs:element name="price" type="xs:decimal" minOccurs="0"/>
    </xs:sequence>
    <xs:attribute name="id" type="xs:int" use="required"/>
    <xs:attribute name="available" type="xs:boolean"/>
  </xs:complexType>
</xs:schema>
`;

const SAMPLE_TEMPLATE = `{
  "root": {
    "name": "catalog",
    "type": "Catalog"
  },
  "types": {
    "Catalog": {
      "kind": "complex",
      "attributes": [
        {"name": "generated", "type": "string", "min": 0},
        {"name": "version", "type": "int", "min": 0}
      ],
      "elements": [
        {"name": "title", "type": "string"},
        {"name": "book", "type": "Book", "min": 0, "max": "unbounded"}
      ]
    },
    "Book": {
      "kind": "complex",
      "attributes": [
        {"name": "id", "type": "int", "required": true},
        {"name": "available", "type": "boolean", "min": 0}
      ],
      "elements": [
        {"name": "title", "type": "string"},
        {"name": "price", "type": "decimal", "min": 0}
      ]
    }
  }
}
`;

const STATUS_LABEL = {
  queued: "в очереди",
  running: "выполняется",
  done: "готово",
  failed: "ошибка",
};

const state = {
  win: "convert",
  format: "xml",
  out: "json",
  busy: false,
  currentID: "",
  schemaID: "",
  results: { xml: "", json: "" },
  schema: false,
  schemaRoot: "",
  schemaSrc: "xsd",
  schemaBusy: false,
  schemaResults: { xml: "", json: "" },
};

const el = {
  source: document.getElementById("source"),
  output: document.getElementById("output"),
  convert: document.getElementById("convert"),
  millError: document.getElementById("mill-error"),
  jobMeta: document.getElementById("job-meta"),
  stages: document.getElementById("stages"),
  jobs: document.getElementById("jobs"),
  copyOut: document.getElementById("copy-out"),
  downloadOut: document.getElementById("download-out"),
  schemaCopy: document.getElementById("schema-copy"),
  schemaDownload: document.getElementById("schema-download"),
  store: document.getElementById("store"),
  storeXML: document.getElementById("store-xml"),
  storeJSON: document.getElementById("store-json"),
  drop: document.getElementById("drop"),
  outHint: document.getElementById("out-hint"),
  schemaStatus: document.getElementById("schema-status"),
  schemaSource: document.getElementById("schema-source"),
  schemaOutput: document.getElementById("schema-output"),
  schemaParse: document.getElementById("schema-parse"),
  schemaError: document.getElementById("schema-error"),
  schemaMeta: document.getElementById("schema-meta"),
  schemaStages: document.getElementById("schema-stages"),
  schemaJobs: document.getElementById("schema-jobs"),
  schemaHint: document.getElementById("schema-hint"),
  schemaResultLabel: document.getElementById("schema-result-label"),
  schemaDrop: document.getElementById("schema-drop"),
  sourceBanner: document.getElementById("source-banner"),
  outBanner: document.getElementById("out-banner"),
  schemaSrcBanner: document.getElementById("schema-src-banner"),
  schemaOutBanner: document.getElementById("schema-out-banner"),
  file: document.getElementById("file"),
  schemaFile: document.getElementById("schema-file"),
  convertResultLabel: document.getElementById("convert-result-label"),
  winConvert: document.getElementById("win-convert"),
  winSchema: document.getElementById("win-schema"),
};

const PREVIEW_LIMIT = 64 * 1024;
const PREVIEW_LINES = 220;

const editors = {
  source: { body: "", locked: false },
  schema: { body: "", locked: false },
};

function contentType(format) {
  return format === "xml" || format === "xsd" ? "application/xml" : "application/json";
}

function isBulky(text) {
  return !!text && text.length > PREVIEW_LIMIT;
}

function formatSize(n) {
  if (n < 1024) return n + " Б";
  if (n < 1024 * 1024) return (n / 1024).toFixed(1).replace(".0", "") + " КБ";
  return (n / (1024 * 1024)).toFixed(1).replace(".0", "") + " МБ";
}

function previewText(text) {
  if (!text || text.length <= PREVIEW_LIMIT) return text;
  return text.slice(0, PREVIEW_LIMIT) + "…";
}

function clipPreview(text) {
  if (!text) return text;
  const lines = text.split("\n");
  if (lines.length > PREVIEW_LINES) {
    return lines.slice(0, PREVIEW_LINES).join("\n") + "\n…";
  }
  if (text.length > PREVIEW_LIMIT) {
    return text.slice(0, PREVIEW_LIMIT) + "…";
  }
  return text;
}

function prettyJSON(text) {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

function prettyXML(text) {
  const trimmed = text.trim();
  const parts = trimmed.replace(/>(\s*)</g, ">\n<").split("\n");
  let pad = 0;
  const out = [];
  for (const raw of parts) {
    const line = raw.trim();
    if (!line) continue;
    if (/^<\//.test(line)) pad = Math.max(0, pad - 1);
    out.push("  ".repeat(pad) + line);
    if (/^<[^!?/][^>]*[^/]>$/.test(line) && !/^<[^>]+>.*<\/[^>]+>$/.test(line)) {
      pad += 1;
    }
  }
  return out.join("\n") + "\n";
}

function formatBody(format, text) {
  return format === "json" ? prettyJSON(text) : prettyXML(text);
}

function displayBody(format, text) {
  if (!text) return "";
  return clipPreview(formatBody(format, text));
}

function exportBody(format, text) {
  if (!text) return "";
  return formatBody(format, text);
}

function setBanner(node, fullText, extra) {
  if (!node) return;
  const parent = node.parentElement;
  const show = !!fullText && (isBulky(fullText) || !!extra);
  if (!show) {
    node.hidden = true;
    node.textContent = "";
    if (parent) parent.classList.remove("has-banner");
    return;
  }
  node.hidden = false;
  if (parent) parent.classList.add("has-banner");
  node.textContent =
    formatSize(fullText.length) +
    " в памяти" +
    (extra ? " · " + extra : " · в окне превью " + formatSize(PREVIEW_LIMIT));
}

function editorEls(name) {
  if (name === "schema") {
    return { textarea: el.schemaSource, banner: el.schemaSrcBanner, drop: el.schemaDrop };
  }
  return { textarea: el.source, banner: el.sourceBanner, drop: el.drop };
}

function editorText(name) {
  const ed = editors[name];
  return ed.locked ? ed.body : editorEls(name).textarea.value;
}

function setEditor(name, text) {
  const ed = editors[name];
  const nodes = editorEls(name);
  ed.body = text || "";
  ed.locked = isBulky(ed.body);
  nodes.textarea.readOnly = ed.locked;
  nodes.textarea.value = ed.locked ? previewText(ed.body) : ed.body;
  nodes.drop.classList.toggle("is-bulk", ed.locked);
  setBanner(nodes.banner, ed.body, ed.locked ? "редактирование отключено, на сервер уйдёт целиком" : "");
}

function onEditorInput(name) {
  const ed = editors[name];
  if (ed.locked) return;
  const value = editorEls(name).textarea.value;
  ed.body = value;
  if (isBulky(value)) setEditor(name, value);
}

function onEditorPaste(name, ev) {
  const text = ev.clipboardData && ev.clipboardData.getData("text");
  if (!text || !isBulky(text)) return;
  ev.preventDefault();
  setEditor(name, text);
}

function templateRootName(text) {
  const head = text.length > 8192 ? text.slice(0, 8192) : text;
  const m =
    head.match(/"root"\s*:\s*\{[\s\S]{0,400}?"name"\s*:\s*"([^"]+)"/) ||
    head.match(/"name"\s*:\s*"([^"]+)"/);
  return m ? m[1] : "";
}

function parseError(text, isXML) {
  if (!text) return "неизвестная ошибка";
  if (isXML) {
    const m = text.match(/<error>([\s\S]*?)<\/error>/);
    return m ? decodeXML(m[1]) : text;
  }
  try {
    const obj = JSON.parse(text);
    return obj.error || text;
  } catch {
    return text;
  }
}

function decodeXML(s) {
  return s
    .replaceAll("&lt;", "<")
    .replaceAll("&gt;", ">")
    .replaceAll("&quot;", '"')
    .replaceAll("&apos;", "'")
    .replaceAll("&amp;", "&");
}

function setError(node, msg) {
  node.hidden = !msg;
  node.textContent = msg || "";
}

function setStages(node, status) {
  if (!node) return;
  node.dataset.status = status || "";
  for (const li of node.querySelectorAll("li")) {
    li.classList.remove("is-on", "is-done", "is-fail");
    const stage = li.dataset.stage;
    if (status === "failed" && stage === "done") {
      li.classList.add("is-fail");
      li.textContent = "ошибка";
    } else if (stage === "done") {
      li.textContent = "готово";
    }
    if (status === stage) li.classList.add("is-on");
    if (
      (status === "running" && stage === "queued") ||
      (status === "done" && (stage === "queued" || stage === "running")) ||
      (status === "failed" && (stage === "queued" || stage === "running"))
    ) {
      li.classList.add("is-done");
    }
  }
}

function convertTarget() {
  return state.format === "xml" ? "json" : "xml";
}

function resetConvertOutput() {
  state.results = { xml: "", json: "" };
  state.currentID = "";
  setError(el.millError, "");
  setStages(el.stages, "");
  el.jobMeta.textContent = "Нет активной задачи";
}

function syncConvertDirection() {
  const toJSON = state.format === "xml";
  state.out = convertTarget();
  el.convertResultLabel.textContent = toJSON ? "JSON" : "XML";
  el.convertResultLabel.classList.toggle("is-json", toJSON);
  el.convertResultLabel.classList.toggle("is-xml", !toJSON);
  el.convert.textContent = toJSON ? "В JSON" : "В XML";
  el.outHint.textContent = toJSON ? "На выходе только JSON" : "На выходе только XML";
  setFileAccept(el.file, state.format);
  showOutput();
}

function showOutput() {
  const format = convertTarget();
  const text = state.results[format];
  renderOut(el.output, text, format, el.outBanner);
  setExportEnabled(el.copyOut, el.downloadOut, !!text);
}

function schemaTarget() {
  return state.schemaSrc === "xsd" ? "json" : "xml";
}

function resetSchemaOutput() {
  state.schemaResults = { xml: "", json: "" };
  state.schemaID = "";
  setError(el.schemaError, "");
  setStages(el.schemaStages, "");
  el.schemaMeta.textContent = "Нет активной задачи";
}

function syncSchemaDirection() {
  const toJSON = state.schemaSrc === "xsd";
  el.schemaResultLabel.textContent = toJSON ? "JSON template" : "XSD";
  el.schemaResultLabel.classList.toggle("is-json", toJSON);
  el.schemaResultLabel.classList.toggle("is-xml", !toJSON);
  el.schemaHint.textContent = toJSON ? "На выходе только JSON template" : "На выходе только XSD";
  el.schemaParse.textContent = toJSON ? "В JSON template" : "В XSD";
  setFileAccept(el.schemaFile, toJSON ? "xsd" : "jsontemplate");
  showSchemaOutput();
}

function showSchemaOutput() {
  const format = schemaTarget();
  const text = state.schemaResults[format];
  renderOut(el.schemaOutput, text, format, el.schemaOutBanner);
  setExportEnabled(el.schemaCopy, el.schemaDownload, !!text);
}

function setExportEnabled(copyBtn, downloadBtn, enabled) {
  copyBtn.disabled = !enabled;
  downloadBtn.disabled = !enabled;
}

function renderOut(node, text, format, banner) {
  node.classList.remove("is-xml", "is-json", "is-empty", "is-fail");
  if (!text) {
    node.classList.add("is-empty");
    node.textContent = "Результат появится здесь.";
    setBanner(banner, "");
    return;
  }
  node.classList.add(format === "xml" ? "is-xml" : "is-json");
  const pretty = formatBody(format, text);
  const preview = clipPreview(pretty);
  const clipped = preview !== pretty;
  setBanner(
    banner,
    clipped ? text : "",
    clipped ? "показаны первые " + PREVIEW_LINES + " строк, полный текст — копировать или скачать" : ""
  );
  node.textContent = preview;
}

async function readResponse(res) {
  const text = await res.text();
  const xml = (res.headers.get("content-type") || "").includes("xml");
  return { ok: res.ok, status: res.status, text, xml };
}

function setWindow(name) {
  state.win = name;
  el.winConvert.hidden = name !== "convert";
  el.winSchema.hidden = name !== "schema";
  for (const btn of document.querySelectorAll("[data-win]")) {
    const on = btn.dataset.win === name;
    btn.classList.toggle("is-on", on);
    btn.setAttribute("aria-selected", String(on));
  }
}

async function submit() {
  const body = editorText("source");
  if (!body.trim()) {
    setError(el.millError, "Пустое тело запроса");
    return;
  }
  state.busy = true;
  el.convert.disabled = true;
  setError(el.millError, "");
  setStages(el.stages, "queued");
  el.jobMeta.textContent = "Отправка…";
  state.results = { xml: "", json: "" };
  showOutput();
  try {
    const res = await fetch("/" + state.format + "?catalog=1", {
      method: "POST",
      headers: { "Content-Type": contentType(state.format) },
      body,
    });
    const got = await readResponse(res);
    if (!res.ok) {
      setStages(el.stages, "");
      setError(el.millError, parseError(got.text, got.xml));
      el.jobMeta.textContent = "HTTP " + res.status;
      return;
    }
    const ticket = parseTicket(got);
    if (!ticket.id) throw new Error("сервер не вернул id задачи");
    state.currentID = ticket.id;
    el.jobMeta.textContent = ticket.id;
    await pollJob(ticket.id, {
      stages: el.stages,
      meta: el.jobMeta,
      error: el.millError,
      onDone: async (id) => {
        await loadResults(id);
        el.outHint.textContent = "Готово: " + (convertTarget() === "json" ? "JSON" : "XML");
      },
      onFail: (job) => {
        setError(el.millError, job.error || "задача завершилась ошибкой");
        state.results = { xml: "", json: "" };
        showOutput();
        el.output.classList.add("is-fail");
        el.output.textContent = job.error || "ошибка";
      },
    });
    await refreshJobs();
  } catch (err) {
    setError(el.millError, String(err.message || err));
    setStages(el.stages, "");
  } finally {
    state.busy = false;
    el.convert.disabled = false;
  }
}

function parseTicket(got) {
  if (got.xml) {
    return {
      id: (got.text.match(/<id>([^<]+)<\/id>/) || [])[1],
      status: (got.text.match(/<status>([^<]+)<\/status>/) || [])[1],
    };
  }
  return JSON.parse(got.text);
}

async function pollJob(id, opts) {
  const deadline = Date.now() + 30000;
  while (Date.now() < deadline) {
    const res = await fetch("/jobs/" + id);
    const got = await readResponse(res);
    if (!res.ok) {
      setError(opts.error, parseError(got.text, got.xml));
      setStages(opts.stages, "failed");
      return;
    }
    const job = JSON.parse(got.text);
    setStages(opts.stages, job.status);
    opts.meta.textContent = job.id + " · " + (STATUS_LABEL[job.status] || job.status);
    if (job.status === "done") {
      await opts.onDone(id, job);
      return;
    }
    if (job.status === "failed") {
      opts.onFail(job);
      return;
    }
    await sleep(180);
  }
  setError(opts.error, "Истекло время ожидания задачи");
}

async function loadResults(id) {
  const [xml, json] = await Promise.all([
    fetch("/jobs/" + id + "/xml"),
    fetch("/jobs/" + id + "/json"),
  ]);
  const xmlBody = await xml.text();
  const jsonBody = await json.text();
  if (!xml.ok) throw new Error(parseError(xmlBody, true));
  if (!json.ok) throw new Error(parseError(jsonBody, false));
  state.results = { xml: xmlBody, json: jsonBody };
  showOutput();
}

async function refreshJobs() {
  const res = await fetch("/jobs");
  if (!res.ok) return;
  const jobs = await res.json();
  renderJobList(el.jobs, jobs.filter((job) => !isSchemaJob(job)));
  renderJobList(el.schemaJobs, jobs.filter(isSchemaJob));
}

function isSchemaJob(job) {
  return job.kind === "schema" || job.format === "xsd" || job.format === "jsontemplate";
}

function renderJobList(node, jobs) {
  if (!node) return;
  if (!jobs.length) {
    node.innerHTML = '<li class="empty">Пока пусто</li>';
    return;
  }
  node.innerHTML = jobs
    .slice(0, 40)
    .map((job) => {
      const when = formatTime(job.updated_at || job.created_at);
      return `<li data-id="${escapeAttr(job.id)}" data-status="${escapeAttr(job.status)}">
        <span class="dot ${escapeAttr(job.status)}"></span>
        <div>
          <div class="jid">${escapeHTML(job.id.slice(0, 8))}…</div>
          <div class="jsub">${escapeHTML(job.format)} · ${escapeHTML(STATUS_LABEL[job.status] || job.status)} · ${escapeHTML(when)}</div>
        </div>
      </li>`;
    })
    .join("");
}

function formatTime(iso) {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function escapeHTML(s) {
  return String(s)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function escapeAttr(s) {
  return escapeHTML(s).replaceAll('"', "&quot;");
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function loadSample() {
  setEditor("source", state.format === "xml" ? SAMPLE_XML : SAMPLE_JSON);
}

function loadSchemaSample() {
  setEditor("schema", state.schemaSrc === "xsd" ? SAMPLE_XSD : SAMPLE_TEMPLATE);
}

function setSchemaStatus() {
  if (state.schema) {
    el.schemaStatus.textContent = "Схема: " + (state.schemaRoot || "root");
    return;
  }
  el.schemaStatus.textContent = "Каталог XML ⇄ JSON";
}

async function refreshSchema() {
  const res = await fetch("/schema");
  if (res.status === 404) {
    state.schema = false;
    state.schemaRoot = "";
    setSchemaStatus();
    return;
  }
  const jsonBody = await res.text();
  if (!res.ok) {
    setError(el.schemaError, parseError(jsonBody, false));
    return;
  }
  state.schema = true;
  state.schemaRoot = "schema";
  try {
    const info = JSON.parse(jsonBody);
    if (info.root) state.schemaRoot = info.root;
  } catch {
    /* keep default */
  }
  setSchemaStatus();
}

async function parseSchema() {
  const body = editorText("schema");
  if (!body.trim()) {
    setError(el.schemaError, "Пустое тело схемы");
    return;
  }
  state.schemaBusy = true;
  el.schemaParse.disabled = true;
  setError(el.schemaError, "");
  setStages(el.schemaStages, "queued");
  el.schemaMeta.textContent = "Отправка…";
  state.schemaResults = { xml: "", json: "" };
  showSchemaOutput();
  try {
    const asXSD = state.schemaSrc === "xsd";
    const res = await fetch(asXSD ? "/xsd" : "/jsontemplate", {
      method: "POST",
      headers: { "Content-Type": asXSD ? "application/xml" : "application/json" },
      body,
    });
    const got = await readResponse(res);
    if (!res.ok) {
      setStages(el.schemaStages, "");
      setError(el.schemaError, parseError(got.text, got.xml));
      el.schemaMeta.textContent = "HTTP " + res.status;
      return;
    }
    const ticket = parseTicket(got);
    if (!ticket.id) throw new Error("сервер не вернул id задачи");
    state.schemaID = ticket.id;
    el.schemaMeta.textContent = ticket.id;
    await pollJob(ticket.id, {
      stages: el.schemaStages,
      meta: el.schemaMeta,
      error: el.schemaError,
      onDone: async (id) => {
        await loadSchemaResults(id);
        await refreshSchema();
        el.schemaHint.textContent = asXSD ? "Готово: JSON template" : "Готово: XSD";
      },
      onFail: (job) => {
        setError(el.schemaError, job.error || "задача завершилась ошибкой");
        state.schemaResults = { xml: "", json: "" };
        showSchemaOutput();
        el.schemaOutput.classList.add("is-fail");
        el.schemaOutput.textContent = job.error || "ошибка";
      },
    });
    await refreshJobs();
  } catch (err) {
    setError(el.schemaError, String(err.message || err));
    setStages(el.schemaStages, "");
  } finally {
    state.schemaBusy = false;
    el.schemaParse.disabled = false;
  }
}

async function loadSchemaResults(id) {
  const [xml, json] = await Promise.all([
    fetch("/jobs/" + id + "/xml"),
    fetch("/jobs/" + id + "/json"),
  ]);
  const xmlBody = await xml.text();
  const jsonBody = await json.text();
  if (!xml.ok) throw new Error(parseError(xmlBody, true));
  if (!json.ok) throw new Error(parseError(jsonBody, false));
  state.schemaResults = { xml: xmlBody, json: jsonBody };
  state.schema = true;
  state.schemaRoot = templateRootName(jsonBody) || "schema";
  showSchemaOutput();
}

async function clearJobs() {
  await fetch("/jobs?kind=document", { method: "DELETE" });
  await refreshJobs();
}

async function clearSchemaJobs() {
  await fetch("/jobs?kind=schema", { method: "DELETE" });
  await refreshJobs();
}

async function clearSchema() {
  await fetch("/schema", { method: "DELETE" });
  setError(el.schemaError, "");
  state.schemaResults = { xml: "", json: "" };
  await refreshSchema();
  syncSchemaDirection();
}

async function openStore() {
  el.store.hidden = false;
  el.storeXML.textContent = "Загрузка…";
  el.storeJSON.textContent = "Загрузка…";
  const [xml, json] = await Promise.all([fetch("/xml"), fetch("/json")]);
  el.storeXML.textContent = xml.ok ? displayBody("xml", await xml.text()) : parseError(await xml.text(), true);
  el.storeJSON.textContent = json.ok ? displayBody("json", await json.text()) : parseError(await json.text(), false);
}

function bindDrop(zone, onFile) {
  ["dragenter", "dragover"].forEach((name) => {
    zone.addEventListener(name, (ev) => {
      ev.preventDefault();
      zone.classList.add("is-over");
    });
  });
  ["dragleave", "drop"].forEach((name) => {
    zone.addEventListener(name, (ev) => {
      ev.preventDefault();
      zone.classList.remove("is-over");
    });
  });
  zone.addEventListener("drop", (ev) => {
    const file = ev.dataTransfer && ev.dataTransfer.files && ev.dataTransfer.files[0];
    if (file) onFile(file);
  });
}

function fileExt(name) {
  const m = String(name || "").toLowerCase().match(/(\.[a-z0-9]+)$/);
  return m ? m[1] : "";
}

function requiredExts(format) {
  if (format === "json") return [".json"];
  if (format === "jsontemplate") return [".jsont", ".jsontemplate"];
  if (format === "xsd") return [".xsd"];
  return [".xml"];
}

function extError(format) {
  return "Нужен файл с расширением " + requiredExts(format).join(" или ");
}

function acceptFor(format) {
  const exts = requiredExts(format).join(",");
  if (format === "json" || format === "jsontemplate") return exts + ",application/json";
  if (format === "xsd") return exts + ",application/xml,text/xml,application/xsd+xml";
  return exts + ",application/xml,text/xml";
}

function setFileAccept(input, format) {
  if (input) input.accept = acceptFor(format);
}

function fileMatchesFormat(file, format) {
  return requiredExts(format).includes(fileExt(file && file.name));
}

function readConvertFile(file) {
  if (!fileMatchesFormat(file, state.format)) {
    setError(el.millError, extError(state.format));
    return;
  }
  setError(el.millError, "");
  file.text().then((text) => setEditor("source", text));
}

function readSchemaFile(file) {
  const format = state.schemaSrc === "xsd" ? "xsd" : "jsontemplate";
  if (!fileMatchesFormat(file, format)) {
    setError(el.schemaError, extError(format));
    return;
  }
  setError(el.schemaError, "");
  file.text().then((text) => setEditor("schema", text));
}

function setFormat(format) {
  const changed = state.format !== format;
  state.format = format;
  for (const btn of document.querySelectorAll("[data-format]")) {
    const on = btn.dataset.format === format;
    btn.classList.toggle("is-on", on);
    btn.setAttribute("aria-selected", String(on));
  }
  if (changed) resetConvertOutput();
  syncConvertDirection();
}

function setSchemaSrc(format) {
  const changed = state.schemaSrc !== format;
  state.schemaSrc = format;
  for (const btn of document.querySelectorAll("[data-schema-src]")) {
    const on = btn.dataset.schemaSrc === format;
    btn.classList.toggle("is-on", on);
    btn.setAttribute("aria-selected", String(on));
  }
  if (changed) resetSchemaOutput();
  syncSchemaDirection();
}

function download(text, format, name) {
  const body = formatBody(format, text || "");
  const blob = new Blob([body], { type: contentType(format) + ";charset=utf-8" });
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = name;
  a.click();
  URL.revokeObjectURL(a.href);
}

document.querySelectorAll("[data-win]").forEach((btn) => {
  btn.addEventListener("click", () => setWindow(btn.dataset.win));
});
document.querySelectorAll("[data-format]").forEach((btn) => {
  btn.addEventListener("click", () => {
    setFormat(btn.dataset.format);
    loadSample();
  });
});
document.querySelectorAll("[data-schema-src]").forEach((btn) => {
  btn.addEventListener("click", () => {
    setSchemaSrc(btn.dataset.schemaSrc);
    loadSchemaSample();
  });
});
el.convert.addEventListener("click", submit);
el.schemaParse.addEventListener("click", parseSchema);
document.getElementById("copy-out").addEventListener("click", async () => {
  const format = convertTarget();
  await navigator.clipboard.writeText(exportBody(format, state.results[format]));
  el.outHint.textContent = isBulky(state.results[format]) ? "Скопирован полный текст" : "Скопировано";
});
document.getElementById("schema-copy").addEventListener("click", async () => {
  const format = schemaTarget();
  const text = state.schemaResults[format];
  await navigator.clipboard.writeText(exportBody(format, text));
  el.schemaHint.textContent = isBulky(text) ? "Скопирован полный текст" : "Скопировано";
});
document.getElementById("download-out").addEventListener("click", () => {
  const format = convertTarget();
  download(state.results[format], format, "document." + format);
  el.outHint.textContent = "Скачан отформатированный файл";
});
document.getElementById("schema-download").addEventListener("click", () => {
  const format = schemaTarget();
  download(state.schemaResults[format], format, format === "json" ? "schema.jsont" : "schema.xsd");
  el.schemaHint.textContent = "Скачан отформатированный файл";
});
document.getElementById("open-store").addEventListener("click", openStore);
document.getElementById("close-store").addEventListener("click", () => {
  el.store.hidden = true;
});
document.getElementById("clear-schema").addEventListener("click", clearSchema);
document.getElementById("clear-jobs").addEventListener("click", clearJobs);
document.getElementById("clear-schema-jobs").addEventListener("click", clearSchemaJobs);
document.getElementById("file").addEventListener("change", (ev) => {
  const file = ev.target.files && ev.target.files[0];
  if (file) readConvertFile(file);
  ev.target.value = "";
});
document.getElementById("schema-file").addEventListener("change", (ev) => {
  const file = ev.target.files && ev.target.files[0];
  if (file) readSchemaFile(file);
  ev.target.value = "";
});
document.addEventListener("keydown", (ev) => {
  if (ev.key === "Escape") el.store.hidden = true;
});
el.source.addEventListener("input", () => onEditorInput("source"));
el.schemaSource.addEventListener("input", () => onEditorInput("schema"));
el.source.addEventListener("paste", (ev) => onEditorPaste("source", ev));
el.schemaSource.addEventListener("paste", (ev) => onEditorPaste("schema", ev));
bindDrop(el.drop, readConvertFile);
bindDrop(el.schemaDrop, readSchemaFile);
el.jobs.addEventListener("click", async (ev) => {
  const row = ev.target.closest("li[data-id]");
  if (!row) return;
  const id = row.dataset.id;
  state.currentID = id;
  el.jobMeta.textContent = id;
  setError(el.millError, "");
  if (row.dataset.status === "done") {
    setStages(el.stages, "done");
    try {
      await loadResults(id);
    } catch (err) {
      state.results = { xml: "", json: "" };
      showOutput();
      setError(el.millError, String(err.message || err));
    }
    return;
  }
  await pollJob(id, {
    stages: el.stages,
    meta: el.jobMeta,
    error: el.millError,
    onDone: async (jobID) => {
      await loadResults(jobID);
      el.outHint.textContent = "Готово: " + (convertTarget() === "json" ? "JSON" : "XML");
    },
    onFail: (job) => {
      setError(el.millError, job.error || "задача завершилась ошибкой");
      state.results = { xml: "", json: "" };
      showOutput();
      el.output.classList.add("is-fail");
      el.output.textContent = job.error || "ошибка";
    },
  });
  await refreshJobs();
});
el.schemaJobs.addEventListener("click", async (ev) => {
  const row = ev.target.closest("li[data-id]");
  if (!row) return;
  const id = row.dataset.id;
  state.schemaID = id;
  el.schemaMeta.textContent = id;
  setError(el.schemaError, "");
  if (row.dataset.status === "done") {
    setStages(el.schemaStages, "done");
    try {
      await loadSchemaResults(id);
    } catch (err) {
      state.schemaResults = { xml: "", json: "" };
      showSchemaOutput();
      setError(el.schemaError, String(err.message || err));
    }
    return;
  }
  await pollJob(id, {
    stages: el.schemaStages,
    meta: el.schemaMeta,
    error: el.schemaError,
    onDone: async (jobID) => {
      await loadSchemaResults(jobID);
      await refreshSchema();
      el.schemaHint.textContent = "Готово";
    },
    onFail: (job) => {
      setError(el.schemaError, job.error || "задача завершилась ошибкой");
      state.schemaResults = { xml: "", json: "" };
      showSchemaOutput();
      el.schemaOutput.classList.add("is-fail");
      el.schemaOutput.textContent = job.error || "ошибка";
    },
  });
  await refreshJobs();
});

loadSample();
loadSchemaSample();
syncConvertDirection();
syncSchemaDirection();
setStages(el.stages, "");
setStages(el.schemaStages, "");
refreshJobs();
refreshSchema();
