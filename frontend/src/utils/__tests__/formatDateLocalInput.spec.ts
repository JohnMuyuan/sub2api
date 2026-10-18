import { afterEach, describe, expect, it } from 'vitest'

import { formatDateLocalInput } from '../format'
import { setSiteTimeZone } from '../siteTimezone'
afterEach(() => setSiteTimeZone())

describe('formatDateLocalInput', () => {
  it('formats the calendar date in the site timezone', () => {
    const localDate = new Date('2026-07-12T16:30:00Z')
    setSiteTimeZone('Asia/Taipei')

    expect(formatDateLocalInput(localDate)).toBe('2026-07-13')
  })

  it('returns an empty string for an invalid date', () => {
    expect(formatDateLocalInput(new Date('invalid'))).toBe('')
  })
})
