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
import { Link } from '@tanstack/react-router'
import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  Alert,
  AlertAction,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type { EarningsCostSource } from '../types'

interface EstimatedCostNoticeProps {
  source: EarningsCostSource
  /** The charge-to-cost multiplier the estimated part is priced with. */
  costRatio: number
}

/**
 * Tells the operator when the shown cost is not fully recorded, so the profit
 * figures are not read as exact. Nothing is shown for a fully recorded window.
 */
export function EstimatedCostNotice(props: EstimatedCostNoticeProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  if (props.source === 'recorded') return null

  const ratio = formatNumber(props.costRatio, locale)
  const isMixed = props.source === 'mixed'

  return (
    <Alert>
      <Info aria-hidden='true' />
      <AlertTitle>
        {isMixed
          ? t('Part of the cost is estimated')
          : t('The cost is estimated')}
      </AlertTitle>
      <AlertDescription>
        {isMixed
          ? t(
              'Requests without a recorded purchase ratio are estimated as the charge divided by {{ratio}}. Set a purchase ratio on every channel to record the real cost of new requests.',
              { ratio }
            )
          : t(
              'No purchase ratio was recorded in this period, so the cost is estimated as the charge divided by {{ratio}} and every channel shows a similar margin. Set a purchase ratio on each channel to record the real cost of new requests.',
              { ratio }
            )}
      </AlertDescription>
      <AlertAction>
        <Button
          variant='outline'
          size='xs'
          render={<Link to='/channels' search={{}} />}
        >
          {t('Set purchase ratios')}
        </Button>
      </AlertAction>
    </Alert>
  )
}
