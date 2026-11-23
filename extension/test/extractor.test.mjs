import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);

const extractor = require("../bulk-extractor.js");
const runner = require("../bulk-runner.js");

function parseAttributes(raw) {
  const attrs = Object.create(null);
  const attrRe = /([^\s=/>]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'>]+)))?/g;
  let match;
  while ((match = attrRe.exec(raw)) !== null) {
    const key = match[1].toLowerCase();
    const val = match[2] ?? match[3] ?? match[4] ?? "";
    attrs[key] = val;
  }
  return attrs;
}

class MiniElement {
  constructor(tagName, attrs, parent) {
    this.tagName = tagName.toUpperCase();
    this.attrs = attrs || Object.create(null);
    this.parentElement = parent || null;
    this.children = [];
    this.textParts = [];
  }

  getAttribute(name) {
    const key = String(name || "").toLowerCase();
    return Object.prototype.hasOwnProperty.call(this.attrs, key) ? this.attrs[key] : null;
  }

  get textContent() {
    let out = this.textParts.join("");
    for (const child of this.children) {
      out += child.textContent;
    }
    return out;
  }

  matchesSingle(selector) {
    const sel = selector.trim();
    if (!sel) {
      return false;
    }

    const attrContainsMatch = /^\[([a-z0-9_-]+)\*="([^"]+)"\]$/i.exec(sel);
    if (attrContainsMatch) {
      const val = this.getAttribute(attrContainsMatch[1]);
      return val !== null && val.includes(attrContainsMatch[2]);
    }

    const attrExactMatch = /^\[([a-z0-9_-]+)="([^"]+)"\]$/i.exec(sel);
    if (attrExactMatch) {
      return this.getAttribute(attrExactMatch[1]) === attrExactMatch[2];
    }

    const tagAttrMatch = /^([a-z0-9]+)\[([a-z0-9_-]+)\]$/i.exec(sel);
    if (tagAttrMatch) {
      return (
        this.tagName === tagAttrMatch[1].toUpperCase() &&
        this.getAttribute(tagAttrMatch[2]) !== null
      );
    }

    return this.tagName === sel.toUpperCase();
  }

  matches(selectorList) {
    const parts = String(selectorList || "").split(",");
    return parts.some((part) => this.matchesSingle(part));
  }

  closest(selectorList) {
    let cur = this;
    while (cur) {
      if (cur.matches(selectorList)) {
        return cur;
      }
      cur = cur.parentElement;
    }
    return null;
  }

  querySelectorAll(selectorList) {
    const results = [];
    const visit = (node) => {
      for (const child of node.children) {
        if (child.matches(selectorList)) {
          results.push(child);
        }
        visit(child);
      }
    };
    visit(this);
    return results;
  }

  querySelector(selectorList) {
    return this.querySelectorAll(selectorList)[0] || null;
  }
}

function parseHtml(html) {
  const root = new MiniElement("div", {}, null);
  const stack = [root];
  const tokenRe = /<!--[\s\S]*?-->|<\/([a-zA-Z0-9]+)\s*>|<([a-zA-Z0-9]+)([^>]*)>|([^<]+)/g;
  const voidTags = new Set(["br", "hr", "img", "input", "meta", "link"]);
  let match;

  while ((match = tokenRe.exec(html)) !== null) {
    if (match[1]) {
      const closeTag = match[1].toUpperCase();
      while (stack.length > 1) {
        const popped = stack.pop();
        if (popped.tagName === closeTag) {
          break;
        }
      }
    } else if (match[2]) {
      const tagName = match[2];
      const rawAttrs = match[3] || "";
      const selfClosing = rawAttrs.trim().endsWith("/") || voidTags.has(tagName.toLowerCase());
      const parent = stack[stack.length - 1];
      const el = new MiniElement(tagName, parseAttributes(rawAttrs), parent);
      parent.children.push(el);
      if (!selfClosing) {
        stack.push(el);
      }
    } else if (match[4]) {
      const parent = stack[stack.length - 1];
      parent.textParts.push(match[4]);
    }
  }

  return root;
}

test("detectPage matches supported X and Threads routes only", () => {
  assert.deepEqual(extractor.detectPage("https://x.com/i/bookmarks"), {
    platform: "x",
    pageKind: "bookmarks",
    label: "X Bookmarks",
    source: "extension-bulk-x"
  });
  assert.deepEqual(extractor.detectPage("https://twitter.com/aditya_hima/likes/"), {
    platform: "x",
    pageKind: "likes",
    label: "X Likes",
    source: "extension-bulk-x"
  });
  assert.deepEqual(extractor.detectPage("https://www.threads.com/saved"), {
    platform: "threads",
    pageKind: "saved",
    label: "Threads Saved",
    source: "extension-bulk-threads"
  });
  assert.deepEqual(extractor.detectPage("https://threads.net/liked"), {
    platform: "threads",
    pageKind: "likes",
    label: "Threads Likes",
    source: "extension-bulk-threads"
  });
  assert.deepEqual(extractor.detectPage("https://www.threads.com/@aditya/saved/"), {
    platform: "threads",
    pageKind: "saved",
    label: "Threads Saved",
    source: "extension-bulk-threads"
  });

  // Unsupported pages must return null
  assert.equal(extractor.detectPage("https://x.com/home"), null);
  assert.equal(extractor.detectPage("https://x.com/explore/likes"), null);
  assert.equal(extractor.detectPage("https://x.com/alice/status/123"), null);
  assert.equal(extractor.detectPage("https://www.threads.com/"), null);
  assert.equal(extractor.detectPage("https://www.threads.com/@aditya"), null);
  assert.equal(extractor.detectPage("https://www.threads.com/@aditya/post/DD123"), null);
  assert.equal(extractor.detectPage("https://example.com/i/bookmarks"), null);
});

test("extractXPostUrls extracts canonical deduplicated URLs and skips deleted tombstones", () => {
  const html = fs.readFileSync(path.join(__dirname, "fixtures", "x-bookmarks.html"), "utf8");
  const doc = parseHtml(html);

  const urls = extractor.extractXPostUrls(doc, "https://x.com");
  assert.deepEqual(urls, [
    "https://x.com/alice_dev/status/1850000000000000001",
    "https://x.com/bob_ai/status/1850000000000000002",
    "https://x.com/carol_go/status/1850000000000000003",
    "https://x.com/i/web/status/1850000000000000004"
  ]);
});

test("extractThreadsPostUrls extracts and deduplicates Threads post URLs without class reliance", () => {
  const html = fs.readFileSync(path.join(__dirname, "fixtures", "threads-saved.html"), "utf8");
  const doc = parseHtml(html);

  const urls = extractor.extractThreadsPostUrls(doc, "https://www.threads.com");
  assert.deepEqual(urls, [
    "https://www.threads.com/@aditya.dev/post/DD12345abc_",
    "https://www.threads.com/@zuck/post/EE98765xyz",
    "https://www.threads.com/post/FF55555mno",
    "https://www.threads.net/@mosseri/post/GG11111pqr"
  ]);
});

test("extractors handle empty or malformed DOM safely", () => {
  assert.deepEqual(extractor.extractXPostUrls(null), []);
  assert.deepEqual(extractor.extractThreadsPostUrls(null), []);
  assert.deepEqual(extractor.extractPostUrls("unknown", {}), []);
  assert.deepEqual(extractor.extractThreadsPostUrls(parseHtml("<div>no links</div>")), []);
});

test("createBulkSession collects across virtualized scroll, handles 409 duplicates, and continues past transient errors", async () => {
  const batches = [
    ["https://x.com/u/status/1", "https://x.com/u/status/2"],
    ["https://x.com/u/status/2", "https://x.com/u/status/3"],
    ["https://x.com/u/status/3"],
    ["https://x.com/u/status/3"]
  ];
  let round = 0;
  let scrollY = 0;
  const savedCalls = [];

  const session = runner.createBulkSession({
    maxPosts: 200,
    saveDelayMs: 0,
    scrollWaitMs: 0,
    maxStagnantRounds: 2,
    sleep: () => Promise.resolve(),
    extractUrls: () => batches[Math.min(round, batches.length - 1)],
    scrollStep: () => {
      round++;
      if (round === 1) {
        scrollY += 800;
      }
    },
    getScrollMetrics: () => ({ scrollY, scrollHeight: 2000 }),
    saveUrl: async (url) => {
      savedCalls.push(url);
      if (url.endsWith("/1")) {
        return { ok: true, status: 201 };
      }
      if (url.endsWith("/2")) {
        return { ok: false, code: "DUPLICATE", status: 409 };
      }
      return { ok: false, code: "HTTP_ERROR", status: 500, error: "Temporary failure" };
    }
  });

  const result = await session.start({ autoScroll: true });
  assert.equal(result.phase, "done");
  assert.equal(result.total, 3);
  assert.equal(result.processed, 3);
  assert.equal(result.saved, 1);
  assert.equal(result.skipped, 1);
  assert.equal(result.failed, 1);
  assert.deepEqual(savedCalls, [
    "https://x.com/u/status/1",
    "https://x.com/u/status/2",
    "https://x.com/u/status/3"
  ]);
});

test("createBulkSession enforces 200-post cap and supports Stop", async () => {
  const largeBatch = [];
  for (let i = 1; i <= 250; i++) {
    largeBatch.push("https://x.com/u/status/" + i);
  }

  let calls = 0;
  const session = runner.createBulkSession({
    maxPosts: 200,
    saveDelayMs: 0,
    scrollWaitMs: 0,
    sleep: () => Promise.resolve(),
    extractUrls: () => largeBatch,
    saveUrl: async () => {
      calls++;
      if (calls === 5) {
        session.stop();
      }
      return { ok: true, status: 201 };
    }
  });

  const result = await session.start({ autoScroll: true });
  assert.equal(result.phase, "stopped");
  assert.equal(result.total, 200);
  assert.equal(result.limitReached, true);
  assert.equal(result.processed, 5);
  assert.equal(result.saved, 5);
});

test("createBulkSession stops immediately on fatal auth error", async () => {
  let calls = 0;
  const session = runner.createBulkSession({
    saveDelayMs: 0,
    scrollWaitMs: 0,
    sleep: () => Promise.resolve(),
    extractUrls: () => ["https://x.com/u/status/1", "https://x.com/u/status/2"],
    saveUrl: async () => {
      calls++;
      return { ok: false, code: "UNAUTHORIZED", status: 401, error: "Invalid API key" };
    }
  });

  const result = await session.start({ autoScroll: false });
  assert.equal(result.phase, "error");
  assert.equal(calls, 1);
  assert.equal(result.failed, 1);
  assert.equal(result.fatalError.code, "UNAUTHORIZED");
});
