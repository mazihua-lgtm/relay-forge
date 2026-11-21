/*
Copyright (C) 2023-2026 QuantumNous

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

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, it } from 'vitest'

import { clientFormSchema } from '../schema'

describe('clientFormSchema', () => {
  it('accepts a named client with a zero budget', () => {
    const result = clientFormSchema.safeParse({
      name: 'Acme',
      budget: 0,
      remark: '',
    })
    expect(result.success).toBe(true)
  })

  it('rejects a blank name', () => {
    const result = clientFormSchema.safeParse({
      name: '   ',
      budget: 10,
      remark: '',
    })
    expect(result.success).toBe(false)
  })

  it('rejects a negative budget', () => {
    const result = clientFormSchema.safeParse({
      name: 'Acme',
      budget: -1,
      remark: '',
    })
    expect(result.success).toBe(false)
  })

  it('rejects a name longer than 64 characters', () => {
    const result = clientFormSchema.safeParse({
      name: 'a'.repeat(65),
      budget: 1,
      remark: '',
    })
    expect(result.success).toBe(false)
  })
})
