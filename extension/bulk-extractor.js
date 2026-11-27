(function (root) {
  "use strict";

  var MAX_POSTS_PER_SESSION = 200;

  var X_HOSTS = {
    "x.com": true,
    "www.x.com": true,
    "mobile.x.com": true,
    "twitter.com": true,
    "www.twitter.com": true,
    "mobile.twitter.com": true
  };

  var X_RESERVED_HANDLES = {
    i: true,
    home: true,
    explore: true,
    notifications: true,
    messages: true,
    settings: true,
    search: true,
    compose: true,
    intent: true,
    tos: true,
    privacy: true,
    login: true,
    signup: true,
    hashtag: true
  };

  var THREADS_HOSTS = {
    "threads.com": true,
    "www.threads.com": true,
    "threads.net": true,
    "www.threads.net": true
  };

  var THREADS_RESERVED_SEGMENTS = {
    login: true,
    search: true,
    activity: true,
    settings: true,
    api: true,
    t: true,
    post: true,
    privacy: true,
    terms: true,
    saved: true,
    liked: true,
    likes: true
  };

  var X_UNAVAILABLE_PATTERNS = [
    /\bthis (?:post|tweet) (?:was deleted|is unavailable|is from a suspended|is from an account that no longer exists|violated)\b/i,
    /\bunable to view this (?:post|tweet)\b/i,
    /\bpost (?:was deleted|is unavailable)\b/i
  ];

  function parseLocationInput(urlOrLoc) {
    if (!urlOrLoc) {
      return null;
    }

    if (typeof urlOrLoc === "string") {
      try {
        var parsed = new URL(urlOrLoc);
        return {
          hostname: (parsed.hostname || "").toLowerCase(),
          pathname: parsed.pathname || "/"
        };
      } catch (_err) {
        return null;
      }
    }

    if (typeof urlOrLoc === "object") {
      var hostname = String(urlOrLoc.hostname || "").trim().toLowerCase();
      var pathname = String(urlOrLoc.pathname || "/");
      if (!hostname && typeof urlOrLoc.href === "string") {
        return parseLocationInput(urlOrLoc.href);
      }
      if (!hostname) {
        return null;
      }
      return {
        hostname: hostname,
        pathname: pathname
      };
    }

    return null;
  }

  function detectPage(urlOrLoc) {
    var loc = parseLocationInput(urlOrLoc);
    if (!loc) {
      return null;
    }

    var hostname = loc.hostname;
    var pathname = loc.pathname || "/";
    var cleanPath = pathname.replace(/\/+$/, "") || "/";

    if (X_HOSTS[hostname]) {
      if (cleanPath === "/i/bookmarks" || cleanPath.indexOf("/i/bookmarks/") === 0) {
        return {
          platform: "x",
          pageKind: "bookmarks",
          label: "X Bookmarks",
          source: "extension-bulk-x"
        };
      }

      if (cleanPath === "/i/history/likes") {
        return {
          platform: "x",
          pageKind: "likes",
          label: "X Likes",
          source: "extension-bulk-x"
        };
      }

      // /i/history itself renders the Bookmarks tab by default (the Likes
      // tab is a link to /i/history/likes; Bookmarks is a tab control with
      // no href). Without this the panel never mounts on the History page.
      if (cleanPath === "/i/history") {
        return {
          platform: "x",
          pageKind: "bookmarks",
          label: "X Bookmarks",
          source: "extension-bulk-x"
        };
      }

      if (cleanPath === "/i/history/bookmarks") {
        return {
          platform: "x",
          pageKind: "bookmarks",
          label: "X Bookmarks",
          source: "extension-bulk-x"
        };
      }

      var xLikesMatch = /^\/([A-Za-z0-9_]{1,30})\/likes$/.exec(cleanPath);
      if (xLikesMatch) {
        var handle = xLikesMatch[1];
        if (!X_RESERVED_HANDLES[handle.toLowerCase()]) {
          return {
            platform: "x",
            pageKind: "likes",
            label: "X Likes",
            source: "extension-bulk-x"
          };
        }
      }

      return null;
    }

    if (THREADS_HOSTS[hostname]) {
      if (cleanPath === "/saved") {
        return {
          platform: "threads",
          pageKind: "saved",
          label: "Threads Saved",
          source: "extension-bulk-threads"
        };
      }

      if (cleanPath === "/liked" || cleanPath === "/likes") {
        return {
          platform: "threads",
          pageKind: "likes",
          label: "Threads Likes",
          source: "extension-bulk-threads"
        };
      }

      var threadsUserMatch = /^\/@?([A-Za-z0-9._]{1,40})\/(saved|liked|likes)$/.exec(cleanPath);
      if (threadsUserMatch) {
        var segment = threadsUserMatch[1].toLowerCase();
        var section = threadsUserMatch[2];
        if (!THREADS_RESERVED_SEGMENTS[segment]) {
          var isSaved = section === "saved";
          return {
            platform: "threads",
            pageKind: isSaved ? "saved" : "likes",
            label: isSaved ? "Threads Saved" : "Threads Likes",
            source: "extension-bulk-threads"
          };
        }
      }

      return null;
    }

    return null;
  }

  function safeQueryAll(el, selector) {
    if (!el || typeof el.querySelectorAll !== "function") {
      return [];
    }
    try {
      var list = el.querySelectorAll(selector);
      return list ? Array.prototype.slice.call(list) : [];
    } catch (_err) {
      return [];
    }
  }

  function safeQuery(el, selector) {
    if (!el || typeof el.querySelector !== "function") {
      return null;
    }
    try {
      return el.querySelector(selector);
    } catch (_err) {
      return null;
    }
  }

  function safeClosest(el, selector) {
    if (!el || typeof el.closest !== "function") {
      return null;
    }
    try {
      return el.closest(selector);
    } catch (_err) {
      return null;
    }
  }

  function safeGetAttr(el, name) {
    if (!el || typeof el.getAttribute !== "function") {
      return "";
    }
    var val = el.getAttribute(name);
    return val === null || val === undefined ? "" : String(val);
  }

  function normalizeMinDocumentTop(options) {
    if (!options || typeof options !== "object") {
      return 0;
    }
    if (typeof options.minDocumentTop === "number" && isFinite(options.minDocumentTop) && options.minDocumentTop > 0) {
      return options.minDocumentTop;
    }
    return 0;
  }

  function resolveScrollY(rootEl, options) {
    if (options && typeof options === "object" && typeof options.scrollY === "number" && isFinite(options.scrollY)) {
      return options.scrollY;
    }
    var docView = rootEl && rootEl.defaultView;
    if (docView) {
      if (typeof docView.scrollY === "number" && isFinite(docView.scrollY)) {
        return docView.scrollY;
      }
      if (typeof docView.pageYOffset === "number" && isFinite(docView.pageYOffset)) {
        return docView.pageYOffset;
      }
    }
    if (typeof window !== "undefined" && window) {
      if (typeof window.scrollY === "number" && isFinite(window.scrollY)) {
        return window.scrollY;
      }
      if (typeof window.pageYOffset === "number" && isFinite(window.pageYOffset)) {
        return window.pageYOffset;
      }
    }
    if (root && typeof root.scrollY === "number" && isFinite(root.scrollY)) {
      return root.scrollY;
    }
    return null;
  }

  function getElementRectTop(el) {
    if (!el || typeof el.getBoundingClientRect !== "function") {
      return null;
    }
    var rect;
    try {
      rect = el.getBoundingClientRect();
    } catch (_err) {
      return null;
    }
    if (rect && typeof rect.top === "number" && isFinite(rect.top)) {
      return rect.top;
    }
    return null;
  }

  function isAboveMinDocumentTop(el, minDocumentTop, currentScrollY) {
    if (!minDocumentTop || minDocumentTop <= 0 || currentScrollY === null) {
      return false;
    }
    var rectTop = getElementRectTop(el);
    if (rectTop === null) {
      return false;
    }
    return rectTop + currentScrollY < minDocumentTop;
  }

  function isInsideQuoteOrNestedArticle(node, rootArticle) {
    var quote = safeClosest(node, '[role="blockquote"]');
    if (quote && quote !== rootArticle) {
      return true;
    }
    var nearestArticle = safeClosest(node, "article");
    if (nearestArticle && nearestArticle !== rootArticle) {
      return true;
    }
    return false;
  }

  function parseXStatusUrl(rawHref, baseOrigin) {
    if (!rawHref) {
      return null;
    }
    var href = String(rawHref).trim();
    if (!href || href.indexOf("javascript:") === 0 || href.indexOf("#") === 0) {
      return null;
    }

    var parsed;
    try {
      parsed = new URL(href, baseOrigin || "https://x.com");
    } catch (_err) {
      return null;
    }

    var host = (parsed.hostname || "").toLowerCase();
    if (!X_HOSTS[host]) {
      return null;
    }

    var path = parsed.pathname || "";
    var primaryMatch = /^\/(?:(i\/web)|([A-Za-z0-9_]{1,30}))\/status\/(\d+)\/?$/.exec(path);
    if (primaryMatch) {
      var handle = primaryMatch[2] || "";
      var id = primaryMatch[3];
      if (handle && X_RESERVED_HANDLES[handle.toLowerCase()]) {
        return null;
      }
      return {
        id: id,
        handle: handle,
        hasHandle: Boolean(handle),
        isExactPermalink: true,
        canonicalUrl: handle
          ? "https://x.com/" + handle + "/status/" + id
          : "https://x.com/i/web/status/" + id
      };
    }

    var mediaMatch = /^\/(?:(i\/web)|([A-Za-z0-9_]{1,30}))\/status\/(\d+)(?:\/(?:photo|video)\/\d+)?\/?$/.exec(path);
    if (mediaMatch) {
      var mediaHandle = mediaMatch[2] || "";
      var mediaId = mediaMatch[3];
      if (mediaHandle && X_RESERVED_HANDLES[mediaHandle.toLowerCase()]) {
        return null;
      }
      return {
        id: mediaId,
        handle: mediaHandle,
        hasHandle: Boolean(mediaHandle),
        isExactPermalink: false,
        canonicalUrl: mediaHandle
          ? "https://x.com/" + mediaHandle + "/status/" + mediaId
          : "https://x.com/i/web/status/" + mediaId
      };
    }

    return null;
  }

  function hasNonQuotedTombstone(article) {
    if (
      safeGetAttr(article, "data-unavailable") === "true" ||
      safeGetAttr(article, "data-deleted") === "true" ||
      safeGetAttr(article, "data-testid").indexOf("tombstone") !== -1
    ) {
      return true;
    }

    var markers = safeQueryAll(article, '[data-testid*="tombstone"], [data-unavailable="true"], [data-deleted="true"]');
    for (var i = 0; i < markers.length; i++) {
      if (!isInsideQuoteOrNestedArticle(markers[i], article)) {
        return true;
      }
    }
    return false;
  }

  function hasNonQuotedTweetBody(article) {
    var bodies = safeQueryAll(article, '[data-testid="tweetText"]');
    for (var i = 0; i < bodies.length; i++) {
      if (!isInsideQuoteOrNestedArticle(bodies[i], article)) {
        return true;
      }
    }
    return false;
  }

  function findXArticlePermalink(article, baseOrigin) {
    var anchors = safeQueryAll(article, "a[href]");
    var fallback = null;

    for (var i = 0; i < anchors.length; i++) {
      var anchor = anchors[i];
      if (isInsideQuoteOrNestedArticle(anchor, article)) {
        continue;
      }

      var info = parseXStatusUrl(safeGetAttr(anchor, "href"), baseOrigin);
      if (!info) {
        continue;
      }

      if (info.isExactPermalink && safeQuery(anchor, "time")) {
        return info;
      }

      if (!fallback) {
        fallback = info;
      } else if (!fallback.hasHandle && info.hasHandle) {
        fallback = info;
      }
    }

    return fallback;
  }

  function isUnavailableXArticle(article, hasPrimaryTimePermalink) {
    var hasBody = hasNonQuotedTweetBody(article);
    var hasTombstone = hasNonQuotedTombstone(article);

    if (hasTombstone && !(hasPrimaryTimePermalink && hasBody)) {
      return true;
    }

    if (!hasBody) {
      var text = String(article.textContent || "").replace(/\s+/g, " ").trim();
      for (var i = 0; i < X_UNAVAILABLE_PATTERNS.length; i++) {
        if (X_UNAVAILABLE_PATTERNS[i].test(text)) {
          return true;
        }
      }
    }

    return false;
  }

  function extractXPostUrls(rootEl, baseOrigin, options) {
    if (!rootEl) {
      return [];
    }

    var origin = baseOrigin || "https://x.com";
    var minDocumentTop = normalizeMinDocumentTop(options);
    var currentScrollY = minDocumentTop > 0 ? resolveScrollY(rootEl, options) : null;
    var articles = safeQueryAll(rootEl, "article");
    var orderedIds = [];
    var byId = Object.create(null);

    for (var i = 0; i < articles.length; i++) {
      var article = articles[i];
      if (article.parentElement && safeClosest(article.parentElement, "article")) {
        continue;
      }

      if (isAboveMinDocumentTop(article, minDocumentTop, currentScrollY)) {
        continue;
      }

      var anchors = safeQueryAll(article, "a[href]");
      var hasTimePermalink = false;
      for (var j = 0; j < anchors.length; j++) {
        var anchor = anchors[j];
        if (isInsideQuoteOrNestedArticle(anchor, article)) {
          continue;
        }
        var timeCheck = parseXStatusUrl(safeGetAttr(anchor, "href"), origin);
        if (timeCheck && timeCheck.isExactPermalink && safeQuery(anchor, "time")) {
          hasTimePermalink = true;
          break;
        }
      }

      if (isUnavailableXArticle(article, hasTimePermalink)) {
        continue;
      }

      var found = findXArticlePermalink(article, origin);
      if (!found) {
        continue;
      }

      if (!byId[found.id]) {
        byId[found.id] = found;
        orderedIds.push(found.id);
      } else if (!byId[found.id].hasHandle && found.hasHandle) {
        byId[found.id] = found;
      }
    }

    var out = [];
    for (var k = 0; k < orderedIds.length; k++) {
      out.push(byId[orderedIds[k]].canonicalUrl);
    }
    return out;
  }

  function normalizeThreadsHost(hostname) {
    if (hostname === "threads.net" || hostname === "www.threads.net") {
      return "www.threads.net";
    }
    return "www.threads.com";
  }

  function parseThreadsPostUrl(rawHref, baseOrigin) {
    if (!rawHref) {
      return null;
    }
    var href = String(rawHref).trim();
    if (!href || href.indexOf("javascript:") === 0 || href.indexOf("mailto:") === 0 || href.indexOf("#") === 0) {
      return null;
    }

    var parsed;
    try {
      parsed = new URL(href, baseOrigin || "https://www.threads.com");
    } catch (_err) {
      return null;
    }

    if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
      return null;
    }

    var host = (parsed.hostname || "").toLowerCase();
    if (!THREADS_HOSTS[host]) {
      return null;
    }

    var normalizedHost = normalizeThreadsHost(host);
    var path = parsed.pathname || "";

    var atPostMatch = /^\/@([A-Za-z0-9._]{1,40})\/post\/([A-Za-z0-9_-]+)(?:\/(?:media)?)?\/?$/.exec(path);
    if (atPostMatch) {
      var atUser = atPostMatch[1];
      var atId = atPostMatch[2];
      return {
        id: atId,
        user: atUser,
        hasUser: true,
        canonicalUrl: "https://" + normalizedHost + "/@" + atUser + "/post/" + atId
      };
    }

    var bareUserPostMatch = /^\/([A-Za-z0-9._]{1,40})\/post\/([A-Za-z0-9_-]+)(?:\/(?:media)?)?\/?$/.exec(path);
    if (bareUserPostMatch) {
      var bareUser = bareUserPostMatch[1];
      var bareId = bareUserPostMatch[2];
      if (!THREADS_RESERVED_SEGMENTS[bareUser.toLowerCase()]) {
        return {
          id: bareId,
          user: bareUser,
          hasUser: true,
          canonicalUrl: "https://" + normalizedHost + "/@" + bareUser + "/post/" + bareId
        };
      }
    }

    var tMatch = /^\/t\/([A-Za-z0-9_-]+)\/?$/.exec(path);
    if (tMatch) {
      return {
        id: tMatch[1],
        hasUser: false,
        canonicalUrl: "https://" + normalizedHost + "/t/" + tMatch[1]
      };
    }

    var postMatch = /^\/post\/([A-Za-z0-9_-]+)\/?$/.exec(path);
    if (postMatch) {
      return {
        id: postMatch[1],
        hasUser: false,
        canonicalUrl: "https://" + normalizedHost + "/post/" + postMatch[1]
      };
    }

    return null;
  }

  function extractThreadsPostUrls(rootEl, baseOrigin, options) {
    if (!rootEl) {
      return [];
    }

    var origin = baseOrigin || "https://www.threads.com";
    var minDocumentTop = normalizeMinDocumentTop(options);
    var currentScrollY = minDocumentTop > 0 ? resolveScrollY(rootEl, options) : null;
    var anchors = safeQueryAll(rootEl, "a[href]");
    var orderedIds = [];
    var byId = Object.create(null);

    for (var i = 0; i < anchors.length; i++) {
      var anchor = anchors[i];
      if (safeGetAttr(anchor, "aria-disabled") === "true") {
        continue;
      }
      if (safeClosest(anchor, '[data-unavailable="true"], [data-deleted="true"]')) {
        continue;
      }

      if (minDocumentTop > 0 && currentScrollY !== null) {
        var positionTarget = getElementRectTop(anchor) !== null ? anchor : safeClosest(anchor, "article");
        if (isAboveMinDocumentTop(positionTarget, minDocumentTop, currentScrollY)) {
          continue;
        }
      }

      var info = parseThreadsPostUrl(safeGetAttr(anchor, "href"), origin);
      if (!info) {
        continue;
      }

      if (!byId[info.id]) {
        byId[info.id] = info;
        orderedIds.push(info.id);
      } else if (!byId[info.id].hasUser && info.hasUser) {
        byId[info.id] = info;
      }
    }

    var out = [];
    for (var k = 0; k < orderedIds.length; k++) {
      out.push(byId[orderedIds[k]].canonicalUrl);
    }
    return out;
  }

  function extractXArticleDescription(article) {
    var bodies = safeQueryAll(article, '[data-testid="tweetText"]');
    var parts = [];
    for (var i = 0; i < bodies.length; i++) {
      if (isInsideQuoteOrNestedArticle(bodies[i], article)) {
        continue;
      }
      var txt = String(bodies[i].textContent || "").replace(/\s+/g, " ").trim();
      if (txt) {
        parts.push(txt);
      }
    }
    return parts.join(" ").trim();
  }

  function extractThreadsAnchorDescription(anchor, userHandle) {
    var container = safeClosest(anchor, "article");
    if (!container) {
      container = anchor.parentElement && anchor.parentElement.parentElement
        ? anchor.parentElement.parentElement
        : anchor.parentElement;
    }
    if (!container) {
      return "";
    }
    var candidates = safeQueryAll(container, '[dir="auto"]');
    var best = "";
    for (var i = 0; i < candidates.length; i++) {
      var txt = String(candidates[i].textContent || "").replace(/\s+/g, " ").trim();
      if (!txt || txt.length < 12) {
        continue;
      }
      if (userHandle && (txt === userHandle || txt === "@" + userHandle)) {
        continue;
      }
      if (/^\d+[smhdw]$/.test(txt)) {
        continue;
      }
      if (txt.length > best.length) {
        best = txt;
      }
    }
    return best;
  }

  function extractPostMetadataMap(platform, rootEl, baseOrigin, options) {
    var map = Object.create(null);
    if (!rootEl) {
      return map;
    }
    var minDocumentTop = normalizeMinDocumentTop(options);
    var currentScrollY = minDocumentTop > 0 ? resolveScrollY(rootEl, options) : null;

    if (platform === "x") {
      var origin = baseOrigin || "https://x.com";
      var articles = safeQueryAll(rootEl, "article");
      for (var i = 0; i < articles.length; i++) {
        var article = articles[i];
        if (article.parentElement && safeClosest(article.parentElement, "article")) {
          continue;
        }
        if (isAboveMinDocumentTop(article, minDocumentTop, currentScrollY)) {
          continue;
        }
        var xAnchors = safeQueryAll(article, "a[href]");
        var hasTimePermalink = false;
        for (var j = 0; j < xAnchors.length; j++) {
          var xAnchor = xAnchors[j];
          if (isInsideQuoteOrNestedArticle(xAnchor, article)) {
            continue;
          }
          var timeCheck = parseXStatusUrl(safeGetAttr(xAnchor, "href"), origin);
          if (timeCheck && timeCheck.isExactPermalink && safeQuery(xAnchor, "time")) {
            hasTimePermalink = true;
            break;
          }
        }
        if (isUnavailableXArticle(article, hasTimePermalink)) {
          continue;
        }
        var found = findXArticlePermalink(article, origin);
        if (!found) {
          continue;
        }
        var title = found.handle ? "@" + found.handle + " on X" : "";
        var description = extractXArticleDescription(article);
        var prevX = map[found.canonicalUrl];
        if (!prevX) {
          if (title || description) {
            map[found.canonicalUrl] = {
              title: title,
              description: description
            };
          }
        } else {
          map[found.canonicalUrl] = {
            title: prevX.title || title,
            description: prevX.description || description
          };
        }
      }
      return map;
    }

    if (platform === "threads") {
      var tOrigin = baseOrigin || "https://www.threads.com";
      var anchors = safeQueryAll(rootEl, "a[href]");
      for (var j = 0; j < anchors.length; j++) {
        var anchor = anchors[j];
        if (safeGetAttr(anchor, "aria-disabled") === "true") {
          continue;
        }
        if (safeClosest(anchor, '[data-unavailable="true"], [data-deleted="true"]')) {
          continue;
        }
        var info = parseThreadsPostUrl(safeGetAttr(anchor, "href"), tOrigin);
        if (!info) {
          continue;
        }
        var existing = map[info.canonicalUrl];
        var tTitle = info.user ? "@" + info.user + " on Threads" : "";
        var tDesc = extractThreadsAnchorDescription(anchor, info.user);
        if (!existing || (!existing.description && tDesc) || (!existing.title && tTitle)) {
          map[info.canonicalUrl] = {
            title: (existing && existing.title) || tTitle,
            description: (existing && existing.description) || tDesc
          };
        }
      }
      return map;
    }

    return map;
  }

  function extractPostUrls(platform, rootEl, baseOrigin, options) {
    if (platform === "x") {
      return extractXPostUrls(rootEl, baseOrigin, options);
    }
    if (platform === "threads") {
      return extractThreadsPostUrls(rootEl, baseOrigin, options);
    }
    return [];
  }

  function mergeUniqueUrls(existingUrls, incomingUrls, maxCount) {
    var limit = typeof maxCount === "number" && maxCount > 0 ? maxCount : MAX_POSTS_PER_SESSION;
    var existing = Array.isArray(existingUrls) ? existingUrls : [];
    var incoming = Array.isArray(incomingUrls) ? incomingUrls : [];

    var seen = Object.create(null);
    var out = [];

    for (var i = 0; i < existing.length; i++) {
      var u = String(existing[i] || "").trim();
      if (!u || seen[u]) {
        continue;
      }
      seen[u] = true;
      out.push(u);
      if (out.length >= limit) {
        return { urls: out, added: 0 };
      }
    }

    var added = 0;
    for (var j = 0; j < incoming.length; j++) {
      if (out.length >= limit) {
        break;
      }
      var next = String(incoming[j] || "").trim();
      if (!next || seen[next]) {
        continue;
      }
      seen[next] = true;
      out.push(next);
      added++;
    }

    return {
      urls: out,
      added: added
    };
  }

  var api = {
    MAX_POSTS_PER_SESSION: MAX_POSTS_PER_SESSION,
    detectPage: detectPage,
    extractXPostUrls: extractXPostUrls,
    extractThreadsPostUrls: extractThreadsPostUrls,
    extractPostUrls: extractPostUrls,
    extractPostMetadataMap: extractPostMetadataMap,
    mergeUniqueUrls: mergeUniqueUrls
  };

  root.SocialVaultBulkExtractor = api;
  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
})(typeof globalThis !== "undefined" ? globalThis : this);
