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
import { Wallet } from 'lucide-react'
import { afterEach, expect, test } from 'vitest'

import { StatCard } from '../stat-card'

afterEach(cleanup)

const DETAILS = [{ label: 'Refunds', value: '$0.2' }]

function renderCard(extra: { detailsOnMobile?: boolean } = {}) {
  render(
    <StatCard
      title='Revenue'
      value='$14.2'
      description='Charged to users'
      icon={Wallet}
      details={DETAILS}
      {...extra}
    />
  )
  return screen.getByText('Refunds').closest('.hidden, .block')
}

test('details are hidden on small screens by default', () => {
  const wrapper = renderCard()

  expect(wrapper).toHaveClass('hidden', 'sm:block')
})

test('details stay visible on small screens when asked to', () => {
  const wrapper = renderCard({ detailsOnMobile: true })

  expect(wrapper).toHaveClass('block')
  expect(wrapper).not.toHaveClass('hidden')
})

test('asking for details on small screens changes nothing when there are none', () => {
  render(
    <StatCard
      title='Requests'
      value='18'
      description='Billed requests'
      icon={Wallet}
      detailsOnMobile
    />
  )

  expect(screen.queryByText('Refunds')).not.toBeInTheDocument()
  const wrapper = document.querySelector('.hidden')
  expect(wrapper).toHaveClass('sm:block')
  expect(wrapper).not.toHaveClass('block')
})
