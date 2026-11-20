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
import dayjs from '@/lib/dayjs'

import { EARNINGS_RANGE_PRESETS, type EarningsRangePreset } from '../constants'
import type { EarningsQuery } from '../types'

/** A reporting window in whole days. Both ends are local midnights. */
export interface EarningsRange {
  start: Date
  end: Date
}

const API_DATE_FORMAT = 'YYYY-MM-DD'

/** The dates the report API takes: local calendar days, the end day included. */
export function toApiDate(date: Date): string {
  return dayjs(date).format(API_DATE_FORMAT)
}

export function toEarningsQuery(range: EarningsRange): EarningsQuery {
  return {
    start_date: toApiDate(range.start),
    end_date: toApiDate(range.end),
  }
}

/** A window whose start is after its end is rejected by the API. */
export function isValidRange(range: EarningsRange): boolean {
  return !dayjs(range.start).isAfter(range.end, 'day')
}

/**
 * The whole-day window of a quick preset, counted back from today. The last 7
 * and 30 days include today, so they span exactly 7 and 30 calendar days.
 */
export function rangeForPreset(
  preset: EarningsRangePreset,
  now: Date = new Date()
): EarningsRange {
  const today = dayjs(now).startOf('day')
  switch (preset) {
    case '7d':
      return { start: today.subtract(6, 'day').toDate(), end: today.toDate() }
    case '30d':
      return { start: today.subtract(29, 'day').toDate(), end: today.toDate() }
    case 'month':
      return { start: today.startOf('month').toDate(), end: today.toDate() }
    case 'lastMonth': {
      const firstOfThisMonth = today.startOf('month')
      return {
        start: firstOfThisMonth.subtract(1, 'month').toDate(),
        end: firstOfThisMonth.subtract(1, 'day').toDate(),
      }
    }
  }
}

/** The preset a window equals, or null for a custom window. */
export function matchPreset(
  range: EarningsRange,
  now: Date = new Date()
): EarningsRangePreset | null {
  const query = toEarningsQuery(range)
  for (const preset of EARNINGS_RANGE_PRESETS) {
    const candidate = toEarningsQuery(rangeForPreset(preset.value, now))
    if (
      candidate.start_date === query.start_date &&
      candidate.end_date === query.end_date
    ) {
      return preset.value
    }
  }
  return null
}
