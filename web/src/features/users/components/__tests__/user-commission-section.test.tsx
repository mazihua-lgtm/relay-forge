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
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { UserCommissionSection } from '../user-commission-section'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

const PATCH_URL = '/api/user/5/commission'

function setup(rate: number, extra?: { onSubmit?: () => void }) {
  const onSaved = vi.fn()
  const section = (
    <UserCommissionSection userId={5} rate={rate} onSaved={onSaved} />
  )
  render(
    extra?.onSubmit ? (
      <form
        onSubmit={(event) => {
          event.preventDefault()
          extra.onSubmit?.()
        }}
      >
        {section}
        <button type='submit'>Save user</button>
      </form>
    ) : (
      section
    )
  )
  return {
    onSaved,
    input: screen.getByLabelText('Commission rate (%)'),
    save: () => screen.getByRole('button', { name: 'Save' }),
  }
}

test('shows the saved rate as a percent and keeps Save off until it changes', async () => {
  const user = userEvent.setup()
  const { input, save } = setup(0.1)

  expect(input).toHaveValue(10)
  expect(save()).toBeDisabled()

  await user.clear(input)
  await user.type(input, '12.5')
  expect(save()).toBeEnabled()

  await user.clear(input)
  await user.type(input, '10')
  expect(save()).toBeDisabled()
})

test('a stored rate with more decimals than shown is not reported as changed', () => {
  const { input, save } = setup(0.123456)

  expect(input).toHaveValue(12.35)
  expect(save()).toBeDisabled()
})

test('saving sends the rate as a fraction and reports the new rate', async () => {
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const success = vi.spyOn(toast, 'success')
  const user = userEvent.setup()
  const { input, save, onSaved } = setup(0)

  await user.type(input, '12.5')
  await user.click(save())

  await waitFor(() => expect(onSaved).toHaveBeenCalledWith(0.125))
  expect(patch).toHaveBeenCalledOnce()
  expect(patch).toHaveBeenCalledWith(PATCH_URL, { commission_rate: 0.125 })
  expect(success).toHaveBeenCalledWith('Commission rate updated')
})

test('a rate set back to 0 is sent as 0', async () => {
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const user = userEvent.setup()
  const { input, save, onSaved } = setup(0.1)

  await user.clear(input)
  await user.type(input, '0')
  await user.click(save())

  await waitFor(() => expect(onSaved).toHaveBeenCalledWith(0))
  expect(patch).toHaveBeenCalledWith(PATCH_URL, { commission_rate: 0 })
})

test('a refusal shows the server message, keeps the draft and can be retried', async () => {
  const patch = vi.spyOn(api, 'patch').mockResolvedValueOnce({
    data: { success: false, message: 'Commission rate is too high' },
  })
  const error = vi.spyOn(toast, 'error')
  const success = vi.spyOn(toast, 'success')
  const user = userEvent.setup()
  const { input, save, onSaved } = setup(0)

  await user.type(input, '40')
  await user.click(save())

  await waitFor(() =>
    expect(error).toHaveBeenCalledWith('Commission rate is too high')
  )
  expect(onSaved).not.toHaveBeenCalled()
  expect(success).not.toHaveBeenCalled()
  expect(input).toHaveValue(40)
  await waitFor(() => expect(save()).toBeEnabled())

  patch.mockResolvedValueOnce({ data: { success: true } })
  await user.click(save())
  await waitFor(() => expect(onSaved).toHaveBeenCalledWith(0.4))
})

test('a request that fails to reach the server reports a plain failure', async () => {
  vi.spyOn(api, 'patch').mockRejectedValue(new Error('Network Error'))
  const error = vi.spyOn(toast, 'error')
  const user = userEvent.setup()
  const { input, save, onSaved } = setup(0)

  await user.type(input, '5')
  await user.click(save())

  await waitFor(() => expect(error).toHaveBeenCalledOnce())
  expect(onSaved).not.toHaveBeenCalled()
})

test.each(['150', '-5', ''])(
  'the rate %j is flagged and cannot be saved',
  async (text) => {
    const patch = vi.spyOn(api, 'patch')
    const user = userEvent.setup()
    const { input, save } = setup(0.1)

    await user.clear(input)
    if (text) await user.type(input, text)

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Enter a number from 0 to 100'
    )
    expect(input).toHaveAttribute('aria-invalid', 'true')
    expect(save()).toBeDisabled()
    await user.type(input, '{Enter}')
    expect(patch).not.toHaveBeenCalled()
  }
)

test('a valid rate shows no error', () => {
  const { input } = setup(0.1)

  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(input).toHaveAttribute('aria-invalid', 'false')
})

test('Enter saves the rate without submitting the surrounding form', async () => {
  const patch = vi
    .spyOn(api, 'patch')
    .mockResolvedValue({ data: { success: true } })
  const onSubmit = vi.fn()
  const user = userEvent.setup()
  const { input, onSaved } = setup(0, { onSubmit })

  await user.type(input, '8{Enter}')

  await waitFor(() => expect(onSaved).toHaveBeenCalledWith(0.08))
  expect(patch).toHaveBeenCalledOnce()
  expect(onSubmit).not.toHaveBeenCalled()
})

test('a rate that is being typed never blocks saving the surrounding form', async () => {
  const onSubmit = vi.fn()
  const user = userEvent.setup()
  const { input } = setup(0.1, { onSubmit })

  await user.clear(input)
  await user.type(input, '150')
  await user.click(screen.getByRole('button', { name: 'Save user' }))

  expect(onSubmit).toHaveBeenCalledOnce()
})
