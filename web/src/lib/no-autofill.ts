/**
 * Antennarr has no logins, passwords or payment forms, so a password manager or the
 * browser offering to fill a field is only in the way. `autocomplete="off"` is
 * ignored by 1Password, LastPass and Bitwarden, which each have an attribute of their own.
 * The shared Input and Textarea set these first, so a field can still override them.
 */
export const noAutofill = {
  autoComplete: 'off',
  'data-1p-ignore': true,
  'data-lpignore': 'true',
  'data-bwignore': true,
  'data-form-type': 'other',
} as const
