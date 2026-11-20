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
import { toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { handleProbeChannels } from '../channel-actions'

afterEach(() => {
  vi.restoreAllMocks()
})

test('a started sweep is requested once and reported as started', async () => {
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      started: true,
      message: 'channel health probe started',
    },
  })
  const success = vi.spyOn(toast, 'success')
  const info = vi.spyOn(toast, 'info')
  const error = vi.spyOn(toast, 'error')

  await handleProbeChannels()

  expect(post).toHaveBeenCalledOnce()
  expect(post.mock.calls[0]?.[0]).toBe('/api/channel/probe')
  expect(success).toHaveBeenCalledWith(
    'Channel health probe started. Refresh in a minute to see the results.'
  )
  expect(info).not.toHaveBeenCalled()
  expect(error).not.toHaveBeenCalled()
})

test('a sweep that is already running is reported without claiming a new start', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      started: false,
      message: 'channel health probe already running',
    },
  })
  const success = vi.spyOn(toast, 'success')
  const info = vi.spyOn(toast, 'info')

  await handleProbeChannels()

  expect(info).toHaveBeenCalledWith(
    'A channel health probe is already running.'
  )
  expect(success).not.toHaveBeenCalled()
})

test('a refusal from the server shows the server message', async () => {
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: false, message: 'No permission to operate channels' },
  })
  const success = vi.spyOn(toast, 'success')
  const error = vi.spyOn(toast, 'error')

  await handleProbeChannels()

  expect(error).toHaveBeenCalledWith('No permission to operate channels')
  expect(success).not.toHaveBeenCalled()
})

test('a request that fails to reach the server falls back to a plain message', async () => {
  vi.spyOn(api, 'post').mockRejectedValue(new Error('Network Error'))
  const success = vi.spyOn(toast, 'success')
  const error = vi.spyOn(toast, 'error')

  await handleProbeChannels()

  expect(error).toHaveBeenCalledOnce()
  expect(success).not.toHaveBeenCalled()
})
