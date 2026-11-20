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
import { useState, type ComponentProps } from 'react'

import { Input } from '@/components/ui/input'

type CostRatioInputProps = Omit<
  ComponentProps<typeof Input>,
  'value' | 'onChange' | 'type'
> & {
  /** The ratio in the form; 0 or unset means not set. */
  value: number | undefined
  onChange: (ratio: number) => void
}

const toText = (ratio: number | undefined) => (ratio ? String(ratio) : '')
const toRatio = (text: string) => (text.trim() === '' ? 0 : Number(text))

/**
 * A decimal input that keeps what the operator is typing as text and gives
 * the form a number. A number input controlled by the number itself turns an
 * emptied field into "0" at once, so typing "0.4" over a saved ratio would come
 * out as "00.4". A ratio that is not set shows empty, with 0 as the placeholder.
 */
export function CostRatioInput({
  value,
  onChange,
  ...props
}: CostRatioInputProps) {
  const [text, setText] = useState(() => toText(value))
  const [formValue, setFormValue] = useState(value)
  // The form changes the ratio by itself when a channel loads or resets; follow
  // that, but not the changes made here (they already match the text).
  if (value !== formValue) {
    setFormValue(value)
    if (toRatio(text) !== (value ?? 0)) setText(toText(value))
  }

  return (
    <Input
      type='number'
      inputMode='decimal'
      step='any'
      placeholder='0'
      {...props}
      value={text}
      onChange={(event) => {
        const next = event.target.value
        setText(next)
        setFormValue(toRatio(next))
        onChange(toRatio(next))
      }}
    />
  )
}
