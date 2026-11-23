(function () {
  "use strict";

  var api = globalThis.SocialVaultAPI;

  var form = document.getElementById("options-form");
  var vaultUrlInput = document.getElementById("vault-url");
  var vaultUrlError = document.getElementById("vault-url-error");
  var apiKeyInput = document.getElementById("api-key");
  var apiKeyError = document.getElementById("api-key-error");
  var toggleKeyBtn = document.getElementById("toggle-key-btn");
  var resetUrlBtn = document.getElementById("reset-url-btn");

  var saveBtn = document.getElementById("save-settings-btn");
  var saveSpinner = document.getElementById("save-settings-spinner");
  var saveLabel = document.getElementById("save-settings-label");

  var testBtn = document.getElementById("test-connection-btn");
  var testSpinner = document.getElementById("test-spinner");
  var testLabel = document.getElementById("test-label");

  var statusBanner = document.getElementById("options-status");

  function setFieldError(inputEl, errorEl, errorId, message) {
    if (!message) {
      errorEl.textContent = "";
      errorEl.classList.add("hidden");
      inputEl.removeAttribute("aria-invalid");
      inputEl.removeAttribute("aria-describedby");
      return;
    }
    errorEl.textContent = message;
    errorEl.classList.remove("hidden");
    inputEl.setAttribute("aria-invalid", "true");
    inputEl.setAttribute("aria-describedby", errorId);
  }

  function clearStatus() {
    statusBanner.className = "banner hidden";
    statusBanner.replaceChildren();
    statusBanner.setAttribute("role", "status");
  }

  function showStatus(kind, title, detail) {
    statusBanner.replaceChildren();
    statusBanner.className = "banner banner-" + kind;
    statusBanner.setAttribute("role", kind === "error" ? "alert" : "status");

    var body = document.createElement("div");
    body.className = "banner-body";

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

    body.appendChild(textWrap);
    statusBanner.appendChild(body);
  }

  function validateVaultUrl() {
    var check = api.normalizeVaultUrl(vaultUrlInput.value);
    if (!check.ok) {
      setFieldError(vaultUrlInput, vaultUrlError, "vault-url-error", check.error);
      return null;
    }
    setFieldError(vaultUrlInput, vaultUrlError, "vault-url-error", "");
    return check;
  }

  function validateApiKey() {
    var trimmed = apiKeyInput.value.trim();
    if (!trimmed) {
      setFieldError(apiKeyInput, apiKeyError, "api-key-error", "API key is required.");
      return null;
    }
    setFieldError(apiKeyInput, apiKeyError, "api-key-error", "");
    return trimmed;
  }

  function ensureOriginPermission(originPattern) {
    return api.hasOriginPermission(originPattern).then(function (granted) {
      if (granted) {
        return true;
      }
      return api.requestOriginPermission(originPattern);
    });
  }

  function setBusy(target, busy) {
    saveBtn.disabled = busy;
    testBtn.disabled = busy;

    if (target === "save") {
      saveSpinner.classList.toggle("hidden", !busy);
      saveLabel.textContent = busy ? "Saving..." : "Save Settings";
    } else if (target === "test") {
      testSpinner.classList.toggle("hidden", !busy);
      testLabel.textContent = busy ? "Testing..." : "Test Connection";
    }
  }

  toggleKeyBtn.addEventListener("click", function () {
    var isHidden = apiKeyInput.type === "password";
    apiKeyInput.type = isHidden ? "text" : "password";
    toggleKeyBtn.textContent = isHidden ? "Hide" : "Show";
    toggleKeyBtn.setAttribute("aria-pressed", isHidden ? "true" : "false");
    toggleKeyBtn.setAttribute("aria-label", isHidden ? "Hide API key" : "Show API key");
  });

  resetUrlBtn.addEventListener("click", function () {
    vaultUrlInput.value = api.DEFAULT_VAULT_URL;
    validateVaultUrl();
    clearStatus();
  });

  vaultUrlInput.addEventListener("blur", validateVaultUrl);
  apiKeyInput.addEventListener("blur", function () {
    if (apiKeyInput.value.trim() !== "") {
      validateApiKey();
    }
  });

  form.addEventListener("submit", function (event) {
    event.preventDefault();
    clearStatus();

    var urlCheck = validateVaultUrl();
    var key = validateApiKey();
    if (!urlCheck) {
      vaultUrlInput.focus();
      return;
    }
    if (!key) {
      apiKeyInput.focus();
      return;
    }

    setBusy("save", true);
    ensureOriginPermission(urlCheck.originPattern)
      .then(function (granted) {
        return api
          .saveSettings({
            vaultUrl: urlCheck.url,
            apiKey: key
          })
          .then(function (saved) {
            vaultUrlInput.value = saved.vaultUrl;
            if (granted) {
              showStatus("success", "Settings saved", "Ready to save bookmarks to " + saved.vaultUrl + ".");
            } else {
              showStatus(
                "info",
                "Settings saved, permission pending",
                "Grant site access for " + saved.vaultUrl + " when saving from the popup."
              );
            }
          });
      })
      .catch(function (err) {
        showStatus("error", "Could not save settings", err.message || "Unexpected error.");
      })
      .finally(function () {
        setBusy("save", false);
      });
  });

  testBtn.addEventListener("click", function () {
    clearStatus();

    var urlCheck = validateVaultUrl();
    var key = validateApiKey();
    if (!urlCheck) {
      vaultUrlInput.focus();
      return;
    }
    if (!key) {
      apiKeyInput.focus();
      return;
    }

    setBusy("test", true);
    ensureOriginPermission(urlCheck.originPattern)
      .then(function (granted) {
        if (!granted) {
          return {
            ok: false,
            error: "Site permission was not granted for " + urlCheck.url + "."
          };
        }
        return api.testConnection({
          vaultUrl: urlCheck.url,
          apiKey: key
        });
      })
      .then(function (res) {
        if (res.ok) {
          showStatus("success", "Connection verified", res.message);
        } else {
          showStatus("error", "Connection failed", res.error);
        }
      })
      .finally(function () {
        setBusy("test", false);
      });
  });

  api.getSettings().then(function (settings) {
    vaultUrlInput.value = settings.vaultUrl || api.DEFAULT_VAULT_URL;
    apiKeyInput.value = settings.apiKey || "";
  });
})();
