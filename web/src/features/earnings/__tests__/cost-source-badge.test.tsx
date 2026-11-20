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
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, expect, it } from 'vitest'

import { CostSourceBadge } from '../components/cost-source-badge'
import type { EarningsCostSource } from '../types'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

afterEach(() => {
  cleanup()
})

it.each<[EarningsCostSource, string]>([
  ['recorded', 'Recorded'],
  ['estimated', 'Estimated'],
  ['mixed', 'Mixed'],
])('labels a %s cost as "%s"', (source, label) => {
  render(
    <I18nextProvider i18n={i18n}>
      <CostSourceBadge source={source} />
    </I18nextProvider>
  )

  expect(screen.getByText(label)).toBeInTheDocument()
})

it('does not turn the badge into a copy-to-clipboard control', () => {
  render(
    <I18nextProvider i18n={i18n}>
      <CostSourceBadge source='estimated' />
    </I18nextProvider>
  )

  expect(
    screen.getByText('Estimated').closest('[data-slot="status-badge"]')
  ).not.toHaveClass('cursor-copy')
})
