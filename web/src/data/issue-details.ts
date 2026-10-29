/** Placeholder content shown on every issue that has no plan of its own. */
export const sampleIssueDetails = {
  branch: "feat/sp-142-email-code",
  commentCount: 8,
  pullRequest: { number: 324, title: "Add email code sign-in" },
  summary: "Adds sign-in with email code, branch creation flow, and related UI updates.",
  plan: {
    goal: "Allow users to sign in with a one-time code sent to their email address. Provide a seamless, secure flow with clear messaging and handle edge cases gracefully.",
    approach: [
      "Add backend endpoint to request and verify email codes.",
      "Implement UI for requesting a code and entering the code.",
      "Integrate the authentication flow with session management.",
      "Update related loading, error, and success states.",
    ],
    edgeCases: [
      "Expired code — show a clear error and allow requesting a new code.",
      "Too many requests — rate limit with a helpful message.",
      "Network errors — offer a retry option.",
    ],
    checklist: [
      { text: "Backend endpoints (request + verify code)", done: true },
      { text: "Email code UI (request, input, states)", done: true },
      { text: "Integrate with session management", done: false },
      { text: "Write tests", done: false },
      { text: "Update documentation", done: false },
    ],
  },
};
