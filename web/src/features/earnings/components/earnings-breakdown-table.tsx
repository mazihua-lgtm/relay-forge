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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatPercent, formatQuota } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { EarningsFigures } from '../types'
import { CostSourceBadge } from './cost-source-badge'

interface EarningsBreakdownTableProps<TRow extends EarningsFigures> {
  /** Header of the first column: what the rows are grouped by. */
  nameHeader: string
  /** Accessible name of the table. */
  label: string
  renderName: (row: TRow) => ReactNode
  getRowKey: (row: TRow) => string | number
  rows: TRow[]
  total: number
  page: number
  pageSize: number
  /** True while the first page is loading and there is nothing to show yet. */
  loading: boolean
  /** True while another page is being fetched; the old rows stay visible. */
  fetching: boolean
  error: boolean
  onRetry: () => void
  onPageChange: (page: number) => void
}

const NUMERIC_CELL = 'text-right tabular-nums'

/** One page of a per-channel or per-model earnings report. */
export function EarningsBreakdownTable<TRow extends EarningsFigures>(
  props: EarningsBreakdownTableProps<TRow>
) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  const columns: StaticDataTableColumn<TRow>[] = [
    {
      id: 'name',
      header: props.nameHeader,
      cell: (row) => props.renderName(row),
    },
    {
      id: 'revenue',
      header: t('Revenue'),
      className: 'text-right',
      cellClassName: NUMERIC_CELL,
      cell: (row) => formatQuota(row.total_revenue),
    },
    {
      id: 'cost',
      header: t('Upstream cost'),
      className: 'text-right',
      cellClassName: NUMERIC_CELL,
      cell: (row) => formatQuota(row.total_cost),
    },
    {
      id: 'profit',
      header: t('Gross profit'),
      className: 'text-right',
      cellClassName: (row) =>
        cn(NUMERIC_CELL, row.gross_profit < 0 && 'text-destructive'),
      cell: (row) => formatQuota(row.gross_profit),
    },
    {
      id: 'margin',
      header: t('Gross margin'),
      className: 'text-right',
      cellClassName: (row) =>
        cn(NUMERIC_CELL, row.gross_profit < 0 && 'text-destructive'),
      cell: (row) =>
        row.total_revenue > 0 ? formatPercent(row.gross_margin * 100) : '-',
    },
    {
      id: 'requests',
      header: t('Requests'),
      className: 'text-right',
      cellClassName: NUMERIC_CELL,
      cell: (row) => formatNumber(row.request_count, locale),
    },
    {
      id: 'source',
      header: t('Cost source'),
      cell: (row) => <CostSourceBadge source={row.cost_source} />,
    },
  ]

  if (props.loading) return <LoadingState />
  if (props.error) return <ErrorState onRetry={props.onRetry} />

  const pageCount = Math.max(1, Math.ceil(props.total / props.pageSize))

  return (
    <div className='flex flex-col gap-3'>
      <StaticDataTable
        columns={columns}
        data={props.rows}
        getRowKey={(row) => props.getRowKey(row)}
        className={cn(props.fetching && 'opacity-60 transition-opacity')}
        tableProps={{ 'aria-label': props.label }}
        emptyContent={
          <EmptyState
            title={t('No billed requests in this period')}
            className='min-h-40'
          />
        }
      />
      <div className='flex items-center justify-between gap-2'>
        <span className='text-muted-foreground text-sm'>
          {t('Page {{current}} of {{total}}', {
            current: props.page,
            total: pageCount,
          })}
        </span>
        <div className='flex gap-2'>
          <Button
            variant='outline'
            size='sm'
            disabled={props.page <= 1}
            onClick={() => props.onPageChange(props.page - 1)}
          >
            {t('Previous page')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            disabled={props.page >= pageCount}
            onClick={() => props.onPageChange(props.page + 1)}
          >
            {t('Next page')}
          </Button>
        </div>
      </div>
    </div>
  )
}
