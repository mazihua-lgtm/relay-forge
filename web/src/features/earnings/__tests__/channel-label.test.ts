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

import { channelLabel } from '../lib'
import type { EarningsChannelRow } from '../types'

function row(overrides: Partial<EarningsChannelRow>): EarningsChannelRow {
  return {
    channel_id: 1,
    channel_name: 'openai-main',
    total_revenue: 0,
    total_cost: 0,
    gross_profit: 0,
    gross_margin: 0,
    request_count: 0,
    refund_quota: 0,
    cost_source: 'estimated',
    ...overrides,
  }
}

describe('channelLabel', () => {
  it('shows the channel name when there is one', () => {
    expect(channelLabel(row({}), 'Deleted channel', 'No channel')).toBe(
      'openai-main'
    )
  })

  it('says the channel was deleted when a real id has no name left', () => {
    expect(
      channelLabel(
        row({ channel_id: 7, channel_name: '' }),
        'Deleted channel',
        'No channel'
      )
    ).toBe('Deleted channel')
  })

  it('says there was no channel for logs that carry channel id 0', () => {
    expect(
      channelLabel(
        row({ channel_id: 0, channel_name: '' }),
        'Deleted channel',
        'No channel'
      )
    ).toBe('No channel')
  })
})
