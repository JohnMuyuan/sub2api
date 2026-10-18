export function resolveCompletedSetupRedirectPath(
  isAuthenticated: boolean,
  isAdmin: boolean,
  canViewAccounts = false
): string {
  if (!isAuthenticated) {
    return '/login'
  }

  if (isAdmin) return '/admin/dashboard'
  return canViewAccounts ? '/admin/accounts' : '/dashboard'
}
