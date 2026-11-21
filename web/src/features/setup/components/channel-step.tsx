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
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { cn } from '@/lib/utils'

import type { SetupFormValues } from '../types'

interface ChannelStepProps {
  form: UseFormReturn<SetupFormValues>
}

// Only the two channel types that cover almost every relay deployment are
// offered here. Everything else stays in the admin panel.
const CHANNEL_TYPES = [
  { value: 1, titleKey: 'OpenAI compatible' },
  { value: 14, titleKey: 'Anthropic' },
]

export function ChannelStep({ form }: ChannelStepProps) {
  const { t } = useTranslation()

  return (
    <div className='space-y-6'>
      <div className='space-y-1'>
        <h2 className='text-lg font-semibold'>{t('First upstream channel')}</h2>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Connect one upstream now so the site can serve requests. You can add more channels later.'
          )}
        </p>
      </div>

      <FormField
        control={form.control}
        name='channelType'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Channel type')}</FormLabel>
            <FormControl>
              <RadioGroup
                value={String(field.value)}
                onValueChange={(value) => field.onChange(Number(value))}
                className='grid gap-3 sm:grid-cols-2'
              >
                {CHANNEL_TYPES.map((option) => (
                  <Label
                    key={option.value}
                    htmlFor={`channel-type-${option.value}`}
                    className={cn(
                      'hover:border-primary/40 has-data-[checked]:border-primary has-data-[checked]:ring-primary/20 bg-card border-muted flex cursor-pointer items-center gap-3 rounded-xl border p-4 font-normal transition-all has-data-[checked]:ring-2'
                    )}
                  >
                    <RadioGroupItem
                      id={`channel-type-${option.value}`}
                      value={String(option.value)}
                    />
                    <span className='text-sm font-semibold'>
                      {t(option.titleKey)}
                    </span>
                  </Label>
                ))}
              </RadioGroup>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name='channelKey'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('API key')}</FormLabel>
            <FormControl>
              <Input
                {...field}
                type='password'
                placeholder='sk-…'
                autoComplete='off'
                onChange={(event) => {
                  form.clearErrors('channelKey')
                  field.onChange(event)
                }}
              />
            </FormControl>
            <FormDescription>
              {t('Leave the API key empty to skip this step.')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <div className='grid gap-6 sm:grid-cols-2'>
        <FormField
          control={form.control}
          name='channelName'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Channel name')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  maxLength={64}
                  placeholder={t(
                    'Optional. Defaults to the channel type name.'
                  )}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name='channelBaseURL'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Base URL')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  maxLength={512}
                  placeholder='https://api.example.com'
                />
              </FormControl>
              <FormDescription>
                {t('Optional. Leave empty to use the provider default.')}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <FormField
        control={form.control}
        name='channelModels'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Models')}</FormLabel>
            <FormControl>
              <Input
                {...field}
                placeholder='gpt-4o, gpt-4o-mini'
                onChange={(event) => {
                  form.clearErrors('channelModels')
                  field.onChange(event)
                }}
              />
            </FormControl>
            <FormDescription>
              {t(
                'Comma separated. At least one model is required once a key is set.'
              )}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  )
}
