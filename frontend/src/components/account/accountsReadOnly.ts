import type { InjectionKey } from 'vue'

// Provided by the account page when the viewer role may only read accounts.
export const accountsReadOnlyKey: InjectionKey<boolean> = Symbol('accountsReadOnly')
