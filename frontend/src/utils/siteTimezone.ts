import { ref } from 'vue'

// The effective server clock is public; a saved, pending timezone must never
// change query boundaries before the server has restarted.
const siteTimeZone = ref<string | undefined>()

export function setSiteTimeZone(value?: string): void {
  try {
    if (value) new Intl.DateTimeFormat('en', { timeZone: value }).format(0)
    siteTimeZone.value = value || undefined
  } catch { siteTimeZone.value = undefined }
}

export function getSiteTimeZone(): string {
  if (siteTimeZone.value) return siteTimeZone.value
  const value = window.__APP_CONFIG__?.server_timezone
  if (value) {
    try { setSiteTimeZone(value); if (siteTimeZone.value) return siteTimeZone.value } catch { /* Old servers may report Local. */ }
  }
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
}

export function formatSiteDateInput(date: Date): string {
  if (Number.isNaN(date.getTime())) return ''
  const parts = new Intl.DateTimeFormat('en', {
    timeZone: getSiteTimeZone(), year: 'numeric', month: '2-digit', day: '2-digit'
  }).formatToParts(date)
  const get = (key: string) => parts.find(part => part.type === key)?.value
  return `${get('year')}-${get('month')}-${get('day')}`
}

// A UTC calendar carrier for date-only arithmetic. It is not a real instant in
// the site timezone; UTC setters keep preset dates stable across browser DST.
export function siteCalendarDate(date = new Date()): Date {
  return new Date(`${formatSiteDateInput(date)}T00:00:00Z`)
}
