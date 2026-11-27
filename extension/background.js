importScripts("vault-api.js");

(function () {
  "use strict";

  var MENU_SAVE_PAGE = "sv-save-page";
  var MENU_SAVE_LINK = "sv-save-link";
  var DEFAULT_TITLE = "Save to Social Vault";
  var badgeTimer = null;

  function registerContextMenus() {
    if (!chrome.contextMenus) {
      return;
    }

    chrome.contextMenus.removeAll(function () {
      chrome.contextMenus.create({
        id: MENU_SAVE_PAGE,
        title: "Save page to Social Vault",
        contexts: ["page"]
      });

      chrome.contextMenus.create({
        id: MENU_SAVE_LINK,
        title: "Save link to Social Vault",
        contexts: ["link"]
      });
    });
  }

  function setBadge(text, color, title) {
    if (badgeTimer !== null) {
      clearTimeout(badgeTimer);
      badgeTimer = null;
    }

    chrome.action.setBadgeText({ text: text });
    if (color) {
      chrome.action.setBadgeBackgroundColor({ color: color });
    }
    if (chrome.action.setBadgeTextColor) {
      chrome.action.setBadgeTextColor({ color: "#FFFFFF" });
    }
    if (title) {
      chrome.action.setTitle({ title: title });
    }
  }

  function flashBadge(kind, customTitle) {
    var text = "✓";
    var color = "#10B981";
    var title = customTitle || "Saved to Social Vault";

    if (kind === "duplicate") {
      text = "•";
      color = "#0284C7";
      title = customTitle || "Already in Social Vault";
    } else if (kind === "error") {
      text = "!";
      color = "#EF4444";
      title = customTitle || "Could not save to Social Vault";
    }

    setBadge(text, color, title);
    badgeTimer = setTimeout(function () {
      chrome.action.setBadgeText({ text: "" });
      chrome.action.setTitle({ title: DEFAULT_TITLE });
      badgeTimer = null;
    }, 3000);
  }

  chrome.runtime.onInstalled.addListener(registerContextMenus);
  chrome.runtime.onStartup.addListener(registerContextMenus);

  chrome.contextMenus.onClicked.addListener(function (info, tab) {
    if (!info) {
      return;
    }

    var targetUrl = "";
    var targetTitle = "";

    if (info.menuItemId === MENU_SAVE_LINK && info.linkUrl) {
      targetUrl = info.linkUrl;
    } else if (info.menuItemId === MENU_SAVE_PAGE) {
      targetUrl = info.pageUrl || (tab && tab.url) || "";
      targetTitle = (tab && tab.title) || "";
    } else {
      return;
    }

    setBadge("...", "#1E3A5F", "Saving to Social Vault...");

    globalThis.SocialVaultAPI.saveBookmark({
      url: targetUrl,
      title: targetTitle,
      source: "extension-context-menu"
    }).then(function (result) {
      if (result.ok) {
        flashBadge("success", "Saved to Social Vault");
        return;
      }

      if (result.code === "DUPLICATE") {
        flashBadge("duplicate", result.error);
        return;
      }

      flashBadge("error", result.error);
      if (result.code === "NO_API_KEY" && chrome.runtime.openOptionsPage) {
        chrome.runtime.openOptionsPage();
      }
    });
  });

  chrome.runtime.onMessage.addListener(function (message, sender, sendResponse) {
    if (!message || !message.type) {
      return;
    }

    if (message.type === "FLASH_BADGE") {
      flashBadge(message.kind || "success", message.title);
      return;
    }

    if (!sender || sender.id !== chrome.runtime.id) {
      return;
    }

    if (message.type === "BULK_GET_CONFIG") {
      globalThis.SocialVaultAPI.getSettings()
        .then(function (settings) {
          return globalThis.SocialVaultAPI.hasOriginPermission(settings.originPattern).then(function (granted) {
            var vaultHost = settings.vaultUrl;
            try {
              vaultHost = new URL(settings.vaultUrl).host;
            } catch (_err) {}

            sendResponse({
              ok: true,
              hasApiKey: Boolean(settings.apiKey),
              hasPermission: Boolean(granted),
              vaultUrl: settings.vaultUrl,
              vaultHost: vaultHost
            });
          });
        })
        .catch(function (err) {
          sendResponse({
            ok: false,
            error: (err && err.message) || "Could not read extension settings."
          });
        });
      return true;
    }

    if (message.type === "BULK_SAVE_BOOKMARK") {
      globalThis.SocialVaultAPI.saveBookmark({
        url: message.url,
        title: message.title,
        description: message.description,
        source: message.source || "extension-bulk"
      })
        .then(function (result) {
          sendResponse(result);
        })
        .catch(function (err) {
          sendResponse({
            ok: false,
            code: "NETWORK_ERROR",
            error: (err && err.message) || "Failed to save bookmark."
          });
        });
      return true;
    }

    if (message.type === "OPEN_OPTIONS") {
      if (chrome.runtime && chrome.runtime.openOptionsPage) {
        chrome.runtime.openOptionsPage();
      }
      sendResponse({ ok: true });
      return;
    }
  });
})();
