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
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { requireServerSuccess } from '@/lib/server-error-message'

import { getEarningsByChannel, getEarningsByModel } from '../api'
import { EARNINGS_PAGE_SIZE } from '../constants'
import { channelLabel } from '../lib'
import type {
  EarningsChannelRow,
  EarningsListQuery,
  EarningsModelRow,
  EarningsQuery,
} from '../types'
import { EarningsBreakdownTable } from './earnings-breakdown-table'

interface BreakdownProps {
  query: EarningsQuery
}

function listQuery(query: EarningsQuery, page: number): EarningsListQuery {
  return { ...query, p: page, page_size: EARNINGS_PAGE_SIZE }
}

/** Earnings per upstream channel, largest revenue first. */
export function ChannelBreakdown(props: BreakdownProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const params = listQuery(props.query, page)
  const result = useQuery({
    queryKey: ['earnings', 'by-channel', params],
    queryFn: async () =>
      requireServerSuccess(await getEarningsByChannel(params)),
    placeholderData: keepPreviousData,
  })
  const list = result.data?.data

  return (
    <EarningsBreakdownTable<EarningsChannelRow>
      nameHeader={t('Channel')}
      label={t('Earnings by channel')}
      renderName={(row) => (
        <span className='flex min-w-0 items-baseline gap-1.5'>
          <span className='truncate font-medium'>
            {channelLabel(row, t('Deleted channel'), t('No channel'))}
          </span>
          {row.channel_id > 0 && (
            <span className='text-muted-foreground text-xs tabular-nums'>
              #{row.channel_id}
            </span>
          )}
        </span>
      )}
      getRowKey={(row) => row.channel_id}
      rows={list?.items ?? []}
      total={list?.total ?? 0}
      page={page}
      pageSize={EARNINGS_PAGE_SIZE}
      loading={result.isLoading}
      fetching={result.isFetching}
      error={result.isError}
      onRetry={() => void result.refetch()}
      onPageChange={setPage}
    />
  )
}

/** Earnings per model, largest revenue first. */
export function ModelBreakdown(props: BreakdownProps) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const params = listQuery(props.query, page)
  const result = useQuery({
    queryKey: ['earnings', 'by-model', params],
    queryFn: async () => requireServerSuccess(await getEarningsByModel(params)),
    placeholderData: keepPreviousData,
  })
  const list = result.data?.data

  return (
    <EarningsBreakdownTable<EarningsModelRow>
      nameHeader={t('Model')}
      label={t('Earnings by model')}
      renderName={(row) => (
        <span className='truncate font-medium'>
          {row.model_name || t('Unknown model')}
        </span>
      )}
      getRowKey={(row) => row.model_name}
      rows={list?.items ?? []}
      total={list?.total ?? 0}
      page={page}
      pageSize={EARNINGS_PAGE_SIZE}
      loading={result.isLoading}
      fetching={result.isFetching}
      error={result.isError}
      onRetry={() => void result.refetch()}
      onPageChange={setPage}
    />
  )
}
