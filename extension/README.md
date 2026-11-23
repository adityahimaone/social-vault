# Social Vault Browser Extension (MV3)

Lightweight Manifest V3 extension for Brave and Chrome to save the active page or any link directly to Social Vault with one click — no build step required.

## Install (Unpacked)

1. Open `brave://extensions` (Brave) or `chrome://extensions` (Chrome).
2. Enable **Developer mode** (top-right toggle).
3. Click **Load unpacked** and select the `extension/` directory inside this repository.
4. (Optional) Pin **Social Vault** to your toolbar.

## Configure

1. Click the **Social Vault** toolbar icon and click the **Settings** gear button (or right-click the extension icon → **Options**).
2. Set **Vault URL** (defaults to `https://vault.adityahimaone.space`).
3. Paste your `VAULT_API_KEY`.
4. Click **Test Connection** to verify authentication, then **Save Settings**.

Your Vault URL and API key are stored exclusively in `chrome.storage.local` on that browser profile (never `chrome.storage.sync`, and never sent anywhere other than the configured Vault URL).

## Features

- **Toolbar Popup**: Auto-fills the current tab's URL and title, lets you add comma-separated tags (with live chip preview) and optional notes, and submits with `Save to Vault` or `⌘/Ctrl + Enter`.
- **Context Menu**: Right-click any page (**Save page to Social Vault**) or hyperlink (**Save link to Social Vault**) to save immediately in the background.
- **Badge Feedback**: Briefly displays `✓` (saved), `•` (already saved), or `!` (error) on the extension icon after saving.

## Known Limitations

- **Internal browser pages**: Pages such as `chrome://`, `brave://`, `edge://`, or `about:` cannot be bookmarked directly; the popup lets you paste a valid `http://` or `https://` URL manually.
- **Custom Vault URLs**: The manifest pre-grants access to `https://vault.adityahimaone.space/*`. If you point the extension to another domain or local `http://127.0.0.1:8787`, the browser will ask for site access permission when you save settings or test the connection.
- **Duplicate links**: If a URL already exists in Social Vault (`HTTP 409`), the extension reports that it is already saved rather than overwriting existing notes/tags.
- **API key required**: Uses `Authorization: Bearer <key>` rather than browser session cookies.
