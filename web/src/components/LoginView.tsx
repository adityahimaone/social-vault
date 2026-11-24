import { useState, type FormEvent } from "react";
import { Eye, EyeOff, Lock, Moon, Sun } from "lucide-react";
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
        <div className="login-header">
          <div className="brand-mark">
            <span className="brand-badge" aria-hidden="true">
              <Lock size={15} />
            </span>
            <div className="brand-copy">
              <span className="brand-name">Social Vault</span>
              <span className="brand-sub">Private Archive</span>
            </div>
          </div>
          <button
            type="button"
            className="btn btn-secondary btn-icon"
            onClick={onToggleTheme}
            aria-label={theme === "dark" ? "Switch to Paper appearance" : "Switch to Dark appearance"}
          >
            {theme === "dark" ? <Sun size={16} aria-hidden="true" /> : <Moon size={16} aria-hidden="true" />}
          </button>
        </div>

        <div className="login-intro">
          <h1 id="login-heading" className="login-title">
            Unlock your vault
          </h1>
          <p className="login-subtitle">
            Personal archive for harvested threads, articles, and references.
          </p>
        </div>

        {!passwordConfigured && (
          <div className="alert-banner" role="alert">
            <code>VAULT_PASSWORD</code> is not set in the server environment.
          </div>
        )}

        <form onSubmit={handleSubmit} className="login-form">
          <div className="field">
            <label className="field-label" htmlFor="vault-password">
              Password
            </label>
            <div className="password-wrap">
              <input
                id="vault-password"
                type={showPassword ? "text" : "password"}
                className="input password-input"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                autoFocus
                placeholder="Enter VAULT_PASSWORD"
                aria-invalid={error ? "true" : "false"}
              />
              <button
                type="button"
                className="btn btn-ghost btn-icon password-toggle-btn"
                onClick={() => setShowPassword((prev) => !prev)}
                aria-label={showPassword ? "Hide password" : "Show password"}
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

          <button type="submit" className="btn btn-primary btn-block" disabled={submitting || !passwordConfigured}>
            {submitting ? "Unlocking..." : "Sign in"}
          </button>
        </form>
      </section>
    </main>
  );
}
