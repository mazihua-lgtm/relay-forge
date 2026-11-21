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
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'

import { clientFormSchema, type ClientFormSchema } from '../lib/schema'

interface ClientFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isSubmitting: boolean
  /** Receives the budget already converted to quota units. */
  onSubmit: (values: { name: string; budget: number; remark: string }) => void
}

const DEFAULT_VALUES: ClientFormSchema = {
  name: '',
  budget: 0,
  remark: '',
}

export function ClientFormDialog(props: ClientFormDialogProps) {
  const { t } = useTranslation()
  const form = useForm<ClientFormSchema>({
    resolver: zodResolver(clientFormSchema),
    defaultValues: DEFAULT_VALUES,
    mode: 'onBlur',
  })

  useEffect(() => {
    if (props.open) {
      form.reset(DEFAULT_VALUES)
    }
  }, [props.open, form])

  const quotaPerUnit = getCurrencyDisplay().config.quotaPerUnit

  const handleSubmit = (values: ClientFormSchema) => {
    props.onSubmit({
      name: values.name.trim(),
      budget: Math.ceil(Math.max(0, values.budget) * quotaPerUnit),
      remark: values.remark.trim(),
    })
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('New client')}</DialogTitle>
          <DialogDescription>
            {t(
              'The budget is what this customer paid you. Usage is read from the keys you issue under the client.'
            )}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            className='space-y-4'
            onSubmit={form.handleSubmit(handleSubmit)}
          >
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Client name')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      maxLength={64}
                      placeholder={t('For example: Acme Corp')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='budget'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('Budget')} ({getCurrencyLabel()})
                  </FormLabel>
                  <FormControl>
                    <Input
                      name={field.name}
                      ref={field.ref}
                      onBlur={field.onBlur}
                      value={field.value}
                      type='number'
                      min={0}
                      step='0.01'
                      inputMode='decimal'
                      onChange={(event) =>
                        field.onChange(
                          event.target.value === ''
                            ? 0
                            : Number(event.target.value)
                        )
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='remark'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Remark')}</FormLabel>
                  <FormControl>
                    <Input {...field} maxLength={255} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => props.onOpenChange(false)}
              >
                {t('Cancel')}
              </Button>
              <Button type='submit' disabled={props.isSubmitting}>
                {t('Create')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
