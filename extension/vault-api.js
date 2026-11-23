(function (root) {
  "use strict";

  var DEFAULT_VAULT_URL = "https://vault.adityahimaone.space";
  var SAVE_TIMEOUT_MS = 15000;
  var TEST_TIMEOUT_MS = 10000;

  function normalizeVaultUrl(raw) {
    var trimmed = String(raw || "").trim();
    if (!trimmed) {
      trimmed = DEFAULT_VAULT_URL;
    }

    var parsed;
    try {
      parsed = new URL(trimmed);
    } catch (_err) {
      return {
        ok: false,
        error: "Enter a valid http:// or https:// Vault URL."
      };
    }

    if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || !parsed.hostname) {
      return {
        ok: false,
        error: "Vault URL must use http:// or https://."
      };
    }

    var baseUrl = parsed.origin + parsed.pathname.replace(/\/+$/, "");
    if (baseUrl === parsed.origin + "/") {
      baseUrl = parsed.origin;
    }

    return {
      ok: true,
      url: baseUrl,
      origin: parsed.origin,
      originPattern: parsed.origin + "/*"
    };
  }

  function validateBookmarkUrl(raw) {
    var trimmed = String(raw || "").trim();
    if (!trimmed) {
      return {
        ok: false,
        error: "Page URL is required."
      };
    }

    var parsed;
    try {
      parsed = new URL(trimmed);
    } catch (_err) {
      return {
        ok: false,
        error: "Enter a valid web address starting with http:// or https://."
      };
    }

    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return {
        ok: false,
        error: "Browser internal pages (" + parsed.protocol + ") cannot be saved. Paste an http:// or https:// link."
      };
    }

    return {
      ok: true,
      url: parsed.toString()
    };
  }

  function parseTags(raw) {
    var parts = String(raw || "").split(",");
    var out = [];
    var seen = Object.create(null);
    for (var i = 0; i < parts.length; i++) {
      var tag = parts[i].trim();
      if (!tag) {
        continue;
      }
      var key = tag.toLowerCase();
      if (seen[key]) {
        continue;
      }
      seen[key] = true;
      out.push(tag);
    }
    return out;
  }

  function getSettings() {
    return new Promise(function (resolve) {
      chrome.storage.local.get(["vaultUrl", "apiKey"], function (items) {
        var normalized = normalizeVaultUrl(items && items.vaultUrl);
        resolve({
          vaultUrl: normalized.ok ? normalized.url : DEFAULT_VAULT_URL,
          originPattern: normalized.ok ? normalized.originPattern : DEFAULT_VAULT_URL + "/*",
          apiKey: items && typeof items.apiKey === "string" ? items.apiKey.trim() : ""
        });
      });
    });
  }

  function saveSettings(settings) {
    var normalized = normalizeVaultUrl(settings && settings.vaultUrl);
    if (!normalized.ok) {
      return Promise.reject(new Error(normalized.error));
    }

    var apiKey = String((settings && settings.apiKey) || "").trim();
    if (!apiKey) {
      return Promise.reject(new Error("API key is required."));
    }

    return new Promise(function (resolve, reject) {
      chrome.storage.local.set(
        {
          vaultUrl: normalized.url,
          apiKey: apiKey
        },
        function () {
          if (chrome.runtime && chrome.runtime.lastError) {
            reject(new Error(chrome.runtime.lastError.message || "Could not save settings locally."));
            return;
          }
          resolve({
            vaultUrl: normalized.url,
            originPattern: normalized.originPattern,
            apiKey: apiKey
          });
        }
      );
    });
  }

  function hasOriginPermission(originPattern) {
    return new Promise(function (resolve) {
      if (!chrome.permissions || !chrome.permissions.contains) {
        resolve(true);
        return;
      }
      chrome.permissions.contains({ origins: [originPattern] }, function (granted) {
        resolve(Boolean(granted));
      });
    });
  }

  function requestOriginPermission(originPattern) {
    return new Promise(function (resolve) {
      if (!chrome.permissions || !chrome.permissions.request) {
        resolve(false);
        return;
      }
      chrome.permissions.request({ origins: [originPattern] }, function (granted) {
        resolve(Boolean(granted));
      });
    });
  }

  function fetchWithTimeout(url, options, timeoutMs) {
    var controller = new AbortController();
    var timer = setTimeout(function () {
      controller.abort();
    }, timeoutMs);

    var opts = Object.assign({}, options, { signal: controller.signal });
    return fetch(url, opts).finally(function () {
      clearTimeout(timer);
    });
  }

  function parseJSONSafe(response) {
    return response.text().then(function (text) {
      if (!text) {
        return null;
      }
      try {
        return JSON.parse(text);
      } catch (_err) {
        return null;
      }
    });
  }

  function explainNetworkFailure(vaultUrl, err) {
    if (err && err.name === "AbortError") {
      return "Request timed out. The Social Vault server took too long to respond.";
    }
    return "Could not reach " + vaultUrl + ". Check your network connection, Vault URL, or browser site permission (CORS/TLS).";
  }

  function explainHTTPError(status, body, vaultUrl) {
    var apiMsg = body && typeof body.error === "string" ? body.error.trim() : "";

    if (status === 401 || status === 403) {
      return "Your API key was rejected (HTTP " + status + "). Update it in Extension Settings.";
    }
    if (status === 404) {
      return "Vault endpoint not found at " + vaultUrl + " (HTTP 404). Check your Vault URL.";
    }
    if (status === 400) {
      return apiMsg ? "Invalid bookmark details: " + apiMsg : "Invalid bookmark request (HTTP 400).";
    }
    if (status >= 500) {
      return apiMsg
        ? "Social Vault server error (HTTP " + status + "): " + apiMsg
        : "Social Vault server error (HTTP " + status + "). Try again in a moment.";
    }
    return apiMsg || "Request failed with HTTP " + status + ".";
  }

  function saveBookmark(input) {
    var bookmarkCheck = validateBookmarkUrl(input && input.url);
    if (!bookmarkCheck.ok) {
      return Promise.resolve({
        ok: false,
        code: "INVALID_URL",
        error: bookmarkCheck.error
      });
    }

    return getSettings().then(function (settings) {
      if (!settings.apiKey) {
        return {
          ok: false,
          code: "NO_API_KEY",
          vaultUrl: settings.vaultUrl,
          error: "API key is not configured yet. Open Settings to add your VAULT_API_KEY."
        };
      }

      return hasOriginPermission(settings.originPattern).then(function (hasPerm) {
        if (!hasPerm) {
          return {
            ok: false,
            code: "PERMISSION_REQUIRED",
            vaultUrl: settings.vaultUrl,
            originPattern: settings.originPattern,
            error: "Permission needed to connect to " + settings.vaultUrl + "."
          };
        }

        var payload = {
          url: bookmarkCheck.url,
          source: (input && input.source) || "extension"
        };

        var title = String((input && input.title) || "").trim();
        if (title) {
          payload.title = title;
        }
        var notes = String((input && input.notes) || "").trim();
        if (notes) {
          payload.notes = notes;
        }
        if (input && Array.isArray(input.tags) && input.tags.length > 0) {
          payload.tags = input.tags;
        }

        return fetchWithTimeout(
          settings.vaultUrl + "/api/bookmarks",
          {
            method: "POST",
            headers: {
              "Authorization": "Bearer " + settings.apiKey,
              "Content-Type": "application/json",
              "Accept": "application/json"
            },
            body: JSON.stringify(payload)
          },
          SAVE_TIMEOUT_MS
        )
          .then(function (response) {
            return parseJSONSafe(response).then(function (body) {
              if (response.status === 201 || response.ok) {
                return {
                  ok: true,
                  status: response.status,
                  duplicate: false,
                  vaultUrl: settings.vaultUrl,
                  bookmark: body
                };
              }

              if (response.status === 409) {
                return {
                  ok: false,
                  code: "DUPLICATE",
                  status: 409,
                  duplicate: true,
                  vaultUrl: settings.vaultUrl,
                  bookmark: body && body.bookmark ? body.bookmark : null,
                  error: "This link is already saved in your Social Vault."
                };
              }

              return {
                ok: false,
                code: response.status === 401 || response.status === 403 ? "UNAUTHORIZED" : "HTTP_ERROR",
                status: response.status,
                vaultUrl: settings.vaultUrl,
                error: explainHTTPError(response.status, body, settings.vaultUrl)
              };
            });
          })
          .catch(function (err) {
            return {
              ok: false,
              code: "NETWORK_ERROR",
              vaultUrl: settings.vaultUrl,
              error: explainNetworkFailure(settings.vaultUrl, err)
            };
          });
      });
    });
  }

  function testConnection(rawSettings) {
    var normalized = normalizeVaultUrl(rawSettings && rawSettings.vaultUrl);
    if (!normalized.ok) {
      return Promise.resolve({
        ok: false,
        code: "INVALID_URL",
        error: normalized.error
      });
    }

    var apiKey = String((rawSettings && rawSettings.apiKey) || "").trim();
    if (!apiKey) {
      return Promise.resolve({
        ok: false,
        code: "NO_API_KEY",
        error: "Enter an API key before testing the connection."
      });
    }

    return hasOriginPermission(normalized.originPattern).then(function (hasPerm) {
      if (!hasPerm) {
        return {
          ok: false,
          code: "PERMISSION_REQUIRED",
          originPattern: normalized.originPattern,
          error: "Permission needed to access " + normalized.url + "."
        };
      }

      return fetchWithTimeout(
        normalized.url + "/api/stats",
        {
          method: "GET",
          headers: {
            "Authorization": "Bearer " + apiKey,
            "Accept": "application/json"
          }
        },
        TEST_TIMEOUT_MS
      )
        .then(function (response) {
          return parseJSONSafe(response).then(function (body) {
            if (response.ok) {
              var total = body && typeof body.total === "number" ? body.total : null;
              return {
                ok: true,
                stats: body,
                vaultUrl: normalized.url,
                message: total !== null
                  ? "Connected to Social Vault (" + total + " saved bookmarks)."
                  : "Connected to Social Vault."
              };
            }

            return {
              ok: false,
              code: response.status === 401 || response.status === 403 ? "UNAUTHORIZED" : "HTTP_ERROR",
              status: response.status,
              error: explainHTTPError(response.status, body, normalized.url)
            };
          });
        })
        .catch(function (err) {
          return {
            ok: false,
            code: "NETWORK_ERROR",
            error: explainNetworkFailure(normalized.url, err)
          };
        });
    });
  }

  root.SocialVaultAPI = {
    DEFAULT_VAULT_URL: DEFAULT_VAULT_URL,
    normalizeVaultUrl: normalizeVaultUrl,
    validateBookmarkUrl: validateBookmarkUrl,
    parseTags: parseTags,
    getSettings: getSettings,
    saveSettings: saveSettings,
    hasOriginPermission: hasOriginPermission,
    requestOriginPermission: requestOriginPermission,
    saveBookmark: saveBookmark,
    testConnection: testConnection
  };
})(typeof globalThis !== "undefined" ? globalThis : this);
