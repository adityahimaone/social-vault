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
- **Bulk Fetch (X & Threads)**: Injects a compact bottom-right floating panel on supported X and Threads collection pages so you can collect and save visible (and auto-scrolled) posts in one session.

## Bulk Fetch (X & Threads)

### Supported Pages
- **X / Twitter**: `https://x.com/i/bookmarks`, `https://x.com/<username>/likes` (and matching `twitter.com` URLs).
- **Threads**: `https://www.threads.com/saved`, `https://www.threads.com/liked` (plus `threads.net` and `/@<username>/(saved|liked|likes)` variants if routed there).
- On any other X or Threads page, no panel is rendered.

### How It Works
1. Navigate to one of the supported pages above while signed in to X or Threads.
2. The **Social Vault** floating panel appears in the bottom-right corner with a live count of detected posts.
3. Keep **Auto-scroll to load more** enabled (default **ON**) to automatically scroll down and collect posts before saving, or turn it **OFF** to save only what is currently loaded.
4. Click **Save all to Vault** to start. Nothing is auto-saved before you click this button.
5. You can click **Stop** at any time during scrolling or saving.
6. The extension saves each deduplicated post URL sequentially via `POST {VAULT_URL}/api/bookmarks` (with a ~150 ms pause between requests, empty title/notes so your Social Vault server fetches metadata) and shows live counts for **Saved**, **Skipped** (duplicates / HTTP 409), and **Failed**.

### Limits & Known Risks
- **Session cap**: Up to **200 posts per bulk session** (`MAX_POSTS_PER_SESSION = 200`).
- **Threads DOM & route fragility**: Threads uses obfuscated CSS class names and serves `/saved` and `/liked` behind authenticated views (with `threads.net` redirecting to `threads.com`). The extractor avoids class-name selectors and tries multiple post link patterns (`/@user/post/<id>`, `/user/post/<id>`, `/t/<id>`, `/post/<id>`), but Meta may change URL structures or route paths without notice.
- **X virtualized feed & markup changes**: X unmounts older `<article>` nodes as you scroll and may change `data-testid` or permalink markup. The collector merges URLs incrementally during scroll and skips deleted/unavailable tombstones heuristically.
- **Metadata fetching on auth-walled posts**: Because `title` and `notes` are left empty for the server to crawl, posts that require login or block server-side metadata scraping may be saved with fallback URL titles.
- **Custom Vault URLs**: If you use a local or custom Vault URL (not the default `https://vault.adityahimaone.space`), configure it and grant site permission in **Options** first.
- **Live browser verification**: Automated checks verify extraction on HTML fixtures, JS syntax, and local API saving; live testing in Brave against real authenticated X and Threads sessions is performed separately.

## Known Limitations

- **Internal browser pages**: Pages such as `chrome://`, `brave://`, `edge://`, or `about:` cannot be bookmarked directly; the popup lets you paste a valid `http://` or `https://` URL manually.
- **Custom Vault URLs**: The manifest pre-grants access to `https://vault.adityahimaone.space/*`. If you point the extension to another domain or local `http://127.0.0.1:8787`, the browser will ask for site access permission when you save settings or test the connection.
- **Duplicate links**: If a URL already exists in Social Vault (`HTTP 409`), the extension reports that it is already saved rather than overwriting existing notes/tags.
- **API key required**: Uses `Authorization: Bearer <key>` rather than browser session cookies.
