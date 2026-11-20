/*
Copyright (C) 2026 relay-forge contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { describe, expect, it } from 'vitest'

import {
  isValidRange,
  matchPreset,
  rangeForPreset,
  toApiDate,
  toEarningsQuery,
} from '../lib'

// Friday 20 November 2026, mid-afternoon: the time of day must not matter.
const NOW = new Date(2026, 10, 20, 15, 30)

function windowOf(
  preset: Parameters<typeof rangeForPreset>[0],
  now: Date = NOW
) {
  return toEarningsQuery(rangeForPreset(preset, now))
}

describe('rangeForPreset', () => {
  it('spans exactly 7 calendar days for the last 7 days, today included', () => {
    expect(windowOf('7d')).toEqual({
      start_date: '2026-11-14',
      end_date: '2026-11-20',
    })
  })

  it('spans exactly 30 calendar days for the last 30 days, today included', () => {
    expect(windowOf('30d')).toEqual({
      start_date: '2026-10-22',
      end_date: '2026-11-20',
    })
  })

  it('runs from the first of the month to today for this month', () => {
    expect(windowOf('month')).toEqual({
      start_date: '2026-11-01',
      end_date: '2026-11-20',
    })
  })

  it('covers the whole previous month for last month', () => {
    expect(windowOf('lastMonth')).toEqual({
      start_date: '2026-10-01',
      end_date: '2026-10-31',
    })
  })

  it('crosses the year boundary for last month in January', () => {
    expect(windowOf('lastMonth', new Date(2026, 0, 10))).toEqual({
      start_date: '2025-12-01',
      end_date: '2025-12-31',
    })
  })

  it('ends on the 29th for last month in March of a leap year', () => {
    expect(windowOf('lastMonth', new Date(2024, 2, 5))).toEqual({
      start_date: '2024-02-01',
      end_date: '2024-02-29',
    })
  })

  it('stays on one day for this month on the first of the month', () => {
    expect(windowOf('month', new Date(2026, 10, 1, 0, 5))).toEqual({
      start_date: '2026-11-01',
      end_date: '2026-11-01',
    })
  })
})

describe('toApiDate', () => {
  it('formats the local calendar day, not the UTC day', () => {
    // Just after local midnight is still the previous day in UTC for any
    // timezone east of Greenwich; the report API takes local days.
    expect(toApiDate(new Date(2026, 10, 5, 0, 1))).toBe('2026-11-05')
    expect(toApiDate(new Date(2026, 10, 5, 23, 59))).toBe('2026-11-05')
  })
})

describe('isValidRange', () => {
  it('accepts a single-day window', () => {
    const day = new Date(2026, 10, 5)
    expect(isValidRange({ start: day, end: day })).toBe(true)
  })

  it('accepts a start earlier than the end', () => {
    expect(
      isValidRange({ start: new Date(2026, 10, 1), end: new Date(2026, 10, 5) })
    ).toBe(true)
  })

  it('rejects a start later than the end', () => {
    expect(
      isValidRange({ start: new Date(2026, 10, 6), end: new Date(2026, 10, 5) })
    ).toBe(false)
  })

  it('compares days, so the same day at different times is valid', () => {
    expect(
      isValidRange({
        start: new Date(2026, 10, 5, 23, 0),
        end: new Date(2026, 10, 5, 1, 0),
      })
    ).toBe(true)
  })
})

describe('matchPreset', () => {
  it('recognizes each preset window', () => {
    expect(matchPreset(rangeForPreset('7d', NOW), NOW)).toBe('7d')
    expect(matchPreset(rangeForPreset('30d', NOW), NOW)).toBe('30d')
    expect(matchPreset(rangeForPreset('month', NOW), NOW)).toBe('month')
    expect(matchPreset(rangeForPreset('lastMonth', NOW), NOW)).toBe('lastMonth')
  })

  it('returns null for a custom window', () => {
    expect(
      matchPreset(
        { start: new Date(2026, 10, 3), end: new Date(2026, 10, 9) },
        NOW
      )
    ).toBeNull()
  })

  it('returns null when only one end of the window matches a preset', () => {
    const sevenDays = rangeForPreset('7d', NOW)
    expect(
      matchPreset({ start: new Date(2026, 10, 15), end: sevenDays.end }, NOW)
    ).toBeNull()
  })
})
