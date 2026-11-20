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
import { useState } from 'react'
import { afterEach, expect, test } from 'vitest'

import { CostRatioInput } from '../cost-ratio-input'

afterEach(cleanup)

function Harness(props: { initial?: number }) {
  const [ratio, setRatio] = useState<number | undefined>(props.initial ?? 0)
  return (
    <>
      <CostRatioInput aria-label='ratio' value={ratio} onChange={setRatio} />
      <output aria-label='form value'>{String(ratio)}</output>
      <button type='button' onClick={() => setRatio(0.7)}>
        Load channel
      </button>
    </>
  )
}

const field = () => screen.getByLabelText('ratio')
const formValue = () => screen.getByLabelText('form value').textContent

test('a ratio that is not set shows empty, with 0 as the placeholder', () => {
  render(<Harness />)

  expect(field()).toHaveDisplayValue('')
  expect(field()).toHaveAttribute('placeholder', '0')
})

test('a saved ratio is shown as saved', () => {
  render(<Harness initial={0.4} />)

  expect(field()).toHaveDisplayValue('0.4')
})

test('typing a decimal gives the form that decimal', async () => {
  const user = userEvent.setup()
  render(<Harness />)

  await user.type(field(), '0.45')

  expect(field()).toHaveDisplayValue('0.45')
  expect(formValue()).toBe('0.45')
})

test('replacing a saved ratio leaves no leading zero behind', async () => {
  const user = userEvent.setup()
  render(<Harness initial={0.4} />)

  await user.clear(field())
  expect(field()).toHaveDisplayValue('')
  expect(formValue()).toBe('0')

  await user.type(field(), '1.5')
  expect(field()).toHaveDisplayValue('1.5')
  expect(formValue()).toBe('1.5')
})

test('a typed 0 stays visible as 0', async () => {
  const user = userEvent.setup()
  render(<Harness initial={0.4} />)

  await user.clear(field())
  await user.type(field(), '0')

  expect(field()).toHaveDisplayValue('0')
  expect(formValue()).toBe('0')
})

test('follows the form when it changes the ratio by itself', async () => {
  const user = userEvent.setup()
  render(<Harness />)

  await user.click(screen.getByRole('button', { name: 'Load channel' }))

  expect(field()).toHaveDisplayValue('0.7')
})
