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
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { EarningsRangePicker } from '../components/earnings-range-picker'
import { rangeForPreset, toEarningsQuery } from '../lib'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

// Presets are counted back from "today", so the clock must be pinned. Only
// Date is faked: user-event and the DOM still need real timers.
const NOW = new Date(2026, 10, 20, 15, 30)

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(NOW)
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

function renderPicker(range = rangeForPreset('30d', NOW), onChange = vi.fn()) {
  render(
    <I18nextProvider i18n={i18n}>
      <EarningsRangePicker range={range} onChange={onChange} />
    </I18nextProvider>
  )
  return onChange
}

it('marks the preset that the current window equals as pressed', () => {
  renderPicker(rangeForPreset('7d', NOW))

  expect(screen.getByRole('button', { name: 'Last 7 days' })).toHaveAttribute(
    'aria-pressed',
    'true'
  )
  expect(screen.getByRole('button', { name: 'Last 30 days' })).toHaveAttribute(
    'aria-pressed',
    'false'
  )
})

it('marks no preset as pressed for a custom window', () => {
  renderPicker({ start: new Date(2026, 10, 3), end: new Date(2026, 10, 9) })

  for (const name of [
    'Last 7 days',
    'Last 30 days',
    'Current month',
    'Last month',
  ]) {
    expect(screen.getByRole('button', { name })).toHaveAttribute(
      'aria-pressed',
      'false'
    )
  }
})

it('reports the window of a preset when it is chosen', async () => {
  const onChange = renderPicker(rangeForPreset('30d', NOW))

  await userEvent
    .setup({ advanceTimers: () => {} })
    .click(screen.getByRole('button', { name: 'Last 7 days' }))

  expect(onChange).toHaveBeenCalledTimes(1)
  expect(toEarningsQuery(onChange.mock.calls[0][0])).toEqual({
    start_date: '2026-11-14',
    end_date: '2026-11-20',
  })
})

it('warns when the start date is later than the end date', () => {
  renderPicker({ start: new Date(2026, 10, 9), end: new Date(2026, 10, 3) })

  expect(screen.getByRole('alert')).toHaveTextContent(
    'The start date must not be later than the end date'
  )
})

it('shows no warning for a valid window', () => {
  renderPicker()

  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
})

it('shows the chosen days on the date pickers', () => {
  renderPicker({ start: new Date(2026, 10, 3), end: new Date(2026, 10, 9) })

  expect(screen.getByRole('button', { name: '2026-11-03' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: '2026-11-09' })).toBeInTheDocument()
})
