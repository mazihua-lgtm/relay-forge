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
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { requireServerSuccess } from '@/lib/server-error-message'

import { getEarningsOverview } from './api'
import {
  ChannelBreakdown,
  ModelBreakdown,
} from './components/earnings-breakdowns'
import { EarningsRangePicker } from './components/earnings-range-picker'
import { EarningsSummaryCards } from './components/earnings-summary-cards'
import { EstimatedCostNotice } from './components/estimated-cost-notice'
import { EARNINGS_DEFAULT_PRESET } from './constants'
import {
  isValidRange,
  rangeForPreset,
  toEarningsQuery,
  type EarningsRange,
} from './lib'

type BreakdownTab = 'channel' | 'model'

/** Operator earnings: what users were charged against what upstream cost. */
export function Earnings() {
  const { t } = useTranslation()
  const [range, setRange] = useState<EarningsRange>(() =>
    rangeForPreset(EARNINGS_DEFAULT_PRESET)
  )
  const [tab, setTab] = useState<BreakdownTab>('channel')

  const valid = isValidRange(range)
  const query = useMemo(() => toEarningsQuery(range), [range])
  const overview = useQuery({
    queryKey: ['earnings', 'overview', query],
    queryFn: async () => requireServerSuccess(await getEarningsOverview(query)),
    enabled: valid,
  })
  const data = overview.data?.data
  // A new window starts every report on its first page again.
  const windowKey = `${query.start_date}:${query.end_date}`

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Earnings')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <EarningsRangePicker range={range} onChange={setRange} />
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-4 pb-4'>
          {data && (
            <EstimatedCostNotice
              source={data.cost_source}
              costRatio={data.cost_ratio}
            />
          )}
          <EarningsSummaryCards
            data={data}
            loading={valid && overview.isLoading}
            error={valid && overview.isError}
          />
          <Tabs
            value={tab}
            onValueChange={(value) =>
              setTab(value === 'model' ? 'model' : 'channel')
            }
          >
            <TabsList>
              <TabsTrigger value='channel'>{t('By channel')}</TabsTrigger>
              <TabsTrigger value='model'>{t('By model')}</TabsTrigger>
            </TabsList>
          </Tabs>
          {valid && tab === 'channel' && (
            <ChannelBreakdown key={windowKey} query={query} />
          )}
          {valid && tab === 'model' && (
            <ModelBreakdown key={windowKey} query={query} />
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
