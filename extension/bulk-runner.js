(function (root) {
  "use strict";

  var DEFAULT_MAX_POSTS = 200;
  var DEFAULT_SAVE_DELAY_MS = 150;
  var DEFAULT_SCROLL_WAIT_MS = 900;
  var DEFAULT_MAX_STAGNANT_ROUNDS = 3;

  function defaultSleep(ms) {
    return new Promise(function (resolve) {
      setTimeout(resolve, ms);
    });
  }

  function mergeUrlsLocal(existing, incoming, maxPosts) {
    if (root.SocialVaultBulkExtractor && typeof root.SocialVaultBulkExtractor.mergeUniqueUrls === "function") {
      return root.SocialVaultBulkExtractor.mergeUniqueUrls(existing, incoming, maxPosts);
    }

    var seen = Object.create(null);
    var out = [];
    for (var i = 0; i < existing.length; i++) {
      var u = existing[i];
      if (u && !seen[u]) {
        seen[u] = true;
        out.push(u);
        if (out.length >= maxPosts) {
          return { urls: out, added: 0 };
        }
      }
    }
    var added = 0;
    for (var j = 0; j < incoming.length; j++) {
      if (out.length >= maxPosts) {
        break;
      }
      var next = incoming[j];
      if (next && !seen[next]) {
        seen[next] = true;
        out.push(next);
        added++;
      }
    }
    return { urls: out, added: added };
  }

  function createBulkSession(options) {
    var opts = options || {};
    var maxPosts = typeof opts.maxPosts === "number" && opts.maxPosts > 0 ? opts.maxPosts : DEFAULT_MAX_POSTS;
    var saveDelayMs = typeof opts.saveDelayMs === "number" && opts.saveDelayMs >= 0 ? opts.saveDelayMs : DEFAULT_SAVE_DELAY_MS;
    var scrollWaitMs = typeof opts.scrollWaitMs === "number" && opts.scrollWaitMs >= 0 ? opts.scrollWaitMs : DEFAULT_SCROLL_WAIT_MS;
    var maxStagnantRounds =
      typeof opts.maxStagnantRounds === "number" && opts.maxStagnantRounds > 0
        ? opts.maxStagnantRounds
        : DEFAULT_MAX_STAGNANT_ROUNDS;

    var extractUrls = typeof opts.extractUrls === "function" ? opts.extractUrls : function () { return []; };
    var scrollStep = typeof opts.scrollStep === "function" ? opts.scrollStep : function () {};
    var getScrollMetrics =
      typeof opts.getScrollMetrics === "function"
        ? opts.getScrollMetrics
        : function () { return { scrollY: 0, scrollHeight: 0 }; };
    var saveUrl = opts.saveUrl;
    var onProgress = typeof opts.onProgress === "function" ? opts.onProgress : function () {};
    var sleepFn = typeof opts.sleep === "function" ? opts.sleep : defaultSleep;

    var aborted = false;
    var running = false;

    function interruptibleWait(totalMs) {
      if (totalMs <= 0 || aborted) {
        return Promise.resolve();
      }
      var chunkMs = 100;
      var remaining = totalMs;

      function nextChunk() {
        if (aborted || remaining <= 0) {
          return Promise.resolve();
        }
        var step = remaining > chunkMs ? chunkMs : remaining;
        remaining -= step;
        return sleepFn(step).then(nextChunk);
      }

      return nextChunk();
    }

    function stop() {
      aborted = true;
    }

    function isRunning() {
      return running;
    }

    function start(runOptions) {
      if (running) {
        return Promise.reject(new Error("Bulk session is already running."));
      }
      if (typeof saveUrl !== "function") {
        return Promise.reject(new Error("saveUrl handler is required."));
      }

      var cfg = runOptions || {};
      var autoScroll = cfg.autoScroll !== false;
      var initialUrls = Array.isArray(cfg.initialUrls) ? cfg.initialUrls : [];
      var minDocumentTop =
        typeof cfg.minDocumentTop === "number" && isFinite(cfg.minDocumentTop) && cfg.minDocumentTop > 0
          ? cfg.minDocumentTop
          : 0;
      var extractOptions = minDocumentTop > 0 ? { minDocumentTop: minDocumentTop } : undefined;

      aborted = false;
      running = true;

      var collected = mergeUrlsLocal([], initialUrls, maxPosts).urls;
      var firstBatch = extractUrls(extractOptions) || [];
      collected = mergeUrlsLocal(collected, firstBatch, maxPosts).urls;

      var stats = {
        phase: "collecting",
        collected: collected.length,
        total: collected.length,
        processed: 0,
        saved: 0,
        skipped: 0,
        failed: 0,
        limitReached: collected.length >= maxPosts,
        minDocumentTop: minDocumentTop,
        resumedFromScroll: minDocumentTop > 0,
        fatalError: null,
        lastError: null
      };

      onProgress(Object.assign({}, stats));

      function runScrollPhase() {
        if (!autoScroll || aborted || collected.length >= maxPosts) {
          return Promise.resolve();
        }

        var stagnantRounds = 0;

        function nextScrollRound() {
          if (aborted || collected.length >= maxPosts || stagnantRounds >= maxStagnantRounds) {
            return Promise.resolve();
          }

          var before = getScrollMetrics() || { scrollY: 0, scrollHeight: 0 };
          scrollStep();

          return interruptibleWait(scrollWaitMs).then(function () {
            if (aborted) {
              return;
            }

            var batch = extractUrls(extractOptions) || [];
            var merged = mergeUrlsLocal(collected, batch, maxPosts);
            collected = merged.urls;

            var after = getScrollMetrics() || { scrollY: 0, scrollHeight: 0 };
            var movedY = Math.abs((after.scrollY || 0) - (before.scrollY || 0));
            var grewHeight = Math.abs((after.scrollHeight || 0) - (before.scrollHeight || 0));

            if (merged.added === 0 && movedY <= 8 && grewHeight <= 8) {
              stagnantRounds++;
            } else {
              stagnantRounds = 0;
            }

            stats.phase = "scrolling";
            stats.collected = collected.length;
            stats.total = collected.length;
            stats.limitReached = collected.length >= maxPosts;
            onProgress(Object.assign({}, stats));

            return nextScrollRound();
          });
        }

        stats.phase = "scrolling";
        onProgress(Object.assign({}, stats));
        return nextScrollRound();
      }

      function runSavePhase() {
        if (aborted) {
          stats.phase = "stopped";
          onProgress(Object.assign({}, stats));
          return Promise.resolve(Object.assign({}, stats));
        }

        stats.phase = "saving";
        stats.collected = collected.length;
        stats.total = collected.length;
        stats.limitReached = collected.length >= maxPosts;
        onProgress(Object.assign({}, stats));

        var index = 0;

        function nextSave() {
          if (aborted || index >= collected.length || stats.fatalError) {
            if (stats.fatalError) {
              stats.phase = "error";
            } else if (aborted) {
              stats.phase = "stopped";
            } else {
              stats.phase = "done";
            }
            onProgress(Object.assign({}, stats));
            return Promise.resolve(Object.assign({}, stats));
          }

          var currentUrl = collected[index];
          index++;

          return Promise.resolve()
            .then(function () {
              return saveUrl(currentUrl);
            })
            .catch(function (err) {
              return {
                ok: false,
                code: "NETWORK_ERROR",
                error: err && err.message ? err.message : "Failed to save bookmark."
              };
            })
            .then(function (res) {
              if (res && res.ok) {
                stats.saved++;
              } else if (res && (res.code === "DUPLICATE" || res.status === 409)) {
                stats.skipped++;
              } else if (
                res &&
                (res.code === "NO_API_KEY" || res.code === "UNAUTHORIZED" || res.code === "PERMISSION_REQUIRED")
              ) {
                stats.failed++;
                stats.fatalError = {
                  code: res.code,
                  message: res.error || "Vault configuration or authorization error."
                };
                stats.lastError = stats.fatalError.message;
              } else {
                stats.failed++;
                stats.lastError = (res && res.error) || "Failed to save bookmark.";
              }

              stats.processed++;
              onProgress(Object.assign({}, stats));

              if (aborted || stats.fatalError || index >= collected.length) {
                return nextSave();
              }

              return interruptibleWait(saveDelayMs).then(nextSave);
            });
        }

        return nextSave();
      }

      return runScrollPhase()
        .then(runSavePhase)
        .finally(function () {
          running = false;
        });
    }

    return {
      start: start,
      stop: stop,
      isRunning: isRunning
    };
  }

  var api = {
    DEFAULT_MAX_POSTS: DEFAULT_MAX_POSTS,
    DEFAULT_SAVE_DELAY_MS: DEFAULT_SAVE_DELAY_MS,
    DEFAULT_SCROLL_WAIT_MS: DEFAULT_SCROLL_WAIT_MS,
    DEFAULT_MAX_STAGNANT_ROUNDS: DEFAULT_MAX_STAGNANT_ROUNDS,
    createBulkSession: createBulkSession
  };

  root.SocialVaultBulkRunner = api;
  if (typeof module !== "undefined" && module.exports) {
    module.exports = api;
  }
})(typeof globalThis !== "undefined" ? globalThis : this);
