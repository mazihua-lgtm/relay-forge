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
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import type { SetupFormValues } from '../types'

interface SiteStepProps {
  form: UseFormReturn<SetupFormValues>
}

export function SiteStep({ form }: SiteStepProps) {
  const { t } = useTranslation()

  return (
    <div className='space-y-6'>
      <div className='space-y-1'>
        <h2 className='text-lg font-semibold'>{t('Site identity')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Both fields are optional. Leave them empty to keep the default name and logo.'
          )}
        </p>
      </div>

      <FormField
        control={form.control}
        name='siteName'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Site name')}</FormLabel>
            <FormControl>
              <Input
                {...field}
                placeholder={t('Shown in the header and browser tab')}
                maxLength={64}
                onChange={(event) => {
                  form.clearErrors('siteName')
                  field.onChange(event)
                }}
              />
            </FormControl>
            <FormDescription>{t('Up to 64 characters.')}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name='logo'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Logo URL')}</FormLabel>
            <FormControl>
              <Input
                {...field}
                placeholder='https://example.com/logo.png'
                maxLength={512}
                onChange={(event) => {
                  form.clearErrors('logo')
                  field.onChange(event)
                }}
              />
            </FormControl>
            <FormDescription>
              {t('An http(s) URL or a path on this site, such as /logo.png.')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  )
}
