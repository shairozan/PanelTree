const $ = (id) => document.getElementById(id);
const state = {
  session: null,
  project: null,
  view: null,
  page: null,
  panel: null,
  layer: null,
  originals: [],
  selectedArt: "",
  source: "",
  supports: [],
  quote: null,
  jobs: [],
  busy: false,
};
const text = (tag, value, cls) => {
  const e = document.createElement(tag);
  e.textContent = value;
  if (cls) e.className = cls;
  return e;
};
const button = (label, fn, cls) => {
  const e = text("button", label, cls);
  e.type = "button";
  e.addEventListener("click", () => task(fn));
  return e;
};
let noticeTimer;
function notice(message) {
  $("status").textContent = message;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => ($("status").textContent = ""), 3500);
}
function showError(error) {
  $("error").hidden = false;
  $("error").querySelector("span").textContent = error.message || String(error);
}
document.addEventListener(
  "error",
  (event) => {
    if (event.target instanceof HTMLImageElement) {
      const label = event.target.alt || "image";
      const url = new URL(event.target.src, location.origin);
      const item =
        url.searchParams.get("job")?.slice(0, 12) ||
        url.searchParams.get("path")?.split("/").pop() ||
        label;
      event.target.alt = "Artwork unavailable: " + label;
      showError(
        new Error(
          "Artwork unavailable: " +
            item +
            ". Refresh the project or restore this asset.",
        ),
      );
    }
  },
  true,
);
let taskQueue = Promise.resolve();
async function task(fn) {
  const previous = taskQueue;
  let finish;
  taskQueue = new Promise((resolve) => {
    finish = resolve;
  });
  await previous;
  state.busy = true;
  $("controls").disabled = true;
  $("home").disabled = true;
  $("characters-nav").disabled = true;
  try {
    await fn();
  } catch (e) {
    showError(e);
  } finally {
    state.busy = false;
    $("controls").disabled = false;
    $("home").disabled = false;
    $("characters-nav").disabled = false;
    finish();
  }
}
async function api(path, body, method) {
  const response = await fetch(path, {
    method: method || (body === undefined ? "GET" : "POST"),
    headers: {
      "Content-Type": "application/json",
      "X-PanelTree-Token": state.session?.token || "",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await response.json();
  if (!response.ok)
    throw new Error(data.error || `Request failed (${response.status})`);
  return data;
}
const base = () => "/api/projects/" + state.project.id;
function screen(name) {
  for (const id of [
    "library",
    "workspace",
    "character-screen",
    "reference-screen",
  ])
    $(id).hidden = id !== name;
}
function target() {
  if (!state.layer) throw new Error("Select a layer first.");
  return { page: state.page, panel: state.panel, layer: state.layer };
}
function currentPage() {
  return state.view?.documents.find((d) => d.document.page?.id === state.page);
}
function findLayer(items, id) {
  for (const item of items) {
    if (item.id === id) return item;
    const child = findLayer(item.children || [], id);
    if (child) return child;
  }
  return null;
}
function selection() {
  const page = currentPage()?.document.page;
  const panel = page?.panels.find((p) => p.id === state.panel);
  return findLayer(panel?.layers || [], state.layer);
}
function relativeFile(file) {
  file = file.replaceAll("\\", "/");
  const root = state.view.documents[0].file
    .replaceAll("\\", "/")
    .replace(/[^/]+$/, "");
  return root && file.startsWith(root) ? file.slice(root.length) : file;
}
async function authoringSource(lettering) {
  const assets = await api(base() + "/authoring", {});
  const prefix = "../".repeat(
    relativeFile(currentPage().file).split("/").length - 1,
  );
  return lettering
    ? {
        kind: "text",
        text: "Your words here",
        font: prefix + assets.font,
        font_size: 28,
        color: "#242923",
      }
    : { kind: "image", path: prefix + assets.blank };
}
async function listProjects() {
  const projects = await api("/api/projects");
  const container = $("projects");
  container.replaceChildren();
  if (!projects.length)
    container.append(
      text(
        "div",
        "An empty page is a good beginning. Create your first project, or open an existing story.",
        "empty",
      ),
    );
  for (const project of projects) {
    const card = button("", () => openProject(project), "project-card");
    const cover = text("div", "", "project-cover");
    const icon = text("div", "", "comic-icon");
    for (let i = 0; i < 3; i++) icon.append(document.createElement("i"));
    cover.append(icon);
    const info = text("div", "", "project-info");
    info.append(
      text("strong", project.name),
      text(
        "small",
        project.backend === "files" ? "Portable files" : "PostgreSQL project",
      ),
    );
    card.append(cover, info);
    container.append(card);
  }
}
async function openProject(project) {
  state.project = project;
  state.page = null;
  state.panel = null;
  state.layer = null;
  state.source = "";
  state.supports = [];
  state.ref = null;
  state.authored = null;
  state.selectedArt = "";
  state.quote = null;
  $("quote").hidden = true;
  $("download").replaceChildren();
  drawReferences();
  $("reference-candidates").replaceChildren();
  $("reference-cards").replaceChildren();
  $("reference-summary").textContent = "";
  $("authored-package").textContent = "";
  $("story-title").textContent = project.name;
  $("backend-label").textContent =
    project.backend === "files"
      ? "PORTABLE FILE PROJECT"
      : "POSTGRESQL PROJECT";
  screen("workspace");
  await refresh();
  await loadOriginals();
  await jobs();
}
async function refresh(preview = true) {
  state.view = await api(base());
  if (!state.view.pages.some((p) => p.page.id === state.page))
    state.page = state.view.pages[0]?.page.id;
  drawNavigation();
  if (preview) await buildPreview();
}
function drawNavigation() {
  $("pages").replaceChildren();
  const book = state.view.documents.find((d) => d.document.book)?.document.book;
  if (book) $("pages").append(text("h3", book.title || book.id));
  const normalize = (path) => {
    const parts = [];
    for (const part of path.split("/")) {
      if (part === "..") parts.pop();
      else if (part && part !== ".") parts.push(part);
    }
    return parts.join("/");
  };
  const chapters = new Map();
  for (const doc of state.view.documents.filter((d) => d.document.chapter)) {
    const prefix = relativeFile(doc.file).replace(/[^/]+$/, "");
    for (const path of doc.document.chapter.pages)
      chapters.set(
        normalize(prefix + path),
        doc.document.chapter.title || doc.document.chapter.id,
      );
  }
  let chapter = "";
  for (const p of state.view.pages) {
    const next = chapters.get(relativeFile(p.file)) || "";
    if (next !== chapter) {
      chapter = next;
      $("pages").append(text("h3", chapter));
    }
    const b = button(
      p.page.id,
      async () => {
        state.page = p.page.id;
        state.panel = null;
        state.layer = null;
        drawNavigation();
        await buildPreview();
      },
      p.page.id === state.page ? "current" : "",
    );
    $("pages").append(b);
  }
  const page = currentPage()?.document.page;
  if (!page) return;
  $("page-label").textContent = page.id;
  const outline = $("outline");
  outline.replaceChildren();
  if (!page.panels.some((p) => p.id === state.panel))
    state.panel = page.panels[0]?.id;
  if (!selection())
    state.layer = page.panels.find((p) => p.id === state.panel)?.layers[0]?.id;
  for (const panel of page.panels) {
    outline.append(
      button(
        panel.id,
        () => {
          state.panel = panel.id;
          state.layer = panel.layers[0]?.id;
          drawNavigation();
        },
        "panel-label",
      ),
    );
    const walk = (ls, depth) => {
      for (const layer of ls) {
        const b = button(
          layer.role || layer.id,
          () => {
            state.panel = panel.id;
            state.layer = layer.id;
            drawNavigation();
          },
          "layer" +
            (state.layer === layer.id && state.panel === panel.id
              ? " current"
              : ""),
        );
        b.style.paddingLeft = 8 + depth * 10 + "px";
        b.title = layer.id;
        outline.append(b);
        walk(layer.children || [], depth + 1);
      }
    };
    walk(panel.layers, 0);
  }
  drawProperties();
}
function drawProperties() {
  const layer = selection();
  $("properties-form").hidden = !layer;
  if (!layer) return;
  const character = layer.source?.character;
  $("character-binding").textContent = character
    ? "Pinned character: " +
      character.package +
      (character.reference_set
        ? " · references " + character.reference_set.version
        : "")
    : "";
  $("character-settings").hidden = !character;
  for (const key of ["costume", "expression", "pose"])
    $(key).value = character?.[key] || "";
  $("selection-title").textContent = layer.id;
  const status =
    state.view.layers?.[`${state.page}/${state.panel}/${state.layer}`] || {};
  $("selection-state").textContent = status.lock
    ? `LOCKED · ${status.lock}`
    : status.state || "draft";
  $("selection-detail").textContent =
    [
      status.pin ? "Approved artwork pinned." : "",
      status.manual ? "Manual artwork selected." : "",
      status.stale ? "Selection is stale; review its changed source." : "",
    ]
      .filter(Boolean)
      .join(" ") || "Original artwork is preserved.";
  $("role").value = layer.role || "";
  $("lettering").value = layer.source?.text || "";
  $("text-label").hidden = layer.source?.kind !== "text";
  const f = layer.frame || { x: 0, y: 0, width: 1, height: 1 };
  for (const k of ["x", "y", "width", "height"]) $(k).value = f[k];
  $("sx").value = layer.transform?.scale_x ?? 1;
  $("sy").value = layer.transform?.scale_y ?? 1;
  $("rotation").value = layer.transform?.rotation || 0;
  $("font-size").value = layer.source?.font_size || 24;
}
async function buildPreview() {
  notice("Building page preview…");
  const result = await api(base() + "/preview", { page: state.page });
  $("page-preview").src = result.url;
  $("page-preview").hidden = false;
  $("preview-empty").hidden = true;
  notice("Preview ready · no generation");
  return result;
}
async function editDocument(document) {
  const doc = currentPage();
  await api(base() + "/edit", {
    revision: state.view.revision,
    edits: [{ file: relativeFile(doc.file), document }],
  });
  await refresh();
}
async function op(action, extra = {}) {
  await api(base() + "/edit", {
    revision: state.view.revision,
    operations: [{ target: target(), action, ...extra }],
  });
  await refresh();
}
function newID(prefix) {
  return prefix + "-" + crypto.randomUUID().slice(0, 8);
}
async function addPanel() {
  const doc = structuredClone(currentPage().document);
  const id = newID("panel");
  const layer = newID("art");
  doc.page.panels.push({
    id,
    layers: [
      {
        id: layer,
        role: "New panel",
        source: await authoringSource(false),
      },
    ],
  });
  const leaf = { panel: id };
  if (doc.page.layout.panel)
    doc.page.layout = {
      type: "column",
      children: [doc.page.layout, leaf],
      gutter: 12,
    };
  else doc.page.layout.children.push(leaf);
  state.panel = id;
  state.layer = layer;
  await editDocument(doc);
}
async function addLayer(kind) {
  const doc = structuredClone(currentPage().document);
  const panel = doc.page.panels.find((p) => p.id === state.panel);
  if (!panel) throw new Error("Select a panel.");
  const id = newID(kind === "text" ? "lettering" : "art");
  const source = await authoringSource(kind === "text");
  panel.layers.push({
    id,
    role: kind === "text" ? "Lettering" : "Artwork",
    source,
    frame: { x: 0.08, y: 0.08, width: 0.84, height: 0.24 },
  });
  state.layer = id;
  await editDocument(doc);
  showTab("properties");
}
async function reorderPanel(delta) {
  const doc = structuredClone(currentPage().document);
  const leaves = [];
  const walk = (node) => {
    if (node.panel) leaves.push(node);
    else for (const child of node.children || []) walk(child);
  };
  walk(doc.page.layout);
  const i = leaves.findIndex((n) => n.panel === state.panel),
    j = i + delta;
  if (i < 0 || j < 0 || j >= leaves.length) return;
  [leaves[i].panel, leaves[j].panel] = [leaves[j].panel, leaves[i].panel];
  const order = leaves.map((n) => n.panel);
  doc.page.panels.sort((a, b) => order.indexOf(a.id) - order.indexOf(b.id));
  await editDocument(doc);
}
$("panel-earlier").onclick = () => task(() => reorderPanel(-1));
$("panel-later").onclick = () => task(() => reorderPanel(1));
async function reorder(delta) {
  const doc = structuredClone(currentPage().document);
  const panel = doc.page.panels.find((p) => p.id === state.panel);
  function move(ls) {
    const i = ls.findIndex((l) => l.id === state.layer);
    if (i >= 0) {
      const j = i + delta;
      if (j >= 0 && j < ls.length) [ls[i], ls[j]] = [ls[j], ls[i]];
      return true;
    }
    return ls.some((l) => move(l.children || []));
  }
  move(panel.layers);
  await editDocument(doc);
}
function showTab(name) {
  $("properties").hidden = name !== "properties";
  $("render").hidden = name !== "render";
  $("properties-tab").classList.toggle("selected", name === "properties");
  $("render-tab").classList.toggle("selected", name === "render");
}
const media = (path, job = "") =>
  base() + "/media?" + new URLSearchParams({ path, job, thumbnail: "1" });
async function loadOriginals() {
  state.originals = await api(base() + "/artworks");
  $("artworks").replaceChildren();
  for (const art of state.originals) {
    const b = button(
      "",
      () => {
        state.selectedArt = art.path;
        loadOriginals();
      },
      "thumb" + (art.path === state.selectedArt ? " selected" : ""),
    );
    const im = document.createElement("img");
    im.src = media(art.path);
    im.alt = art.attribution || "Imported artwork";
    b.title = `${art.license || "Unknown license"} · ${art.attribution || "Unknown attribution"}`;
    b.append(im);
    $("artworks").append(b);
  }
}
function drawReferences() {
  $("references").replaceChildren();
  if (!state.source && !state.supports.length)
    $("references").append(
      text("p", "Select an imported original as the source.", "hint"),
    );
  for (const [index, path] of [state.source, ...state.supports].entries()) {
    if (!path) continue;
    const row = text("div", "", "reference-row");
    const im = document.createElement("img");
    im.src = media(path);
    im.alt = index === 0 ? "Source image" : `Supporting reference ${index}`;
    row.append(
      im,
      text("span", index === 0 ? "1 · SOURCE" : `${index + 1} · SUPPORT`),
    );
    row.append(
      button("Remove", () => {
        if (index === 0) state.source = "";
        else state.supports.splice(index - 1, 1);
        drawReferences();
      }),
    );
    if (index > 1)
      row.append(
        button("Earlier", () => {
          [state.supports[index - 2], state.supports[index - 1]] = [
            state.supports[index - 1],
            state.supports[index - 2],
          ];
          drawReferences();
        }),
      );
    $("references").append(row);
  }
}
function capability() {
  return state.session.renderers[Number($("profile").value) || 0];
}
function showCapability() {
  const c = capability();
  $("capability").textContent = [
    c.model || c.name,
    c.operation || "local artwork",
    c.quality ? `Quality: ${c.quality}` : "",
    c.size ? `Size: ${c.size}` : "",
    c.image_conditioning
      ? `PNG · max ${c.image_conditioning.max_pixels?.toLocaleString()} pixels · source first`
      : "",
    ...(c.limitations || []),
  ]
    .filter(Boolean)
    .join(" · ");
  providerNotice($("provider-notice"), c);
}
function providerNotice(n, c) {
  n.replaceChildren();
  if (!c) return;
  if (c.attribution) n.append(text("strong", c.attribution));
  if (c.usage_policy) {
    const a = text("a", "Usage policy");
    a.href = c.usage_policy;
    a.target = "_blank";
    a.rel = "noreferrer";
    n.append(document.createTextNode(" · "), a);
  }
}
function showQuote(q) {
  state.quote = q;
  $("quote").hidden = false;
  $("quote-status").textContent =
    q.status === "quoted" ? "Provider estimate" : "Quote unavailable";
  $("quote-detail").textContent =
    q.status === "quoted"
      ? JSON.stringify(q.quote)
      : q.reason || "Review the provider pricing before starting.";
}
async function jobs(poll = false) {
  if (!state.project) return;
  const project = state.project;
  const result = await api(base() + "/jobs");
  if (project !== state.project || (poll && state.busy)) return;
  if (poll && JSON.stringify(result) === JSON.stringify(state.jobs)) return;
  state.jobs = result;
  const c = $("jobs");
  c.replaceChildren();
  for (const j of state.jobs) {
    const card = text("div", "", "job");
    card.append(
      text("strong", `${j.state} · ${j.progress}%`),
      text("p", j.id.slice(0, 12)),
    );
    if (j.diagnostic) card.append(text("p", j.diagnostic.message));
    if (j.character_status)
      card.append(
        text("p", `${j.character_status}: ${j.character_diagnostic || ""}`),
      );
    if (j.state === "succeeded") {
      const compare = text("div", "", "compare");
      for (const [label, url] of [
        [
          "Source",
          (j.generation_provenance?.image_inputs || []).some((i) =>
            ["character", "reference"].includes(i.role),
          )
            ? base() +
              "/media?" +
              new URLSearchParams({ job: j.id, source: "1" })
            : "",
        ],
        ["Candidate", media("", j.id)],
      ]) {
        const fig = document.createElement("figure");
        if (url) {
          const im = document.createElement("img");
          im.src = url;
          im.alt = label + " artwork";
          fig.append(im);
        }
        fig.append(text("figcaption", label));
        compare.append(fig);
      }
      card.append(compare);
      if (state.ref && Object.values(state.ref.jobs || {}).includes(j.id))
        card.append(
          button("Collect reference candidate", async () => {
            await refAction("collect", { job_id: j.id });
            screen("reference-screen");
          }),
        );
      else if (!j.rejected)
        card.append(
          button("Select candidate", async () => {
            await api(base() + "/select", {
              revision: state.view.revision,
              job: j.id,
            });
            await refresh();
          }),
          button("Reject candidate", async () => {
            await api(base() + "/reject", { job: j.id });
            await jobs();
          }),
        );
      else card.append(text("p", "Rejected · original retained"));
    }
    if (j.state === "queued")
      card.append(
        button("Review estimate", async () =>
          showQuote(await api(base() + "/quote", { job: j.id })),
        ),
      );
    if (["queued", "running"].includes(j.state))
      card.append(
        button("Cancel", async () => {
          await api(base() + "/cancel", { job: j.id });
          await jobs();
        }),
      );
    if (["failed", "cancelled"].includes(j.state) && j.resumable)
      card.append(
        button("Resume known execution", async () =>
          showQuote(await api(base() + "/resume", { job: j.id })),
        ),
      );
    c.append(card);
  }
}
async function characterLibrary() {
  screen("character-screen");
  const items = await api("/api/library");
  const query = $("character-search").value.toLowerCase();
  const pinned = items.find(
    (v) => v.package === selection()?.source?.character?.package,
  );
  $("character-list").replaceChildren();
  for (const v of items.filter((v) => v.id.toLowerCase().includes(query))) {
    const pkg = await api(
      "/api/library/detail?" +
        new URLSearchParams({ id: v.id, version: v.version }),
    );
    const card = text("article", "", "card");
    card.append(
      text("h2", v.id),
      text("p", "Published version " + v.version),
      text("p", pkg.description),
    );
    if (pinned?.id === v.id)
      card.append(
        text(
          "p",
          pinned.version === v.version
            ? "Pinned in this story"
            : "Different version available · current " + pinned.version,
          "pill",
        ),
      );
    for (const ref of pkg.references || []) {
      if (ref.path.endsWith(".png")) {
        const im = document.createElement("img");
        im.src =
          "/api/library/media?" +
          new URLSearchParams({
            id: v.id,
            version: v.version,
            path: ref.path,
            thumbnail: "1",
          });
        im.alt = ref.description || ref.id;
        card.append(im);
      }
      card.append(
        text(
          "p",
          `${ref.license || "Unknown license"} · ${ref.attribution || "Unknown attribution"}`,
        ),
      );
    }
    for (const [direction, hash] of Object.entries(pkg.published_cards || {})) {
      const figure = document.createElement("figure");
      const im = document.createElement("img");
      im.alt = "Approved " + direction + " reference card";
      im.src =
        "/api/library/media?" +
        new URLSearchParams({
          id: v.id,
          version: v.version,
          path: ".paneltree/assets/" + hash + ".png",
          thumbnail: "1",
        });
      figure.append(im, text("figcaption", direction));
      card.append(figure);
    }
    if (v.reference_set)
      card.append(
        text("p", `Reference set ${v.reference_set} · ${v.reference_version}`),
      );
    card.append(
      button("Use pinned version", async () => {
        if (!state.project)
          throw new Error("Open a story and select its artwork layer first.");
        await api(base() + "/library", {
          action: "use",
          id: v.id,
          version: v.version,
          expected_revision: state.view.revision,
          target: target(),
        });
        screen("workspace");
        showTab("properties");
        await refresh();
      }),
    );
    $("character-list").append(card);
  }
  if (!items.length)
    $("character-list").append(
      text(
        "p",
        "No shared characters yet. Shared publication uses the host-configured PostgreSQL library.",
        "empty",
      ),
    );
}
async function applyCharacter(path) {
  const doc = structuredClone(currentPage().document);
  const layer = findLayer(
    doc.page.panels.find((p) => p.id === state.panel).layers,
    state.layer,
  );
  if (!layer?.source) throw new Error("Select an artwork leaf layer.");
  layer.source.character = { ...(layer.source.character || {}), package: path };
  await editDocument(doc);
}
async function refAction(action, extra = {}) {
  if (!state.project) throw new Error("Open a story first.");
  state.ref = await api(base() + "/references", {
    set: $("reference-id").value,
    action,
    revision: state.ref?.revision || "",
    ...extra,
  });
  drawReferenceSet();
  return state.ref;
}
function drawReferenceSet() {
  const set = state.ref;
  if (!set) return;
  $("reference-summary").textContent =
    `${set.id} · ${Object.keys(set.accepted).length}/8 accepted views`;
  $("reference-candidates").replaceChildren();
  for (const [id, c] of Object.entries(set.candidates)) {
    const card = text("article", "", "card");
    const im = document.createElement("img");
    im.src = media(".paneltree/assets/" + c.image + ".png");
    im.alt = c.slot + " reference";
    const accepted = set.accepted[c.slot] === id;
    const stale = Object.entries(c.approved_against || c.parents || {}).some(
      ([slot, parent]) => set.accepted[slot] !== parent,
    );
    card.append(
      im,
      text(
        "h2",
        (accepted
          ? "Accepted · "
          : c.rejected
            ? "Rejected · "
            : "Candidate · ") + c.slot,
      ),
      text("p", `${c.license} · ${c.attribution}`),
      text(
        "p",
        stale
          ? "Parent changed — regenerate or explicitly reapprove"
          : "Parent lineage current",
      ),
    );
    if (!accepted && !c.rejected) {
      card.append(
        button(
          stale ? "Reapprove against current parents" : "Accept reference",
          () =>
            refAction("accept", {
              slot: c.slot,
              candidate: id,
              reapprove: stale,
            }),
        ),
        button("Reject reference", () =>
          refAction("reject", { slot: c.slot, candidate: id }),
        ),
      );
    }
    $("reference-candidates").append(card);
  }
  $("reference-cards").replaceChildren();
  for (const [version, pub] of Object.entries(set.published)) {
    for (const [direction, hash] of Object.entries(pub.cards)) {
      const card = text("article", "", "card");
      const im = document.createElement("img");
      im.src = media(".paneltree/assets/" + hash + ".png");
      im.alt = version + " " + direction + " card";
      card.append(im, text("h3", version + " · " + direction));
      $("reference-cards").append(card);
    }
  }
}
$("reference-nav").onclick = () => {
  screen("reference-screen");
};
$("reference-back").onclick = () => screen("workspace");
$("character-back").onclick = () =>
  screen(state.project ? "workspace" : "library");
$("reference-create").onclick = () => task(() => refAction("create"));
$("reference-open").onclick = () => task(() => refAction("inspect"));
$("reference-import").onclick = () =>
  task(() => {
    const a = state.originals.find((a) => a.path === state.selectedArt);
    if (!a)
      throw new Error(
        "Import and select an original on the artwork tab first.",
      );
    return refAction("import", {
      slot: $("reference-slot").value,
      path: a.path,
      license: a.license || "Unknown",
      attribution: a.attribution || "Unknown",
    });
  });
$("reference-publish").onclick = () =>
  task(() => refAction("publish", { version: $("reference-version").value }));
$("reference-generate").onclick = () =>
  task(async () => {
    const c = state.session.renderers[Number($("reference-profile").value)];
    if (!c || c.name === "builtin")
      throw new Error("Choose a configured generative profile.");
    const key = crypto.randomUUID();
    const set = await refAction("request", {
      slot: $("reference-slot").value,
      anchor: $("reference-anchor").value,
      key,
      generation: {
        profile: c.profile || "",
        prompt: $("reference-prompt").value,
        seed: Number($("seed").value),
      },
      width: Number($("render-width").value),
      height: Number($("render-height").value),
      license: "Unknown — review provider and source terms",
      attribution: c.attribution || "User-configured renderer",
    });
    showQuote(await api(base() + "/quote", { job: set.jobs[key] }));
    screen("workspace");
    showTab("render");
    await jobs();
  });
$("character-form").onsubmit = (event) => {
  event.preventDefault();
  task(async () => {
    if (!state.project) throw new Error("Open a story first.");
    const a = state.originals.find((a) => a.path === state.selectedArt);
    if (!a) throw new Error("Select an imported original in the story first.");
    const id = $("character-id").value,
      version = $("character-version").value;
    const out = await api(base() + "/character", {
      schema: "paneltree/character/v1",
      id,
      version,
      description: $("character-description").value,
      references: [
        {
          id: "front",
          version,
          path: a.path,
          license: a.license || "Unknown",
          attribution: a.attribution || "Unknown",
          description: "Front reference",
        },
      ],
    });
    state.authored = { id, version, path: out.path };
    $("authored-package").textContent = "Saved " + out.path;
  });
};
$("use-authored").onclick = () =>
  task(async () => {
    if (!state.authored) throw new Error("Create a character package first.");
    await applyCharacter(state.authored.path);
    screen("workspace");
  });
$("publish-character").onclick = () =>
  task(async () => {
    if (!state.authored) throw new Error("Create a character package first.");
    const version = $("library-reference-version").value;
    await api(base() + "/library", {
      action: "publish",
      id: state.authored.id,
      version: state.authored.version,
      package: state.authored.path,
      ...(version
        ? { set: state.ref?.id || "", reference_version: version }
        : {}),
    });
    await characterLibrary();
    notice("Character version published");
  });
$("create-project").addEventListener("submit", (event) => {
  event.preventDefault();
  task(async () => {
    const name = $("project-name").value;
    const backend = $("backend").value;
    const root = $("project-root").value;
    const path =
      backend === "postgres"
        ? name
        : root.replace(/[\\/]$/, "") +
          (root.includes("\\") ? "\\" : "/") +
          name;
    await openProject(await api("/api/projects", { backend, path }));
  });
});
$("register").onclick = () =>
  task(async () =>
    openProject(
      await api("/api/projects", {
        backend: "files",
        path: $("register-path").value,
        register: true,
      }),
    ),
  );
$("home").onclick = () =>
  task(async () => {
    screen("library");
    await listProjects();
  });
$("refresh-projects").onclick = () => task(listProjects);
$("characters-nav").onclick = () => task(characterLibrary);
$("character-search").oninput = () => task(characterLibrary);
$("dismiss-error").onclick = () => ($("error").hidden = true);
$("refresh-error").onclick = () =>
  task(async () => {
    await refresh();
    $("error").hidden = true;
  });
$("preview").onclick = () => task(buildPreview);
$("export").onclick = () =>
  task(async () => {
    const result = await api(base() + "/export", { page: state.page });
    const a = text("a", "Download page PNG");
    a.href = result.url;
    a.download = state.page + ".png";
    $("download").replaceChildren(a);
  });
for (const action of ["undo", "redo"])
  $(action).onclick = () =>
    task(async () => {
      await api(base() + "/" + action, { revision: state.view.revision });
      await refresh();
    });
$("add-panel").onclick = () => task(addPanel);
$("add-text").onclick = () => task(() => addLayer("text"));
$("add-art").onclick = () => task(() => addLayer("art"));
$("move-up").onclick = () => task(() => reorder(-1));
$("move-down").onclick = () => task(() => reorder(1));
$("properties-form").addEventListener("submit", (event) => {
  event.preventDefault();
  task(async () => {
    const doc = structuredClone(currentPage().document);
    const l = findLayer(
      doc.page.panels.find((p) => p.id === state.panel).layers,
      state.layer,
    );
    l.role = $("role").value;
    if (l.source?.character) {
      for (const key of ["costume", "expression", "pose"]) {
        if ($(key).value) l.source.character[key] = $(key).value;
        else delete l.source.character[key];
      }
    }
    l.frame = {
      x: Number($("x").value),
      y: Number($("y").value),
      width: Number($("width").value),
      height: Number($("height").value),
    };
    l.transform = {
      scale_x: Number($("sx").value),
      scale_y: Number($("sy").value),
      rotation: Number($("rotation").value),
    };
    if (l.source?.kind === "text") {
      l.source.text = $("lettering").value;
      l.source.font_size = Number($("font-size").value);
    }
    await editDocument(doc);
    notice("Properties saved");
  });
});
for (const b of document.querySelectorAll("[data-op]"))
  b.onclick = () => task(() => op(b.dataset.op));
$("lock").onclick = () =>
  task(() => op("lock", { scope: $("lock-scope").value }));
$("approve").onclick = () =>
  task(() => {
    const st =
      state.view.layers?.[`${state.page}/${state.panel}/${state.layer}`];
    if (st?.manual || st?.pin) return op("approve");
    const artifact = state.selectedArt;
    if (!artifact)
      throw new Error("Select a candidate or original to approve.");
    return op("approve", { artifact });
  });
$("properties-tab").onclick = () => showTab("properties");
$("render-tab").onclick = () => showTab("render");
$("upload").onchange = () =>
  task(async () => {
    const file = $("upload").files[0];
    if (!file) return;
    if (file.size > 32 * 1024 * 1024)
      throw new Error("PNG must be at most 32 MiB.");
    const r = await fetch(base() + "/upload", {
      method: "POST",
      headers: {
        "Content-Type": "image/png",
        "X-PanelTree-Token": state.session.token,
        "X-Artwork-License": encodeURIComponent($("license").value),
        "X-Artwork-Attribution": encodeURIComponent($("attribution").value),
      },
      body: file,
    });
    const a = await r.json();
    if (!r.ok) throw new Error(a.error);
    state.selectedArt = a.path;
    await loadOriginals();
    notice("Original imported; no selection changed");
  });
$("use-art").onclick = () =>
  task(() => {
    if (!state.selectedArt) throw new Error("Choose an imported original.");
    return op("override", { artifact: state.selectedArt });
  });
$("source-art").onclick = () => {
  state.source = state.selectedArt;
  drawReferences();
};
$("support-art").onclick = () =>
  task(() => {
    if (!state.selectedArt) throw new Error("Choose an imported reference.");
    state.supports.push(state.selectedArt);
    drawReferences();
  });
$("profile").onchange = showCapability;
$("generation-form").addEventListener("submit", (event) => {
  event.preventDefault();
  task(async () => {
    const c = capability();
    const body = {
      revision: state.view.revision,
      target: target(),
      renderer: c.name,
      width: Number($("render-width").value),
      height: Number($("render-height").value),
      key: crypto.randomUUID(),
    };
    if (c.name !== "builtin")
      body.generation = {
        profile: c.profile || "",
        prompt: $("prompt").value,
        seed: Number($("seed").value),
        character_reference: state.source,
        style_references: state.supports,
      };
    showQuote(await api(base() + "/prepare", body));
    await jobs();
  });
});
$("run").onclick = () =>
  task(async () => {
    const q = state.quote;
    await api(base() + "/run", {
      job: q.job.id,
      ticket: q.ticket,
      confirm: true,
    });
    $("quote").hidden = true;
    state.quote = null;
    await jobs();
  });
await task(async () => {
  state.session = await api("/api/session");
  for (const root of state.session.roots) {
    const option = text("option", root);
    option.value = root;
    $("project-root").append(option);
  }
  $("backend").querySelector("[value=postgres]").disabled =
    !state.session.postgres;
  for (const [i, c] of state.session.renderers.entries()) {
    const option = text(
      "option",
      `${c.profile || c.name}${c.model ? " · " + c.model : ""}${c.available ? "" : " (not configured)"}`,
    );
    option.value = i;
    option.disabled = !c.available;
    $("profile").append(option);
    if (c.name !== "builtin")
      $("reference-profile").append(option.cloneNode(true));
  }
  showCapability();
  const referenceNotice = () =>
    providerNotice(
      $("reference-provider-notice"),
      state.session.renderers[Number($("reference-profile").value)],
    );
  $("reference-profile").onchange = referenceNotice;
  referenceNotice();
  drawReferences();
  await listProjects();
});
setInterval(() => {
  if (state.project && !state.busy && !$("workspace").hidden)
    jobs(true).catch(showError);
}, 2500);
