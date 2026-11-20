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
import type { StatusVariant } from '@/components/status-badge'

import type { EarningsCostSource } from './types'

/** Rows per page of the per-channel and per-model reports. */
export const EARNINGS_PAGE_SIZE = 10

/** Labels are i18n keys; render them with t(). */
export const EARNINGS_RANGE_PRESETS = [
  { value: '7d', labelKey: 'Last 7 days' },
  { value: '30d', labelKey: 'Last 30 days' },
  { value: 'month', labelKey: 'Current month' },
  { value: 'lastMonth', labelKey: 'Last month' },
] as const

export type EarningsRangePreset =
  (typeof EARNINGS_RANGE_PRESETS)[number]['value']

export const EARNINGS_DEFAULT_PRESET: EarningsRangePreset = '30d'

/** Labels and descriptions are i18n keys; render them with t(). */
export const EARNINGS_COST_SOURCE_CONFIG: Record<
  EarningsCostSource,
  { labelKey: string; descriptionKey: string; variant: StatusVariant }
> = {
  recorded: {
    labelKey: 'Recorded',
    descriptionKey:
      'Cost recorded from the channel purchase ratio when each request was logged.',
    variant: 'success',
  },
  estimated: {
    labelKey: 'Estimated',
    descriptionKey:
      'No purchase ratio was recorded, so the cost is estimated from the charge.',
    variant: 'warning',
  },
  mixed: {
    labelKey: 'Mixed',
    descriptionKey:
      'Part of the cost was recorded from purchase ratios, the rest is estimated from the charge.',
    variant: 'info',
  },
}
