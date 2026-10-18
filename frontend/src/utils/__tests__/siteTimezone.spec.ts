import { afterEach, describe, expect, it } from 'vitest'
import { setSiteTimeZone, getSiteTimeZone, formatSiteDateInput, siteCalendarDate } from '../siteTimezone'
import { formatDateTime } from '../format'

afterEach(() => { setSiteTimeZone(); delete window.__APP_CONFIG__ })
describe('effective site timezone', () => {
  it('aligns dates and timestamps with Taipei even when the browser is elsewhere', () => {
    setSiteTimeZone('Asia/Taipei')
    const instant = new Date('2026-10-18T16:30:00Z')
    expect(getSiteTimeZone()).toBe('Asia/Taipei')
    expect(formatSiteDateInput(instant)).toBe('2026-10-19')
    expect(siteCalendarDate(instant).toISOString()).toBe('2026-10-19T00:00:00.000Z')
    expect(formatDateTime(instant, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }, 'en-GB')).toBe('00:30')
  })
  it('handles the previous day and daylight saving changes', () => {
    setSiteTimeZone('America/New_York')
    expect(formatSiteDateInput(new Date('2026-03-08T04:30:00Z'))).toBe('2026-03-07')
    expect(formatDateTime('2026-03-08T06:30:00Z', { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }, 'en-GB')).toBe('01:30')
    expect(formatDateTime('2026-03-08T07:30:00Z', { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }, 'en-GB')).toBe('03:30')
    expect(formatSiteDateInput(new Date('invalid'))).toBe('')
    expect(formatDateTime('2026-03-08', { year: 'numeric', month: '2-digit', day: '2-digit' }, 'en-GB')).toBe('08/03/2026')
  })
  it('ignores invalid server clocks safely', () => {
    setSiteTimeZone('Invalid/Zone')
    expect(() => formatSiteDateInput(new Date())).not.toThrow()
  })
})
