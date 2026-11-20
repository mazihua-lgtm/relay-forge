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
import { describe, expect, test } from 'vitest'

import {
  commissionRateToPercent,
  parsePercentText,
  percentToCommissionRate,
} from '../commission'

describe('commissionRateToPercent', () => {
  test.each([
    [0, 0],
    [0.1, 10],
    [1, 100],
    [0.0001, 0.01],
    [0.1234, 12.34],
  ])('shows the fraction %s as %s percent', (rate, percent) => {
    expect(commissionRateToPercent(rate)).toBe(percent)
  })

  test('hides floating point noise', () => {
    // 0.29 * 100 is 28.999999999999996 in floating point.
    expect(commissionRateToPercent(0.29)).toBe(29)
  })

  test('keeps two decimals of a percent', () => {
    expect(commissionRateToPercent(0.123456)).toBe(12.35)
  })

  test.each([undefined, Number.NaN, Number.POSITIVE_INFINITY])(
    'treats %s as no commission',
    (rate) => {
      expect(commissionRateToPercent(rate)).toBe(0)
    }
  )
})

describe('percentToCommissionRate', () => {
  test.each([
    [0, 0],
    [10, 0.1],
    [29, 0.29],
    [100, 1],
    [12.34, 0.1234],
    [33.3333, 0.3333],
  ])('sends %s percent as the fraction %s', (percent, rate) => {
    expect(percentToCommissionRate(percent)).toBe(rate)
  })

  test.each([-0.01, -1, 100.01, 1000, Number.NaN, Number.POSITIVE_INFINITY])(
    'refuses %s',
    (percent) => {
      expect(percentToCommissionRate(percent)).toBeNull()
    }
  )

  test('is the inverse of the display for what the input can hold', () => {
    for (const percent of [0, 0.01, 5, 12.5, 29, 33.33, 99.99, 100]) {
      expect(
        commissionRateToPercent(percentToCommissionRate(percent) ?? Number.NaN)
      ).toBe(percent)
    }
  })
})

describe('parsePercentText', () => {
  test.each([
    ['0', 0],
    ['12.5', 0.125],
    ['100', 1],
    [' 7 ', 0.07],
  ])('reads %j as the fraction %s', (text, rate) => {
    expect(parsePercentText(text)).toBe(rate)
  })

  test.each(['', '   ', 'abc', '12abc', '-1', '101', 'NaN', 'Infinity'])(
    'refuses %j',
    (text) => {
      expect(parsePercentText(text)).toBeNull()
    }
  )
})
