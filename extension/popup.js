(function () {
  "use strict";

  var api = globalThis.SocialVaultAPI;

  var form = document.getElementById("save-form");
  var urlInput = document.getElementById("bookmark-url");
  var urlError = document.getElementById("bookmark-url-error");
  var titleInput = document.getElementById("bookmark-title");
  var tagsInput = document.getElementById("bookmark-tags");
  var tagPreview = document.getElementById("tag-preview");
  var notesInput = document.getElementById("bookmark-notes");

  var saveBtn = document.getElementById("save-btn");
  var saveSpinner = document.getElementById("save-btn-spinner");
  var saveLabel = document.getElementById("save-btn-label");

  var setupBanner = document.getElementById("setup-banner");
  var setupSettingsBtn = document.getElementById("setup-settings-btn");
  var permissionBanner = document.getElementById("permission-banner");
  var permissionBannerText = document.getElementById("permission-banner-text");
  var grantPermissionBtn = document.getElementById("grant-permission-btn");
  var statusBanner = document.getElementById("status-banner");

  var openOptionsBtn = document.getElementById("open-options-btn");
  var vaultHostLabel = document.getElementById("vault-host-label");
  var openVaultLink = document.getElementById("open-vault-link");

  var currentOriginPattern = "";
  var isSubmitting = false;

  function openOptions() {
    if (chrome.runtime && chrome.runtime.openOptionsPage) {
      chrome.runtime.openOptionsPage();
    }
  }

  function setFieldError(message) {
    if (!message) {
      urlError.textContent = "";
      urlError.classList.add("hidden");
      urlInput.removeAttribute("aria-invalid");
      urlInput.removeAttribute("aria-describedby");
      return;
    }
    urlError.textContent = message;
    urlError.classList.remove("hidden");
    urlInput.setAttribute("aria-invalid", "true");
    urlInput.setAttribute("aria-describedby", "bookmark-url-error");
  }

  function clearStatusBanner() {
    statusBanner.className = "banner hidden";
    statusBanner.replaceChildren();
    statusBanner.setAttribute("role", "status");
  }

  function showStatusBanner(kind, title, detail, options) {
    var opts = options || {};
    statusBanner.replaceChildren();
    statusBanner.className = "banner banner-" + kind;
    statusBanner.setAttribute("role", kind === "error" ? "alert" : "status");

    var body = document.createElement("div");
    body.className = "banner-body";

    var icon = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    icon.setAttribute("class", "banner-icon");
    icon.setAttribute("width", "16");
    icon.setAttribute("height", "16");
    icon.setAttribute("viewBox", "0 0 24 24");
    icon.setAttribute("fill", "none");
    icon.setAttribute("stroke", "currentColor");
    icon.setAttribute("stroke-width", "2");
    icon.setAttribute("stroke-linecap", "round");
    icon.setAttribute("stroke-linejoin", "round");
    icon.setAttribute("aria-hidden", "true");

    if (kind === "success") {
      var check = document.createElementNS("http://www.w3.org/2000/svg", "polyline");
      check.setAttribute("points", "20 6 9 17 4 12");
      icon.appendChild(check);
    } else if (kind === "error") {
      var circle = document.createElementNS("http://www.w3.org/2000/svg", "circle");
      circle.setAttribute("cx", "12");
      circle.setAttribute("cy", "12");
      circle.setAttribute("r", "10");
      var line1 = document.createElementNS("http://www.w3.org/2000/svg", "line");
      line1.setAttribute("x1", "12");
      line1.setAttribute("y1", "8");
      line1.setAttribute("x2", "12");
      line1.setAttribute("y2", "12");
      var line2 = document.createElementNS("http://www.w3.org/2000/svg", "line");
      line2.setAttribute("x1", "12");
      line2.setAttribute("y1", "16");
      line2.setAttribute("x2", "12.01");
      line2.setAttribute("y2", "16");
      icon.appendChild(circle);
      icon.appendChild(line1);
      icon.appendChild(line2);
    } else {
      var infoCircle = document.createElementNS("http://www.w3.org/2000/svg", "circle");
      infoCircle.setAttribute("cx", "12");
      infoCircle.setAttribute("cy", "12");
      infoCircle.setAttribute("r", "10");
      var infoLine1 = document.createElementNS("http://www.w3.org/2000/svg", "line");
      infoLine1.setAttribute("x1", "12");
      infoLine1.setAttribute("y1", "16");
      infoLine1.setAttribute("x2", "12");
      infoLine1.setAttribute("y2", "12");
      var infoLine2 = document.createElementNS("http://www.w3.org/2000/svg", "line");
      infoLine2.setAttribute("x1", "12");
      infoLine2.setAttribute("y1", "8");
      infoLine2.setAttribute("x2", "12.01");
      infoLine2.setAttribute("y2", "8");
      icon.appendChild(infoCircle);
      icon.appendChild(infoLine1);
      icon.appendChild(infoLine2);
    }

    var textWrap = document.createElement("div");
    textWrap.className = "banner-text";

    var titleEl = document.createElement("span");
    titleEl.className = "banner-title";
    titleEl.textContent = title;
    textWrap.appendChild(titleEl);

    if (detail) {
      var detailEl = document.createElement("span");
      detailEl.className = "banner-meta";
      detailEl.textContent = detail;
      textWrap.appendChild(detailEl);
    }

    if (opts.vaultUrl) {
      var link = document.createElement("a");
      link.className = "banner-link";
      link.href = opts.vaultUrl;
      link.target = "_blank";
      link.rel = "noreferrer";
      link.textContent = "Open Social Vault";
      textWrap.appendChild(link);
    }

    body.appendChild(icon);
    body.appendChild(textWrap);
    statusBanner.appendChild(body);

    if (opts.showSettingsButton) {
      var actions = document.createElement("div");
      actions.className = "banner-actions";
      var settingsBtn = document.createElement("button");
      settingsBtn.type = "button";
      settingsBtn.className = "btn btn-secondary btn-sm";
      settingsBtn.textContent = "Settings";
      settingsBtn.addEventListener("click", openOptions);
      actions.appendChild(settingsBtn);
      statusBanner.appendChild(actions);
    }
  }

  function renderTagPreview() {
    var tags = api.parseTags(tagsInput.value);
    tagPreview.replaceChildren();
    if (tags.length === 0) {
      tagPreview.classList.add("hidden");
      return;
    }

    for (var i = 0; i < tags.length; i++) {
      var chip = document.createElement("span");
      chip.className = "tag-chip";
      chip.textContent = "#" + tags[i];
      tagPreview.appendChild(chip);
    }
    tagPreview.classList.remove("hidden");
  }

  function setSubmitting(busy) {
    isSubmitting = busy;
    saveBtn.disabled = busy;
    saveSpinner.classList.toggle("hidden", !busy);
    saveLabel.textContent = busy ? "Saving..." : "Save to Vault";
  }

  function notifyBadge(kind, title) {
    if (chrome.runtime && chrome.runtime.sendMessage) {
      chrome.runtime.sendMessage({
        type: "FLASH_BADGE",
        kind: kind,
        title: title
      });
    }
  }

  function validateUrlField() {
    var check = api.validateBookmarkUrl(urlInput.value);
    if (!check.ok) {
      setFieldError(check.error);
      return false;
    }
    setFieldError("");
    return true;
  }

  function refreshConfigurationState() {
    return api.getSettings().then(function (settings) {
      try {
        vaultHostLabel.textContent = new URL(settings.vaultUrl).host;
      } catch (_err) {
        vaultHostLabel.textContent = settings.vaultUrl;
      }
      openVaultLink.href = settings.vaultUrl;
      currentOriginPattern = settings.originPattern;

      var missingKey = !settings.apiKey;
      setupBanner.classList.toggle("hidden", !missingKey);

      return api.hasOriginPermission(settings.originPattern).then(function (granted) {
        if (!granted && !missingKey) {
          permissionBannerText.textContent = "Allow access to " + settings.vaultUrl + " to save bookmarks.";
          permissionBanner.classList.remove("hidden");
        } else {
          permissionBanner.classList.add("hidden");
        }
      });
    });
  }

  function loadActiveTab() {
    if (!chrome.tabs || !chrome.tabs.query) {
      return;
    }
    chrome.tabs.query({ active: true, currentWindow: true }, function (tabs) {
      var tab = tabs && tabs[0];
      if (!tab) {
        return;
      }
      var rawUrl = tab.url || "";
      var check = api.validateBookmarkUrl(rawUrl);
      if (check.ok) {
        urlInput.value = check.url;
        titleInput.value = (tab.title || "").trim();
        tagsInput.focus();
      } else {
        urlInput.value = "";
        titleInput.value = "";
        urlInput.focus();
      }
    });
  }

  function handleSave(event) {
    if (event) {
      event.preventDefault();
    }
    if (isSubmitting) {
      return;
    }

    clearStatusBanner();
    if (!validateUrlField()) {
      urlInput.focus();
      return;
    }

    setSubmitting(true);

    api
      .saveBookmark({
        url: urlInput.value,
        title: titleInput.value,
        notes: notesInput.value,
        tags: api.parseTags(tagsInput.value),
        source: "extension"
      })
      .then(function (result) {
        if (result.ok) {
          var savedTitle =
            (result.bookmark && result.bookmark.title) ||
            titleInput.value.trim() ||
            urlInput.value.trim();
          showStatusBanner("success", "Saved to Social Vault", savedTitle, {
            vaultUrl: result.vaultUrl
          });
          notifyBadge("success", "Saved to Social Vault");
          return;
        }

        if (result.code === "DUPLICATE") {
          var existingTitle = result.bookmark && result.bookmark.title ? result.bookmark.title : "";
          showStatusBanner("info", "Already in Social Vault", existingTitle || result.error, {
            vaultUrl: result.vaultUrl
          });
          notifyBadge("duplicate", "Already in Social Vault");
          return;
        }

        if (result.code === "PERMISSION_REQUIRED") {
          currentOriginPattern = result.originPattern || currentOriginPattern;
          permissionBanner.classList.remove("hidden");
          showStatusBanner("error", "Site access required", result.error);
          return;
        }

        var showSettings = result.code === "NO_API_KEY" || result.code === "UNAUTHORIZED";
        showStatusBanner("error", "Could not save bookmark", result.error, {
          showSettingsButton: showSettings
        });
        notifyBadge("error", result.error);
      })
      .finally(function () {
        setSubmitting(false);
      });
  }

  openOptionsBtn.addEventListener("click", openOptions);
  setupSettingsBtn.addEventListener("click", openOptions);

  grantPermissionBtn.addEventListener("click", function () {
    if (!currentOriginPattern) {
      return;
    }
    api.requestOriginPermission(currentOriginPattern).then(function (granted) {
      if (granted) {
        permissionBanner.classList.add("hidden");
        clearStatusBanner();
      } else {
        showStatusBanner("error", "Permission denied", "Allow access to your Vault URL to save links.");
      }
    });
  });

  urlInput.addEventListener("blur", function () {
    if (urlInput.value.trim() !== "") {
      validateUrlField();
    } else {
      setFieldError("");
    }
  });

  urlInput.addEventListener("input", function () {
    if (urlInput.getAttribute("aria-invalid") === "true") {
      validateUrlField();
    }
  });

  tagsInput.addEventListener("input", renderTagPreview);
  form.addEventListener("submit", handleSave);

  document.addEventListener("keydown", function (event) {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      handleSave(event);
    }
  });

  refreshConfigurationState();
  loadActiveTab();
})();
