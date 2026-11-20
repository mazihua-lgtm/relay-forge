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
/** The most a commission rate can be: the whole of what an invitee spends. */
export const COMMISSION_MAX_PERCENT = 100

/**
 * The server stores the rate as a fraction (0.1 is 10%); people think in
 * percent. Rounding to two decimals of a percent keeps 0.29 shown as 29, not
 * as 28.999999999999996.
 */
export function commissionRateToPercent(rate: number | undefined): number {
  if (rate === undefined || !Number.isFinite(rate)) return 0
  return Math.round(rate * 10000) / 100
}

/**
 * The fraction to send for a percent the operator typed, or null when it is
 * not a number from 0 to 100. Two decimals of a percent are kept.
 */
export function percentToCommissionRate(percent: number): number | null {
  if (!Number.isFinite(percent)) return null
  if (percent < 0 || percent > COMMISSION_MAX_PERCENT) return null
  return Math.round(percent * 100) / 10000
}

/** The text of a percent input; an empty or non-numeric text is not valid. */
export function parsePercentText(text: string): number | null {
  if (text.trim() === '') return null
  return percentToCommissionRate(Number(text))
}
