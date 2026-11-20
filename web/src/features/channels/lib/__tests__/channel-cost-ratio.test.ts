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

import { CHANNEL_STATUS, CHANNEL_STATUS_CONFIG } from '../../constants'
import { channelSchema } from '../../types'
import { getChannelConfigurationState } from '../channel-configuration'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
  type ChannelFormValues,
} from '../channel-form'

const RANGE_MESSAGE = 'Purchase ratio must be between 0 and 100'

function channelWith(extra: Record<string, unknown> = {}) {
  return channelSchema.parse({
    id: 7,
    name: 'Upstream A',
    type: 1,
    key: '',
    status: 1,
    created_time: 1,
    test_time: 0,
    response_time: 0,
    balance_updated_time: 0,
    models: 'gpt-4o',
    group: 'default',
    ...extra,
  })
}

function formWith(costRatio: number | undefined): ChannelFormValues {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'Upstream A',
    key: 'sk-test',
    models: 'gpt-4o',
    cost_ratio: costRatio,
  }
}

describe('purchase ratio in the channel form', () => {
  test('an editing form starts from the saved ratio', () => {
    expect(
      transformChannelToFormDefaults(channelWith({ cost_ratio: 0.4 }))
        .cost_ratio
    ).toBe(0.4)
  })

  test.each([
    ['not sent by an older server', {}],
    ['null', { cost_ratio: null }],
  ])('a channel whose ratio is %s starts the form at 0', (_name, extra) => {
    expect(transformChannelToFormDefaults(channelWith(extra)).cost_ratio).toBe(
      0
    )
  })

  test('a new channel starts without a ratio', () => {
    expect(CHANNEL_FORM_DEFAULT_VALUES.cost_ratio).toBe(0)
  })

  test('the ratio is sent as typed when creating and when updating', () => {
    expect(
      transformFormDataToCreatePayload(formWith(0.4)).channel.cost_ratio
    ).toBe(0.4)
    expect(transformFormDataToUpdatePayload(formWith(0.4), 7).cost_ratio).toBe(
      0.4
    )
  })

  test('an update always carries the ratio, so 0 clears one that was set', () => {
    const cleared = transformFormDataToUpdatePayload(formWith(0), 7)
    expect(cleared).toHaveProperty('cost_ratio', 0)
    const untouched = transformFormDataToUpdatePayload(formWith(undefined), 7)
    expect(untouched).toHaveProperty('cost_ratio', 0)
  })

  test('creation sends 0 when no ratio was entered', () => {
    expect(
      transformFormDataToCreatePayload(formWith(undefined)).channel
    ).toHaveProperty('cost_ratio', 0)
  })
})

describe('purchase ratio validation', () => {
  const parse = (ratio: number | undefined) =>
    channelFormSchema.safeParse(formWith(ratio))

  test.each([0, 0.4, 1, 100])('accepts %s', (ratio) => {
    const result = parse(ratio)
    expect(
      result.success ||
        !result.error.issues.some((issue) => issue.path[0] === 'cost_ratio')
    ).toBe(true)
  })

  test.each([-0.1, 100.1, 1000])(
    'refuses %s with the range message',
    (ratio) => {
      const result = parse(ratio)
      expect(result.success).toBe(false)
      const issues = result.success
        ? []
        : result.error.issues.filter((issue) => issue.path[0] === 'cost_ratio')
      expect(issues.map((issue) => issue.message)).toEqual([RANGE_MESSAGE])
    }
  )
})

describe('purchase ratio in the configuration overview', () => {
  const stateOf = (ratio: number | undefined) =>
    getChannelConfigurationState(formWith(ratio), {}, false)

  test('marks the cost block configured only when a ratio is set', () => {
    expect(stateOf(0.4).blocks.costRatio).toBe('configured')
    expect(stateOf(0).blocks.costRatio).toBe('idle')
    expect(stateOf(undefined).blocks.costRatio).toBe('idle')
  })

  test('a set ratio marks the routing category as configured', () => {
    expect(stateOf(0.4).sections.routing).toBe('configured')
    expect(stateOf(0).sections.routing).toBe('idle')
  })
})

describe('degraded channel status', () => {
  test('is a warning badge with its own label', () => {
    expect(CHANNEL_STATUS.DEGRADED).toBe(4)
    expect(CHANNEL_STATUS_CONFIG[CHANNEL_STATUS.DEGRADED]).toEqual({
      variant: 'warning',
      label: 'Degraded',
    })
  })
})
