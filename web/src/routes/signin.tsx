import { CodeInput, emptyCode } from "../components/code-input";
import { Input } from "../components/input";
import { apiWrite } from "../lib/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useNavigate, createFileRoute } from "@tanstack/react-router";
import { clearQueryCache } from "../queries/persistence";

export const Route = createFileRoute("/signin")({
  staticData: { bare: true },
  pendingComponent: () => null,
  component: SignInPage,
});

/** Signs in with a server-verified emailed code. */
function SignInPage() {
  const navigate = useNavigate();
  const client = useQueryClient();
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [email, setEmail] = useState("");
  const [digits, setDigits] = useState(emptyCode);
  const [sent, setSent] = useState(false);

  const run = async (action: () => Promise<void>) => {
    if (pending) return;
    setPending(true);
    setError("");
    try {
      await action();
    } catch (error) {
      setError(error instanceof Error ? error.message : "Sign-in failed");
    } finally {
      setPending(false);
    }
  };

  const requestCode = () =>
    run(async () => {
      await apiWrite("/api/auth/code", "POST", { email });
      setSent(true);
    });

  const verify = (code: string) =>
    run(async () => {
      try {
        await apiWrite("/api/auth/verify", "POST", { email, code });
      } catch (error) {
        setDigits(emptyCode());
        throw error;
      }
      clearQueryCache(client);
      await navigate({ to: "/" });
    });

  // The code is verified as soon as its last digit is entered, so no button is needed.
  const changeDigits = (next: string[]) => {
    setDigits(next);
    if (next.every(Boolean)) void verify(next.join(""));
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!sent) void requestCode();
  };

  const resetEmail = () => {
    setSent(false);
    setDigits(emptyCode());
  };

  return (
    <div className="grid min-h-screen place-items-center">
      <form className="w-[min(100%-40px,360px)]" onSubmit={submit}>
        <img src="/logo.png" className="mb-[24px] h-[80px] w-[80px]" alt="" />
        <h1 className="text-[26px] font-[650]">{sent ? "Enter your code" : "Sign in"}</h1>
        <div className="mt-8">
          {sent ? (
            <CodeInput digits={digits} onChange={changeDigits} disabled={pending} />
          ) : (
            <label>
              Email
              <Input
                type="email"
                required
                autoComplete="email"
                autoFocus
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="you@example.com"
              />
            </label>
          )}
        </div>
        {!sent && (
          <button
            disabled={pending}
            className="mt-[16px] h-[46px] w-full rounded-xl bg-[#2455eb] font-[550] text-white transition-colors hover:bg-[#3566f6] disabled:opacity-50"
          >
            Continue
          </button>
        )}
        {sent && (
          <button
            type="button"
            className="text-button mt-[18px] block border-0 bg-none p-[4px] text-[13px] text-[#aebbd0] underline underline-offset-4 hover:text-white"
            onClick={resetEmail}
          >
            Use another email
          </button>
        )}
        {error && (
          <p role="alert" className="mt-[16px] text-[13px] text-[#f38b8b]">
            {error}
          </p>
        )}
      </form>
    </div>
  );
}
