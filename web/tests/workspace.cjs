const fs = require("node:fs");
const path = require("node:path");
const screenshot =
  process.env.PANELTREE_SCREENSHOT ||
  path.join(__dirname, "../test-results/workspace.png");
fs.mkdirSync(path.dirname(screenshot), { recursive: true });
const { test } = require("node:test");

const assert = require("node:assert/strict");

const { chromium } = require("playwright");
const AxeBuilder = require("@axe-core/playwright").default;

const base = process.env.PANELTREE_BROWSER_URL || "http://127.0.0.1:8910";

test("create a project, open an ordered page, edit and export", async () => {
  const browser = await chromium.launch({
    headless: true,
    channel: process.env.PANELTREE_BROWSER_CHANNEL || undefined,
  });

  try {
    const page = await (
      await browser.newContext({
        viewport: { width: 1440, height: 1000 },
      })
    ).newPage();

    page.setDefaultTimeout(8000);

    await page.goto(base);

    await page.getByRole("heading", { name: "Your projects" }).waitFor();

    await page.getByLabel("Project name").fill("browser-story-" + Date.now());

    await page
      .getByRole("button", { name: "Create project", exact: true })
      .click();

    await page.getByRole("heading", { name: /browser-story-/ }).waitFor();

    await page
      .getByRole("img", { name: "Page preview", exact: true })
      .waitFor();

    assert.equal(
      await page.getByRole("button", { name: "page-01", exact: true }).count(),
      1,
    );
    await page.getByText("First Panels", { exact: true }).waitFor();
    await page.getByText("chapter-01", { exact: true }).waitFor();

    await page.getByRole("button", { name: "Add panel", exact: true }).click();

    await page
      .getByRole("button", { name: "Move panel earlier", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Add lettering", exact: true })
      .click();

    await page.getByLabel("Lettering text").fill("A new chapter begins.");

    await page
      .getByRole("button", { name: "Apply properties", exact: true })
      .click();

    await page.getByRole("button", { name: "Undo", exact: true }).click();

    await page.getByRole("button", { name: "Redo", exact: true }).click();

    await page.getByRole("button", { name: "Export PNG", exact: true }).click();

    await page
      .getByRole("link", { name: "Download page PNG", exact: true })
      .waitFor();

    assert.equal(
      await page.locator("#error").isVisible(),
      false,
      await page.locator("#error").innerText(),
    );

    assert.equal(
      await page.getByLabel("Lettering text").inputValue(),
      "A new chapter begins.",
    );

    await page.waitForFunction(
      () => !document.querySelector("#controls").disabled,
    );
    const accessibility = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa"])
      .analyze();
    assert.deepEqual(
      accessibility.violations.map((v) => ({
        id: v.id,
        nodes: v.nodes.map((n) => n.target),
      })),
      [],
    );
    await page.screenshot({ path: screenshot, fullPage: true });
  } finally {
    await browser.close();
  }
});

test("import an original and explicitly review the front reference", async () => {
  const browser = await chromium.launch({
    headless: true,
    channel: process.env.PANELTREE_BROWSER_CHANNEL || undefined,
  });

  try {
    const page = await (
      await browser.newContext({
        viewport: { width: 1440, height: 1000 },
      })
    ).newPage();
    page.setDefaultTimeout(8000);
    await page.goto(base);

    await page.getByLabel("Project name").fill("refs-" + Date.now());
    await page
      .getByRole("button", { name: "Create project", exact: true })
      .click();
    await page
      .getByRole("img", { name: "Page preview", exact: true })
      .waitFor();

    await page
      .getByRole("button", { name: "Artwork & render", exact: true })
      .click();

    const png = Buffer.from(
      await page.evaluate(() => {
        const c = document.createElement("canvas");
        c.width = 64;
        c.height = 96;
        const x = c.getContext("2d");
        x.fillStyle = "#657842";
        x.fillRect(0, 0, 64, 96);
        return c.toDataURL().split(",")[1];
      }),
      "base64",
    );

    await page
      .getByLabel("Artwork license", { exact: true })
      .fill("Own artwork");
    await page.getByLabel("Artist / attribution").fill("Test artist");

    await page.getByLabel("Import PNG artwork").setInputFiles({
      name: "original.png",
      mimeType: "image/png",
      buffer: png,
    });

    await page.getByRole("img", { name: "Test artist", exact: true }).waitFor();

    await page
      .getByRole("button", { name: "Character references", exact: true })
      .click();

    await page
      .locator("#reference-provider-notice")
      .getByRole("link", { name: "Usage policy" })
      .waitFor();
    await page.getByLabel("Reference set ID").fill("patrick");
    await page
      .getByRole("button", { name: "Create reference set", exact: true })
      .click();

    await page
      .getByRole("button", {
        name: "Import selected original into slot",
        exact: true,
      })
      .click();

    await page
      .getByRole("button", { name: "Accept reference", exact: true })
      .click();

    await page.getByText("Accepted · body/front", { exact: true }).waitFor();
    for (const slot of [
      "head/front",
      "body/left",
      "body/right",
      "body/rear",
      "head/left",
      "head/right",
      "head/rear",
    ]) {
      await page.getByLabel("Reference slot").selectOption(slot);
      await page
        .getByRole("button", {
          name: "Import selected original into slot",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", { name: "Accept reference", exact: true })
        .click();
      await page.getByText("Accepted · " + slot, { exact: true }).waitFor();
    }
    await page
      .getByRole("button", {
        name: "Publish approved reference set",
        exact: true,
      })
      .click();
    await page
      .getByRole("img", { name: "v1 front card", exact: true })
      .waitFor();
    await page
      .getByRole("button", { name: "Character library", exact: true })
      .click();
    await page
      .getByText("Create a character from the selected original", {
        exact: true,
      })
      .click();
    const name = "patrick-" + Date.now();
    await page.getByLabel("Character ID", { exact: true }).fill(name);
    await page
      .getByLabel("Character description", { exact: true })
      .fill("An older silver-haired cleric in black and crimson.");
    await page
      .getByRole("button", { name: "Create character package", exact: true })
      .click();
    await page.getByLabel("Published reference version (optional)").fill("v1");
    await page
      .getByRole("button", { name: "Publish to shared library", exact: true })
      .click();
    const card = page
      .locator("#character-list .card")
      .filter({ has: page.getByRole("heading", { name, exact: true }) });
    await card
      .getByRole("button", { name: "Use pinned version", exact: true })
      .click();
    await page.getByText(/Pinned character:/).waitFor();
    await page
      .getByRole("button", { name: "Character library", exact: true })
      .click();
    await page.getByLabel("Package version", { exact: true }).fill("v2");
    await page
      .getByRole("button", { name: "Create character package", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Publish to shared library", exact: true })
      .click();
    await page
      .getByText("Different version available · current v1", { exact: true })
      .waitFor();
    await page
      .getByRole("button", { name: "Back to story", exact: true })
      .click();
    assert.match(
      await page.locator("#character-binding").innerText(),
      /\/v1\//,
    );

    assert.equal(
      await page.locator("#error").isVisible(),
      false,
      await page.locator("#error").innerText(),
    );
  } finally {
    await browser.close();
  }
});

for (const backend of ["files", "postgres"])
  test(`${backend}: candidate approval, provider failure, conflicts and missing media`, async () => {
    const browser = await chromium.launch({
      headless: true,
      channel: process.env.PANELTREE_BROWSER_CHANNEL || undefined,
    });
    try {
      const page = await (
        await browser.newContext({
          viewport: { width: 1440, height: 1000 },
        })
      ).newPage();
      page.setDefaultTimeout(15000);
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await page.goto(base);
      await page.getByLabel(/^Storage/).selectOption(backend);
      await page
        .getByLabel("Project name")
        .fill("render-" + backend + "-" + Date.now());
      await page
        .getByRole("button", { name: "Create project", exact: true })
        .click();
      await page
        .getByRole("img", { name: "Page preview", exact: true })
        .waitFor();
      await page.locator('#outline button[title="hero"]').first().click();
      await page
        .getByRole("button", { name: "Artwork & render", exact: true })
        .click();
      const png = Buffer.from(
        await page.evaluate(() => {
          const c = document.createElement("canvas");
          c.width = 64;
          c.height = 96;
          return c.toDataURL().split(",")[1];
        }),
        "base64",
      );
      await page.getByLabel("Import PNG artwork").setInputFiles({
        name: "source.png",
        mimeType: "image/png",
        buffer: png,
      });
      await page
        .getByRole("img", { name: "Imported artwork", exact: true })
        .waitFor();
      await page
        .getByRole("button", { name: "Set as source image", exact: true })
        .click();

      await page
        .getByRole("button", { name: "Character library", exact: true })
        .click();
      await page
        .getByText("Create a character from the selected original", {
          exact: true,
        })
        .click();
      const characterID = "patrick-" + backend + "-" + Date.now();
      await page.getByLabel("Character ID", { exact: true }).fill(characterID);
      await page
        .getByLabel("Character description", { exact: true })
        .fill("Patrick, silver hair and a crimson cape");
      await page
        .getByRole("button", { name: "Create character package", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Publish to shared library", exact: true })
        .click();
      await page
        .locator("#character-list .card")
        .filter({
          has: page.getByRole("heading", { name: characterID, exact: true }),
        })
        .getByRole("button", { name: "Use pinned version", exact: true })
        .click();
      await page.getByText(/Pinned character:/).waitFor();
      await page
        .getByRole("button", { name: "Artwork & render", exact: true })
        .click();
      await page
        .getByLabel("Renderer / model profile")
        .selectOption({ label: "fixture-edit · ideogram-4-5" });
      await page.getByLabel("Prompt", { exact: true }).fill("Point and laugh");
      const stats = () =>
        page.request.get(base + "/__test/stats").then((r) => r.json());
      const before = await stats();
      const prepared = page.waitForResponse((r) =>
        r.url().endsWith("/prepare"),
      );
      await page
        .getByRole("button", {
          name: "Prepare & request estimate",
          exact: true,
        })
        .click();
      const quote = await (await prepared).json();
      assert.equal(
        (await stats()).posts,
        before.posts,
        "prepare must never submit",
      );
      await page.getByText("Provider estimate", { exact: true }).waitFor();
      await page
        .getByRole("button", {
          name: "Confirm & run this candidate",
          exact: true,
        })
        .click();
      const card = page
        .locator(".job")
        .filter({ hasText: quote.job.id.slice(0, 12) });
      await card
        .getByRole("button", { name: "Select candidate", exact: true })
        .waitFor();
      assert.equal((await stats()).posts, before.posts + 1);
      await card
        .getByRole("img", { name: "Source artwork", exact: true })
        .waitFor();
      await card
        .getByRole("button", { name: "Select candidate", exact: true })
        .click();

      const pin = async () => {
        await page.waitForFunction(
          () => !document.querySelector("#controls").disabled,
        );
        return page.evaluate(async () => {
          const projects = await (await fetch("/api/projects")).json();
          const p = projects.find(
            (p) =>
              p.name === document.querySelector("#story-title").textContent,
          );
          const v = await (await fetch("/api/projects/" + p.id)).json();
          if (!v.layers) throw new Error(JSON.stringify(v));
          return v.layers["page-01/p1/hero"].pin;
        });
      };
      const selectedPin = await pin();
      await page
        .getByRole("button", { name: "Properties", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Send to review", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Approve artwork", exact: true })
        .click();
      await page.getByText("approved", { exact: true }).waitFor();
      assert.equal(
        await pin(),
        selectedPin,
        "approval must preserve the selected candidate",
      );
      await page
        .getByRole("button", { name: "Return to draft", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Artwork & render", exact: true })
        .click();
      await page
        .getByLabel("Prompt", { exact: true })
        .fill("FAIL: fixture provider failure");
      const failResponse = page.waitForResponse((r) =>
        r.url().endsWith("/prepare"),
      );
      await page
        .getByRole("button", {
          name: "Prepare & request estimate",
          exact: true,
        })
        .click();
      const failure = await (await failResponse).json();
      await page
        .getByRole("button", {
          name: "Confirm & run this candidate",
          exact: true,
        })
        .click();
      const failed = page
        .locator(".job")
        .filter({ hasText: failure.job.id.slice(0, 12) });
      await failed.getByText(/ideogram generation failed/).waitFor();
      await page
        .getByRole("button", { name: "Properties", exact: true })
        .click();
      await page.getByLabel("Lock scope").selectOption("all");
      await page
        .getByRole("button", { name: "Lock layer", exact: true })
        .click();
      await page.getByLabel("Layer role").fill("Must not save");
      await page
        .getByRole("button", { name: "Apply properties", exact: true })
        .click();
      await page.locator("#error").waitFor();
      assert.match(await page.locator("#error").innerText(), /lock/i);
      await page
        .getByRole("button", { name: "Dismiss error", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Unlock layer", exact: true })
        .click();
      await page.waitForFunction(
        () => !document.querySelector("#controls").disabled,
      );
      // A separate client changes the project while this browser retains its old revision.
      await page.evaluate(async () => {
        const session = await (await fetch("/api/session")).json();
        const projects = await (await fetch("/api/projects")).json();
        const project = projects.find(
          (p) => p.name === document.querySelector("#story-title").textContent,
        );
        const url = "/api/projects/" + project.id;
        const view = await (await fetch(url)).json();
        if (!view.documents) throw new Error(JSON.stringify(view));
        const doc = view.documents.find(
          (d) => d.document.page?.id === "page-01",
        );
        doc.document.page.panels[0].layers[0].role = "External editor";
        const file = doc.file.replaceAll("\\", "/").match(/pages\/.*$/)[0];
        const r = await fetch(url + "/edit", {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            "X-PanelTree-Token": session.token,
          },
          body: JSON.stringify({
            revision: view.revision,
            edits: [{ file, document: doc.document }],
          }),
        });
        if (!r.ok) throw new Error(await r.text());
      });
      await page.getByLabel("Layer role").fill("Stale browser edit");
      await page
        .getByRole("button", { name: "Apply properties", exact: true })
        .click();
      await page.locator("#error").waitFor();
      assert.match(await page.locator("#error").innerText(), /project changed/);
      await page
        .getByRole("button", { name: "Refresh project", exact: true })
        .click();
      await page.route("**/media?**", (route) =>
        route.fulfill({ status: 404, body: "missing fixture image" }),
      );
      await page
        .getByRole("button", { name: "Artwork & render", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Set as source image", exact: true })
        .click();
      await page.locator("#error").waitFor();
      assert.match(await page.locator("#error").innerText(), /unavailable/i);

      await page.unroute("**/media?**");
      await page
        .getByRole("button", { name: "Dismiss error", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Add panel", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Add lettering", exact: true })
        .click();
      await page
        .getByLabel("Lettering text")
        .fill("Patrick has entered the story.");
      await page
        .getByRole("button", { name: "Apply properties", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Export PNG", exact: true })
        .click();
      await page
        .getByRole("link", { name: "Download page PNG", exact: true })
        .waitFor();
      assert.deepEqual(errors, []);
    } finally {
      await browser.close();
    }
  });

test("cancel and safely resume the same known provider execution", async () => {
  const browser = await chromium.launch({
    headless: true,
    channel: process.env.PANELTREE_BROWSER_CHANNEL || undefined,
  });
  try {
    const page = await (await browser.newContext()).newPage();
    page.setDefaultTimeout(25000);
    await page.goto(base);
    await page.getByLabel("Project name").fill("resume-" + Date.now());
    await page
      .getByRole("button", { name: "Create project", exact: true })
      .click();
    await page
      .getByRole("img", { name: "Page preview", exact: true })
      .waitFor();
    await page.locator('#outline button[title="hero"]').first().click();
    await page
      .getByRole("button", { name: "Artwork & render", exact: true })
      .click();
    const png = Buffer.from(
      await page.evaluate(() => {
        const c = document.createElement("canvas");
        c.width = 64;
        c.height = 96;
        return c.toDataURL().split(",")[1];
      }),
      "base64",
    );
    await page.getByLabel("Import PNG artwork").setInputFiles({
      name: "source.png",
      mimeType: "image/png",
      buffer: png,
    });
    await page
      .getByRole("img", { name: "Imported artwork", exact: true })
      .waitFor();
    await page
      .getByRole("button", { name: "Set as source image", exact: true })
      .click();
    await page
      .getByLabel("Renderer / model profile")
      .selectOption({ label: "fixture-edit · ideogram-4-5" });
    await page.getByLabel("Prompt", { exact: true }).fill("SLOW: change pose");
    const prepared = page.waitForResponse((r) => r.url().endsWith("/prepare"));
    await page
      .getByRole("button", { name: "Prepare & request estimate", exact: true })
      .click();
    const q = await (await prepared).json();
    await page
      .getByRole("button", {
        name: "Confirm & run this candidate",
        exact: true,
      })
      .click();
    const card = page
      .locator(".job")
      .filter({ hasText: q.job.id.slice(0, 12) });
    await card.getByText(/running/).waitFor();
    await card.getByRole("button", { name: "Cancel", exact: true }).click();
    await card
      .getByRole("button", { name: "Resume known execution", exact: true })
      .waitFor();
    const count = (
      await (await page.request.get(base + "/__test/stats")).json()
    ).posts;
    await card
      .getByRole("button", { name: "Resume known execution", exact: true })
      .click();
    await page
      .getByRole("button", {
        name: "Confirm & run this candidate",
        exact: true,
      })
      .click();
    await card
      .getByRole("button", { name: "Select candidate", exact: true })
      .waitFor();
    assert.equal(
      (await (await page.request.get(base + "/__test/stats")).json()).posts,
      count,
      "resume must not submit again",
    );
    await card
      .getByRole("button", { name: "Reject candidate", exact: true })
      .click();
    await card
      .getByText("Rejected · original retained", { exact: true })
      .waitFor();
    assert.equal(
      await card
        .getByRole("button", { name: "Select candidate", exact: true })
        .count(),
      0,
    );
  } finally {
    await browser.close();
  }
});
test("keyboard navigation and a narrow viewport remain usable", async () => {
  const browser = await chromium.launch({
    headless: true,
    channel: process.env.PANELTREE_BROWSER_CHANNEL || undefined,
  });
  try {
    const page = await (
      await browser.newContext({ viewport: { width: 720, height: 900 } })
    ).newPage();
    await page.goto(base);
    await page.getByRole("heading", { name: "Your projects" }).waitFor();
    await page.keyboard.press("Tab");
    assert.equal(
      await page.evaluate(() => document.activeElement.className),
      "brand",
    );
    await page.getByLabel("Project name").focus();
    await page.keyboard.type("keyboard-" + Date.now());
    await page
      .getByRole("button", { name: "Create project", exact: true })
      .focus();
    await page.keyboard.press("Enter");
    await page
      .getByRole("img", { name: "Page preview", exact: true })
      .waitFor();
    await page.waitForFunction(
      () => !document.querySelector("#controls").disabled,
    );
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
      true,
    );
    const fits = await page.locator("#add-panel").evaluate((button) => {
      const b = button.getBoundingClientRect();
      const a = button.closest("aside").getBoundingClientRect();
      return b.left >= a.left && b.right <= a.right;
    });
    assert.equal(fits, true, "Add panel must fit inside the outline");
    const result = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa"])
      .analyze();
    assert.deepEqual(
      result.violations.map((v) => v.id),
      [],
    );
    await page.screenshot({
      path: path.join(path.dirname(screenshot), "workspace-narrow.png"),
      fullPage: true,
    });
  } finally {
    await browser.close();
  }
});

test("composition works on an existing page without text or fonts", async () => {
  const browser = await chromium.launch({
    headless: true,
    channel: process.env.PANELTREE_BROWSER_CHANNEL || undefined,
  });
  try {
    const page = await browser.newPage();
    page.setDefaultTimeout(8000);
    await page.goto(base);
    const name = "fontless-" + Date.now();
    await page.getByLabel("Project name").fill(name);
    await page
      .getByRole("button", { name: "Create project", exact: true })
      .click();
    await page
      .getByRole("img", { name: "Page preview", exact: true })
      .waitFor();
    await page.waitForFunction(
      () => !document.querySelector("#controls").disabled,
    );
    await page.evaluate(async () => {
      const session = await (await fetch("/api/session")).json();
      const ps = await (await fetch("/api/projects")).json();
      const p = ps.find(
        (p) => p.name === document.querySelector("#story-title").textContent,
      );
      const url = "/api/projects/" + p.id;
      const v = await (await fetch(url)).json();
      const edits = [];
      for (const d of v.documents.filter((d) => d.document.page)) {
        for (const p of d.document.page.panels)
          p.layers = [
            {
              id: "image",
              source: { kind: "image", path: "../assets/hero.png" },
            },
          ];
        edits.push({
          file: d.file.replaceAll("\\", "/").match(/pages\/.*$/)[0],
          document: d.document,
        });
      }
      const r = await fetch(url + "/edit", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-PanelTree-Token": session.token,
        },
        body: JSON.stringify({ revision: v.revision, edits }),
      });
      if (!r.ok) throw new Error(await r.text());
    });
    await page.reload();
    await page.locator(".project-card").filter({ hasText: name }).click();
    await page
      .getByRole("img", { name: "Page preview", exact: true })
      .waitFor();
    for (const name of ["Add panel", "Add artwork layer", "Add lettering"]) {
      const response = page.waitForResponse((r) => r.url().endsWith("/edit"));
      await page.getByRole("button", { name, exact: true }).click();
      const r = await response;
      assert.equal(r.status(), 200, await r.text());
      await page.waitForFunction(
        () => !document.querySelector("#controls").disabled,
      );
    }
    assert.equal(await page.locator("#error").isVisible(), false);
  } finally {
    await browser.close();
  }
});

test("background job polling leaves editing controls enabled", async () => {
  const browser = await chromium.launch({
    headless: true,
    channel: process.env.PANELTREE_BROWSER_CHANNEL || undefined,
  });
  let release;
  try {
    const page = await browser.newPage();
    await page.goto(base);
    await page.getByLabel("Project name").fill("poll-" + Date.now());
    await page
      .getByRole("button", { name: "Create project", exact: true })
      .click();
    await page
      .getByRole("img", { name: "Page preview", exact: true })
      .waitFor();
    await page.waitForFunction(
      () => !document.querySelector("#controls").disabled,
    );
    let arrived;
    const polling = new Promise((r) => (arrived = r));
    const gate = new Promise((r) => (release = r));
    await page.route("**/jobs", async (route) => {
      arrived();
      await gate;
      await route.continue();
    });
    await polling;
    assert.equal(
      await page.getByRole("button", { name: "Undo", exact: true }).isEnabled(),
      true,
      "poll must not interrupt an edit click",
    );
  } finally {
    release?.();
    await browser.close();
  }
});
