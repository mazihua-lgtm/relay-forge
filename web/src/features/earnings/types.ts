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
// Response shapes of GET /api/admin/earnings/{overview,by-channel,by-model}.

/**
 * Where the cost of a figure came from: recorded from the channel's purchase
 * ratio when each request was logged, estimated from the charge, or a mix.
 */
export type EarningsCostSource = 'recorded' | 'estimated' | 'mixed'

/** The money block shared by the overview and both grouped reports. */
export interface EarningsFigures {
  /** Quota charged to users, minus refunds. */
  total_revenue: number
  total_cost: number
  gross_profit: number
  /** Profit over net revenue, as a fraction (0.25 is 25%). */
  gross_margin: number
  request_count: number
  refund_quota: number
  cost_source: EarningsCostSource
}

export interface EarningsDateRange {
  start: string
  end: string
}

export interface EarningsOverview extends EarningsFigures {
  /** The charge-to-cost multiplier that prices the estimated part of the cost. */
  cost_ratio: number
  date_range: EarningsDateRange
}

export interface EarningsChannelRow extends EarningsFigures {
  channel_id: number
  channel_name: string
}

export interface EarningsModelRow extends EarningsFigures {
  model_name: string
}

export interface EarningsList<TRow> {
  items: TRow[]
  total: number
  page: number
  page_size: number
  /** The cost source of the whole window, not just of this page. */
  cost_source: EarningsCostSource
  cost_ratio: number
  date_range: EarningsDateRange
}

export interface EarningsApiResponse<TData> {
  success: boolean
  message?: string
  data?: TData
}

/** A reporting window in whole days, as YYYY-MM-DD (the end day is included). */
export interface EarningsQuery {
  start_date: string
  end_date: string
}

export interface EarningsListQuery extends EarningsQuery {
  p: number
  page_size: number
}
