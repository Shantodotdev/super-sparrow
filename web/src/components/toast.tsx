import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

const toastDuration = 3000;

type ToastContextValue = { notify: (message: string) => void };

const ToastContext = createContext<ToastContextValue | null>(null);

/** Shows short status messages in the corner of the screen for everything below it. */
export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [message, setMessage] = useState("");
  const timer = useRef<number | undefined>(undefined);

  const notify = useCallback((next: string) => {
    window.clearTimeout(timer.current);
    setMessage(next);
    timer.current = window.setTimeout(() => setMessage(""), toastDuration);
  }, []);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  const value = useMemo(() => ({ notify }), [notify]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      {message && (
        <div className="toast" role="status">
          {message}
        </div>
      )}
    </ToastContext.Provider>
  );
}

/** Returns the function that shows a toast message. */
export function useToast(): ToastContextValue {
  const context = useContext(ToastContext);
  if (!context) throw new Error("useToast requires ToastProvider");

  return context;
}
