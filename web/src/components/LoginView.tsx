import { useState, type FormEvent } from "react";
import { Eye, EyeOff, Lock, Sun, Moon } from "lucide-react";
import { api, ApiError } from "../api";

interface LoginViewProps {
  passwordConfigured: boolean;
  theme: "dark" | "light";
  onToggleTheme: () => void;
  onSuccess: () => void;
}

export function LoginView({ passwordConfigured, theme, onToggleTheme, onSuccess }: LoginViewProps) {
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!password.trim()) {
      setError("Enter your vault password.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await api.login(password);
      onSuccess();
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Unable to reach the server.");
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-card" aria-labelledby="login-heading">
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <div className="brand-mark">
            <span className="brand-badge" aria-hidden="true">
              <Lock size={16} />
            </span>
            <span>Social Vault</span>
          </div>
          <button
            type="button"
            className="btn btn-ghost btn-icon"
            onClick={onToggleTheme}
            aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
          >
            {theme === "dark" ? <Sun size={17} aria-hidden="true" /> : <Moon size={17} aria-hidden="true" />}
          </button>
        </div>

        <div>
          <h1 id="login-heading" style={{ margin: 0, fontSize: 22, fontWeight: 700, letterSpacing: "-0.02em" }}>
            Unlock your vault
          </h1>
          <p style={{ margin: "6px 0 0", fontSize: 14, color: "var(--fg-muted)" }}>
            Private bookmark archive for harvested threads, articles, and references.
          </p>
        </div>

        {!passwordConfigured && (
          <div
            role="alert"
            style={{
              padding: "10px 12px",
              borderRadius: "var(--radius-md)",
              backgroundColor: "var(--destructive-soft)",
              color: "var(--destructive-text)",
              fontSize: 13,
            }}
          >
            <code>VAULT_PASSWORD</code> is not set in the server environment.
          </div>
        )}

        <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <div className="field">
            <label className="field-label" htmlFor="vault-password">
              Password
            </label>
            <div style={{ position: "relative" }}>
              <input
                id="vault-password"
                type={showPassword ? "text" : "password"}
                className="input"
                style={{ paddingRight: 44 }}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                autoFocus
                placeholder="Enter VAULT_PASSWORD"
                aria-invalid={error ? "true" : "false"}
              />
              <button
                type="button"
                className="btn btn-ghost btn-icon"
                onClick={() => setShowPassword((prev) => !prev)}
                aria-label={showPassword ? "Hide password" : "Show password"}
                style={{ position: "absolute", right: 2, top: 1 }}
              >
                {showPassword ? <EyeOff size={16} aria-hidden="true" /> : <Eye size={16} aria-hidden="true" />}
              </button>
            </div>
            {error && (
              <span className="field-error" role="alert">
                {error}
              </span>
            )}
          </div>

          <button type="submit" className="btn btn-primary" disabled={submitting || !passwordConfigured}>
            {submitting ? "Unlocking..." : "Sign in"}
          </button>
        </form>
      </section>
    </main>
  );
}
