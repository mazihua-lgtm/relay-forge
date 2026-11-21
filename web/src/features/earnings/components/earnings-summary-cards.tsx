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
import { Activity, Coins, TrendingUp, Wallet } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatCard } from '@/features/dashboard/components/ui/stat-card'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatPercent, formatQuota } from '@/lib/format'

import type { EarningsOverview } from '../types'
import { CostSourceBadge } from './cost-source-badge'

interface EarningsSummaryCardsProps {
  data?: EarningsOverview
  loading: boolean
  error: boolean
}

/** Revenue, upstream cost, gross profit and request count of the window. */
export function EarningsSummaryCards(props: EarningsSummaryCardsProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const data = props.data
  const loading = props.loading
  const error = props.error

  // Without net revenue there is no margin to speak of.
  const margin =
    data && data.total_revenue > 0
      ? formatPercent(data.gross_margin * 100)
      : '-'
  const losingMoney = data !== undefined && data.gross_profit < 0

  return (
    <div className='grid gap-3 sm:grid-cols-2 xl:grid-cols-4'>
      <StatCard
        title={t('Revenue')}
        value={data ? formatQuota(data.total_revenue) : '-'}
        description={t('Charged to users, minus refunds')}
        icon={Wallet}
        tone='accent-1'
        loading={loading}
        error={error}
        detailsOnMobile
        details={
          data
            ? [{ label: t('Refunds'), value: formatQuota(data.refund_quota) }]
            : undefined
        }
      />
      <StatCard
        title={t('Upstream cost')}
        value={data ? formatQuota(data.total_cost) : '-'}
        description={t('What the upstream channels charged you')}
        icon={Coins}
        tone='accent-2'
        loading={loading}
        error={error}
        action={
          data ? <CostSourceBadge source={data.cost_source} /> : undefined
        }
        detailsOnMobile
        details={
          data
            ? [
                {
                  label: t('Commission paid'),
                  value: formatQuota(data.total_commission),
                },
              ]
            : undefined
        }
      />
      <StatCard
        title={t('Gross profit')}
        value={data ? formatQuota(data.gross_profit) : '-'}
        description={t('Revenue minus upstream cost and commission')}
        icon={TrendingUp}
        tone='accent-3'
        loading={loading}
        error={error}
        detailsOnMobile
        details={
          data
            ? [
                {
                  label: t('Gross margin'),
                  value: margin,
                  tone: losingMoney ? 'destructive' : 'success',
                },
              ]
            : undefined
        }
      />
      <StatCard
        title={t('Requests')}
        value={data ? formatNumber(data.request_count, locale) : '-'}
        description={t('Billed requests in this period')}
        icon={Activity}
        tone='accent-1'
        loading={loading}
        error={error}
      />
    </div>
  )
}
