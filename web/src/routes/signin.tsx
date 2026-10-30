import { apiWrite } from "../lib/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useNavigate, createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/signin")({
  staticData: { bare: true },
  component: SignInPage,
});

/** Signs in with a server-verified emailed code. */
function SignInPage() {
  const navigate = useNavigate();
  const client = useQueryClient();
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [sent, setSent] = useState(false);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (pending) return;
    setPending(true);
    setError("");
    try {
      if (sent) {
        await apiWrite("/api/auth/verify", "POST", { email, code });
        client.clear();
        await navigate({ to: "/issues" });
      } else {
        await apiWrite("/api/auth/code", "POST", { email });
        setSent(true);
      }
    } catch (error) {
      setError(error instanceof Error ? error.message : "Sign-in failed");
    } finally {
      setPending(false);
    }
  };

  const resetEmail = () => {
    setSent(false);
    setCode("");
  };

  return (
    <div className="grid min-h-screen place-items-center">
      <div className="absolute top-[27px] left-[32px] flex items-center gap-[12px] text-[17px] font-[650]">
        <img src="/logo.png" className="h-[40px] w-[40px] flex-none" alt="" /> Super Sparrow
      </div>
      <form className="w-[min(100%-40px,390px)]" onSubmit={submit}>
        <img src="/logo.png" className="mb-[22px] h-[48px] w-[48px]" alt="" />
        <h1 className="text-[27px]">{sent ? "Check your email" : "Welcome to Super Sparrow"}</h1>
        <p className="muted mt-[10px] mb-[25px] leading-normal text-[#a2aec2]">
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
        <button
          disabled={pending}
          className="w-full rounded-full border border-[#3264ff] bg-[#2455eb] px-[15px] py-[10px] font-[550] text-white hover:bg-[#3566f6]"
        >
          {sent ? "Sign in" : "Continue with email"}
        </button>
        {sent && (
          <button
            type="button"
            className="text-button mx-auto my-[16px] block border-0 bg-none p-[4px] text-[12px] text-[#aebbd0] hover:text-white"
            onClick={resetEmail}
          >
            Use another email
          </button>
        )}
        <p className="muted mt-[10px] mb-[25px] text-[12px] leading-normal text-[#a2aec2]">
          {error && <span role="alert">{error}</span>}
        </p>
      </form>
    </div>
  );
}
