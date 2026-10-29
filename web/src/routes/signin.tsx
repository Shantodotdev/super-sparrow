import { useState, type FormEvent } from "react";
import { useNavigate, createFileRoute } from "@tanstack/react-router";
import { Icon } from "../components/icon";

export const Route = createFileRoute("/signin")({
  staticData: { bare: true },
  component: SignInPage,
});

/** Signs in with an emailed code. In this preview no email is sent and any code is accepted. */
function SignInPage() {
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [sent, setSent] = useState(false);

  const submit = (event: FormEvent) => {
    event.preventDefault();

    if (sent) void navigate({ to: "/issues" });
    else setSent(true);
  };

  const resetEmail = () => {
    setSent(false);
    setCode("");
  };

  return (
    <div className="signin-page">
      <div className="signin-brand">
        <Icon name="bird" className="brand-mark" /> Super Sparrow
      </div>
      <form className="signin-card" onSubmit={submit}>
        <Icon name="bird" className="signin-icon" />
        <h1>{sent ? "Check your email" : "Welcome to Super Sparrow"}</h1>
        <p className="muted">
          {sent
            ? "If this email belongs to a teammate, a sign-in code is on its way."
            : "Sign in with your team email to continue."}
        </p>
        <label>
          Email
          <input
            type="email"
            required
            autoComplete="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            disabled={sent}
            placeholder="you@example.com"
          />
        </label>
        {sent && (
          <label>
            Sign-in code
            <input
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              required
              value={code}
              onChange={(event) => setCode(event.target.value)}
              placeholder="000000"
            />
          </label>
        )}
        <button className="primary full">{sent ? "Sign in" : "Continue with email"}</button>
        {sent && (
          <button type="button" className="text-button centered" onClick={resetEmail}>
            Use another email
          </button>
        )}
        <p className="muted small">Frontend preview only. No email is sent.</p>
      </form>
    </div>
  );
}
