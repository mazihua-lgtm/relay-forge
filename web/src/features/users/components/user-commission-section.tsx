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
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SideDrawerSection } from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { handleServerError } from '@/lib/handle-server-error'

import { updateUserCommission } from '../api'
import { commissionRateToPercent, parsePercentText } from '../lib'

interface UserCommissionSectionProps {
  userId: number
  /** The saved rate as a fraction from 0 to 1. */
  rate: number
  /** Called with the new rate once the server has saved it. */
  onSaved: (rate: number) => void
}

/**
 * The share of what the users a person invited spend that is credited to that
 * person as quota on every request. It saves on its own, like the quota
 * adjustment, because it has its own endpoint. Mount it with a key that
 * includes the saved rate so the input starts from the saved value again.
 */
export function UserCommissionSection(props: UserCommissionSectionProps) {
  const { t } = useTranslation()
  const inputId = useId()
  const errorId = useId()
  const savedText = String(commissionRateToPercent(props.rate))
  const [text, setText] = useState(savedText)
  const [saving, setSaving] = useState(false)

  const rate = parsePercentText(text)
  const invalid = rate === null
  // Compare the text, so a stored rate with more decimals than the input
  // shows is not reported as changed before anyone touches it.
  const changed = text.trim() !== savedText
  const canSave = changed && !invalid && !saving

  const save = async () => {
    if (rate === null || !canSave) return
    setSaving(true)
    try {
      const result = await updateUserCommission(props.userId, rate)
      if (result.success) {
        toast.success(t('Commission rate updated'))
        props.onSaved(rate)
      } else {
        handleServerError(result, t('Failed to update the commission rate'))
      }
    } catch (error) {
      handleServerError(error, t('Failed to update the commission rate'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <SideDrawerSection>
      <h3 className='text-sm font-medium'>{t('Referral Commission')}</h3>
      <p className='text-muted-foreground text-xs'>
        {t(
          'On every request, this share of what the users this person invited spend is credited to them as quota. It comes out of your margin, so keep it below your gross margin.'
        )}
      </p>
      <div className='flex items-end gap-2'>
        <div className='flex flex-1 flex-col gap-1.5'>
          <Label htmlFor={inputId}>{t('Commission rate (%)')}</Label>
          <Input
            id={inputId}
            type='number'
            inputMode='decimal'
            // No min/max: native validation would block the whole user form
            // from saving over a rate that is not being saved; this section
            // reports an invalid rate itself.
            step='any'
            value={text}
            onChange={(event) => setText(event.target.value)}
            onKeyDown={(event) => {
              // This section sits inside the user form: Enter must save the
              // rate, not submit the whole form and close the drawer.
              if (event.key !== 'Enter') return
              event.preventDefault()
              void save()
            }}
            aria-invalid={invalid}
            aria-describedby={invalid ? errorId : undefined}
          />
        </div>
        <Button
          type='button'
          variant='outline'
          disabled={!canSave}
          onClick={() => void save()}
        >
          {saving ? t('Saving...') : t('Save')}
        </Button>
      </div>
      {invalid && (
        <p id={errorId} role='alert' className='text-destructive text-xs'>
          {t('Enter a number from 0 to 100')}
        </p>
      )}
    </SideDrawerSection>
  )
}
