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
import { useTranslation } from 'react-i18next'

import { DatePicker } from '@/components/date-picker'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'

import { EARNINGS_RANGE_PRESETS, type EarningsRangePreset } from '../constants'
import {
  isValidRange,
  matchPreset,
  rangeForPreset,
  type EarningsRange,
} from '../lib'

interface EarningsRangePickerProps {
  range: EarningsRange
  onChange: (range: EarningsRange) => void
}

/** Quick presets plus a custom start and end day. */
export function EarningsRangePicker(props: EarningsRangePickerProps) {
  const { t } = useTranslation()
  const activePreset = matchPreset(props.range)

  const handlePresetChange = (values: string[]) => {
    // A single-choice toggle reports the previous value too; the new one is
    // the value that is not the active preset.
    const next = values.find((value) => value !== activePreset)
    if (next) props.onChange(rangeForPreset(next as EarningsRangePreset))
  }

  return (
    <div className='flex flex-col items-end gap-1'>
      <div className='flex flex-wrap items-center justify-end gap-2'>
        <ToggleGroup
          value={activePreset ? [activePreset] : []}
          onValueChange={handlePresetChange}
          aria-label={t('Quick range')}
          variant='outline'
          size='sm'
        >
          {EARNINGS_RANGE_PRESETS.map((preset) => (
            <ToggleGroupItem key={preset.value} value={preset.value}>
              {t(preset.labelKey)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <div className='flex flex-wrap items-center gap-2'>
          <DatePicker
            selected={props.range.start}
            onSelect={(date) => {
              if (date) props.onChange({ ...props.range, start: date })
            }}
            placeholder={t('Start date')}
          />
          <span className='text-muted-foreground' aria-hidden='true'>
            –
          </span>
          <DatePicker
            selected={props.range.end}
            onSelect={(date) => {
              if (date) props.onChange({ ...props.range, end: date })
            }}
            placeholder={t('End date')}
          />
        </div>
      </div>
      {!isValidRange(props.range) && (
        <p role='alert' className='text-destructive text-xs'>
          {t('The start date must not be later than the end date')}
        </p>
      )}
    </div>
  )
}
